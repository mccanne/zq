package semantic

import (
	"encoding/json"
	"fmt"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/field"
)

var passProc = &dag.Pass{Kind: "Pass"}

//XXX
func zbufDirInt(reversed bool) int {
	if reversed {
		return -1
	}
	return 1
}

func ensureSequentialProc(p dag.Op) *dag.Sequential {
	if p, ok := p.(*dag.Sequential); ok {
		return p
	}
	return &dag.Sequential{
		Kind: "Sequential",
		Ops:  []dag.Op{p},
	}
}

func countConsts(ops []dag.Op) int {
	for k, p := range ops {
		switch p.(type) {
		case *dag.Const, *dag.TypeProc:
			continue
		default:
			return k
		}
	}
	return 0
}

// liftFilter removes the filter at the head of the flowgraph AST, if
// one is present, and returns its ast.Expression and the modified
// flowgraph AST. If the flowgraph does not start with a filter, it
// returns nil and the unmodified flowgraph.
func liftFilter(p dag.Op) (dag.Expr, dag.Op) {
	if fp, ok := p.(*dag.Filter); ok {
		return fp.Expr, passProc
	}
	seq, ok := p.(*dag.Sequential)
	if ok && len(seq.Ops) > 0 {
		nc := countConsts(seq.Ops)
		if nc != 0 {
			panic("internal error: consts should have been removed from AST")
		}
		if fp, ok := seq.Ops[0].(*dag.Filter); ok {
			rest := dag.Op(passProc)
			if len(seq.Ops) > 1 {
				rest = &dag.Sequential{
					Kind: "Sequential",
					Ops:  seq.Ops[1:],
				}
			}
			return fp.Expr, rest
		}
	}
	return nil, p
}

// all fields should be turned into field paths by initial semantic pass

func exprToField(e dag.Expr) field.Static {
	f, ok := e.(*dag.Path)
	if !ok {
		return nil
	}
	return f.Name
}

func eq(e dag.Expr, b field.Static) bool {
	a := exprToField(e)
	if a == nil {
		return false
	}
	return a.Equal(b)
}

// SetGroupByProcInputSortDir examines p under the assumption that its input is
// sorted according to inputSortField and inputSortDir.  If p is an
// ast.GroupByProc and SetGroupByProcInputSortDir can determine that its first
// grouping key is inputSortField or an order-preserving function of
// inputSortField, SetGroupByProcInputSortDir sets ast.GroupByProc.InputSortDir
// to inputSortDir.  SetGroupByProcInputSortDir returns true if it determines
// that p's output will remain sorted according to inputSortField and
// inputSortDir; otherwise, it returns false.
func SetGroupByProcInputSortDir(p dag.Op, inputSortField field.Static, inputSortDir int) bool {
	switch p := p.(type) {
	case *dag.Cut:
		// Return true if the output record contains inputSortField.
		for _, f := range p.Args {
			if eq(f.RHS, inputSortField) {
				return true
			}
		}
		return false
	case *dag.Pick:
		// Return true if the output record contains inputSortField.
		for _, f := range p.Args {
			if eq(f.RHS, inputSortField) {
				return true
			}
		}
		return false
	case *dag.Drop:
		// Return true if the output record contains inputSortField.
		for _, e := range p.Args {
			if eq(e, inputSortField) {
				return false
			}
		}
		return true
	case *dag.Summarize:
		// Set p.InputSortDir and return true if the first grouping key
		// is inputSortField or an order-preserving function of it.
		if len(p.Keys) > 0 && eq(p.Keys[0].LHS, inputSortField) {
			rhs := exprToField(p.Keys[0].RHS)
			if rhs != nil && rhs.Equal(inputSortField) {
				p.InputSortDir = inputSortDir
				return true
			}
			if call, ok := p.Keys[0].RHS.(*dag.Call); ok {
				switch call.Name {
				case "ceil", "floor", "round", "trunc":
					if len(call.Args) == 0 {
						return false
					}
					arg0 := exprToField(call.Args[0])
					if arg0 != nil && arg0.Equal(inputSortField) {
						p.InputSortDir = inputSortDir
						return true
					}
				}
			}
		}
		return false
	case *dag.Put:
		for _, c := range p.Args {
			lhs := exprToField(c.LHS)
			if lhs != nil && lhs.Equal(inputSortField) {
				return false
			}
		}
		return true
	case *dag.Sequential:
		for _, pp := range p.Ops {
			if !SetGroupByProcInputSortDir(pp, inputSortField, inputSortDir) {
				return false
			}
		}
		return true
	case *dag.Filter, *dag.Head, *dag.Pass, *dag.Uniq, *dag.Tail, *dag.Fuse, *dag.Const, *dag.TypeProc:
		return true
	default:
		return false
	}
}

func copyOps(ops []dag.Op) []dag.Op {
	var copies []dag.Op
	for _, p := range ops {
		copies = append(copies, copyOp(p))
	}
	return copies
}

func copyOp(p dag.Op) dag.Op {
	if p == nil {
		panic("copyOp nil")
	}
	b, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	copy, err := dag.UnpackJSONAsOp(b)
	if err != nil {
		panic(err)
	}
	return copy
}

func buildSplitFlowgraph(branch, tail []dag.Op, mergeField field.Static, reverse bool, N int) (*dag.Sequential, bool) {
	if len(branch) == 0 {
		return &dag.Sequential{
			Kind: "Sequential",
			Ops:  tail,
		}, false
	}
	if len(tail) == 0 && mergeField != nil {
		// Insert a pass tail in order to force a merge of the
		// parallel branches when compiling. (Trailing parallel branches are wired to
		// a mux output).
		tail = []dag.Op{passProc}
	}
	pp := &dag.Parallel{
		Kind:         "Parallel",
		Ops:          []dag.Op{},
		MergeBy:      mergeField,
		MergeReverse: reverse,
	}
	for i := 0; i < N; i++ {
		pp.Ops = append(pp.Ops, &dag.Sequential{
			Kind: "Sequential",
			Ops:  copyOps(branch),
		})
	}
	return &dag.Sequential{
		Kind: "Sequential",
		Ops:  append([]dag.Op{pp}, tail...),
	}, true
}

func parallelize(p dag.Op, N int, inputSortField field.Static, inputSortReversed bool) (*dag.Sequential, bool) {
	if p == nil {
		panic("parallelize nil")
	}

	seq := ensureSequentialProc(p)
	orderSensitiveTail := true
loop:
	for i := range seq.Ops {
		switch seq.Ops[i].(type) {
		case *dag.Sort, *dag.Summarize:
			orderSensitiveTail = false
			break loop
		default:
			continue
		}
	}
	for i := range seq.Ops {
		switch p := seq.Ops[i].(type) {
		case *dag.Filter, *dag.Pass, *dag.Const, *dag.TypeProc:
			// Stateless procs: continue until we reach one of the procs below at
			// which point we'll either split the flowgraph or see we can't and return it as-is.
			continue
		case *dag.Cut, *dag.Pick:
			if inputSortField == nil || !orderSensitiveTail {
				continue
			}
			var fields []dag.Assignment
			if cut, ok := p.(*dag.Cut); ok {
				fields = cut.Args
			} else {
				fields = p.(*dag.Pick).Args
			}
			var found bool
			for _, f := range fields {
				fieldName := exprToField(f.RHS)
				lhs := exprToField(f.LHS)
				if fieldName != nil && !fieldName.Equal(inputSortField) && lhs.Equal(inputSortField) {
					return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
				}
				if fieldName != nil && fieldName.Equal(inputSortField) && (lhs == nil || lhs.Equal(inputSortField)) {
					found = true
				}
			}
			if !found {
				return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
			}
		case *dag.Drop:
			if inputSortField == nil || !orderSensitiveTail {
				continue
			}
			for _, e := range p.Args {
				if eq(e, inputSortField) {
					return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
				}
			}
			continue
		case *dag.Put:
			if inputSortField == nil || !orderSensitiveTail {
				continue
			}
			for _, c := range p.Args {
				if eq(c.LHS, inputSortField) {
					return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
				}
			}
			continue
		case *dag.Rename:
			if inputSortField == nil || !orderSensitiveTail {
				continue
			}
			for _, f := range p.Args {
				if eq(f.LHS, inputSortField) {
					return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
				}
			}
		case *dag.Summarize:
			// To decompose the groupby, we split the flowgraph into branches that run up to and including a groupby,
			// followed by a post-merge groupby that composes the results.
			var mergeField field.Static
			if p.Duration != nil {
				// Group by time requires a time-ordered merge, irrespective of any upstream ordering.
				mergeField = field.New("ts")
			}
			branch := copyOps(seq.Ops[0 : i+1])
			branch[len(branch)-1].(*dag.Summarize).PartialsOut = true

			composerGroupBy := copyOps([]dag.Op{p})[0].(*dag.Summarize)
			composerGroupBy.PartialsIn = true

			return buildSplitFlowgraph(branch, append([]dag.Op{composerGroupBy}, seq.Ops[i+1:]...), mergeField, inputSortReversed, N)
		case *dag.Sort:
			dir := map[int]bool{-1: true, 1: false}[p.SortDir]
			if len(p.Args) == 1 {
				// Single sort field: we can sort in each parallel branch, and then do an ordered merge.
				mergeField := exprToField(p.Args[0])
				if mergeField == nil {
					return seq, false
				}
				return buildSplitFlowgraph(seq.Ops[0:i+1], seq.Ops[i+1:], mergeField, dir, N)
			} else {
				// Unknown or multiple sort fields: we sort after the merge point, which can be unordered.
				return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], nil, dir, N)
			}
		case *dag.Parallel:
			return seq, false
		case *dag.Head, *dag.Tail:
			if inputSortField == nil {
				// Unknown order: we can't parallelize because we can't maintain this unknown order at the merge point.
				return seq, false
			}
			// put one head/tail on each parallel branch and one after the merge.
			return buildSplitFlowgraph(seq.Ops[0:i+1], seq.Ops[i:], inputSortField, inputSortReversed, N)
		case *dag.Uniq, *dag.Fuse:
			if inputSortField == nil {
				// Unknown order: we can't parallelize because we can't maintain this unknown order at the merge point.
				return seq, false
			}
			// Split all the upstream procs into parallel branches, then merge and continue with this and any remaining procs.
			return buildSplitFlowgraph(seq.Ops[0:i], seq.Ops[i:], inputSortField, inputSortReversed, N)
		case *dag.Sequential:
			return seq, false
			// XXX Joins can be parallelized but we need to write
			// the code to parallelize the flow graph, which is a bit
			// different from how group-bys are parallelized.
		case *dag.Join:
			return seq, false
		default:
			panic(fmt.Sprintf("proc type not handled: %T", p))
		}
	}
	// If we're here, we reached the end of the flowgraph without
	// coming across a merge-forcing proc. If inputs are sorted,
	// we can parallelize the entire chain and do an ordered
	// merge. Otherwise, no parallelization.
	if inputSortField == nil {
		return seq, false
	}
	return buildSplitFlowgraph(seq.Ops, nil, inputSortField, inputSortReversed, N)
}
