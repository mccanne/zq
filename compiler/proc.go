package compiler

import (
	"errors"
	"fmt"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/proc"
	"github.com/brimsec/zq/proc/combine"
	filterproc "github.com/brimsec/zq/proc/filter"
	"github.com/brimsec/zq/proc/fuse"
	"github.com/brimsec/zq/proc/head"
	"github.com/brimsec/zq/proc/join"
	"github.com/brimsec/zq/proc/merge"
	"github.com/brimsec/zq/proc/pass"
	"github.com/brimsec/zq/proc/put"
	"github.com/brimsec/zq/proc/rename"
	"github.com/brimsec/zq/proc/sort"
	"github.com/brimsec/zq/proc/split"
	"github.com/brimsec/zq/proc/tail"
	"github.com/brimsec/zq/proc/top"
	"github.com/brimsec/zq/proc/uniq"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng/resolver"
)

var ErrJoinParents = errors.New("join requires two upstream parallel query paths")

type Hook func(ast.Proc, *proc.Context, proc.Interface) (proc.Interface, error)

func isContainerProc(node ast.Proc) bool {
	if _, ok := node.(*ast.SequentialProc); ok {
		return true
	}
	if _, ok := node.(*ast.ParallelProc); ok {
		return true
	}
	return false
}

func compileProc(custom Hook, node ast.Proc, pctx *proc.Context, parent proc.Interface) (proc.Interface, error) {
	if custom != nil {
		p, err := custom(node, pctx, parent)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, err
		}
	}
	switch v := node.(type) {
	case *ast.GroupByProc:
		return compileGroupBy(pctx, parent, v)

	case *ast.CutProc:
		if v.Complement {
			return nil, errors.New("cut -c deprecated; use drop instead")
		}
		assignments, err := compileAssignments(v.Fields, pctx.TypeContext)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		cutter, err := expr.NewCutter(pctx.TypeContext, lhs, rhs)
		if err != nil {
			return nil, err
		}
		cutter.AllowPartialCuts()
		return proc.FromFunction(pctx, parent, cutter, "cut"), nil

	case *ast.PickProc:
		assignments, err := compileAssignments(v.Fields, pctx.TypeContext)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		cutter, err := expr.NewCutter(pctx.TypeContext, lhs, rhs)
		if err != nil {
			return nil, err
		}
		return proc.FromFunction(pctx, parent, cutter, "pick"), nil

	case *ast.DropProc:
		if len(v.Fields) == 0 {
			return nil, errors.New("drop: no fields given")
		}
		fields := make([]field.Static, 0, len(v.Fields))
		for _, e := range v.Fields {
			field, ok := ast.DotExprToField(e)
			if !ok {
				return nil, errors.New("drop: arg not a field")
			}
			fields = append(fields, field)
		}
		dropper := expr.NewDropper(pctx.TypeContext, fields)
		return proc.FromFunction(pctx, parent, dropper, "drop"), nil

	case *ast.SortProc:
		fields, err := CompileExprs(pctx.TypeContext, v.Fields)
		if err != nil {
			return nil, err
		}
		sort, err := sort.New(pctx, parent, fields, v.SortDir, v.NullsFirst)
		if err != nil {
			return nil, fmt.Errorf("compiling sort: %w", err)
		}
		return sort, nil

	case *ast.HeadProc:
		limit := v.Count
		if limit == 0 {
			limit = 1
		}
		return head.New(parent, limit), nil

	case *ast.TailProc:
		limit := v.Count
		if limit == 0 {
			limit = 1
		}
		return tail.New(parent, limit), nil

	case *ast.UniqProc:
		return uniq.New(pctx, parent, v.Cflag), nil

	case *ast.PassProc:
		return pass.New(parent), nil

	case *ast.FilterProc:
		f, err := compileFilter(pctx.TypeContext, v.Filter)
		if err != nil {
			return nil, fmt.Errorf("compiling filter: %w", err)
		}
		return filterproc.New(parent, f), nil

	case *ast.TopProc:
		fields, err := CompileExprs(pctx.TypeContext, v.Fields)
		if err != nil {
			return nil, fmt.Errorf("compiling top: %w", err)
		}
		return top.New(parent, v.Limit, fields, v.Flush), nil

	case *ast.PutProc:
		clauses, err := compileAssignments(v.Clauses, pctx.TypeContext)
		if err != nil {
			return nil, err
		}
		// XXX fix this limitation (see issue #1753)
		for _, c := range clauses {
			if len(c.LHS) > 1 {
				name := c.LHS.String()
				return nil, fmt.Errorf("%s: put currently supports only top-level field assignemnts", name)
			}
		}
		put, err := put.New(pctx, parent, clauses)
		if err != nil {
			return nil, err
		}
		return put, nil

	case *ast.RenameProc:
		var srcs, dsts []field.Static
		for _, fa := range v.Fields {
			dst, err := CompileLval(fa.LHS)
			if err != nil {
				return nil, err
			}
			// We call CompileLval on the RHS because renames are
			// restricted to dotted field name expressions.
			src, err := CompileLval(fa.RHS)
			if err != nil {
				return nil, err
			}
			if len(dst) != len(src) {
				return nil, fmt.Errorf("cannot rename %s to %s", src, dst)
			}
			// Check that the prefixes match and, if not, report first place
			// that they don't.
			for i := 0; i <= len(src)-2; i++ {
				if src[i] != dst[i] {
					return nil, fmt.Errorf("cannot rename %s to %s (differ in %s vs %s)", src, dst, src[i], dst[i])
				}
			}
			dsts = append(dsts, dst)
			srcs = append(srcs, src)
		}
		rename, err := rename.New(pctx, parent, srcs, dsts)
		if err != nil {
			return nil, err
		}
		return rename, nil

	case *ast.FuseProc:
		return fuse.New(pctx, parent)

	case *ast.JoinProc:
		return nil, ErrJoinParents

	default:
		return nil, fmt.Errorf("unknown AST type: %v", v)

	}
}

func compileAssignments(assignments []ast.Assignment, zctx *resolver.Context) ([]expr.Assignment, error) {
	keys := make([]expr.Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		a, err := CompileAssignment(zctx, &assignment)
		if err != nil {
			return nil, err
		}
		keys = append(keys, a)
	}
	return keys, nil
}

func splitAssignments(assignments []expr.Assignment) ([]field.Static, []expr.Evaluator) {
	n := len(assignments)
	lhs := make([]field.Static, 0, n)
	rhs := make([]expr.Evaluator, 0, n)
	for _, a := range assignments {
		lhs = append(lhs, a.LHS)
		rhs = append(rhs, a.RHS)
	}
	return lhs, rhs
}

func enteringJoin(nodes []ast.Proc) bool {
	var ok bool
	if len(nodes) > 0 {
		_, ok = nodes[0].(*ast.JoinProc)
	}
	return ok
}

func compileSequential(custom Hook, nodes []ast.Proc, pctx *proc.Context, parents []proc.Interface) ([]proc.Interface, error) {
	node := nodes[0]
	parents, err := Compile(custom, node, pctx, parents)
	if err != nil {
		return nil, err
	}
	// merge unless we're at the end of the chain,
	// in which case the output layer will mux
	// into channels.
	if len(nodes) == 1 {
		return parents, nil
	}
	if len(parents) > 1 && !enteringJoin(nodes[1:]) {
		var parent proc.Interface
		p := node.(*ast.ParallelProc)
		if p.MergeOrderField != nil {
			cmp := zbuf.NewCompareFn(p.MergeOrderField, p.MergeOrderReverse)
			parent = merge.New(pctx, parents, cmp)
		} else {
			parent = combine.New(pctx, parents)
		}
		parents = []proc.Interface{parent}
	}
	return compileSequential(custom, nodes[1:], pctx, parents)
}

func compileParallel(custom Hook, pp *ast.ParallelProc, c *proc.Context, parents []proc.Interface) ([]proc.Interface, error) {
	n := len(pp.Procs)
	if len(parents) == 1 {
		// Single parent: insert a splitter and wire to each branch.
		splitter := split.New(parents[0])
		parents = []proc.Interface{}
		for k := 0; k < n; k++ {
			sc := splitter.NewProc()
			parents = append(parents, sc)
		}
	}
	if len(parents) != n {
		return nil, fmt.Errorf("proc.CompileProc: %d parents for parallel proc with %d branches", len(parents), len(pp.Procs))
	}
	var procs []proc.Interface
	for k := 0; k < n; k++ {
		proc, err := Compile(custom, pp.Procs[k], c, []proc.Interface{parents[k]})
		if err != nil {
			return nil, err
		}
		procs = append(procs, proc...)
	}
	return procs, nil
}

// Compile compiles an AST into a graph of Procs, and returns
// the leaves.  A custom compiler hook can be included and it will be tried first
// for each node encountered during the compilation.
func Compile(custom Hook, node ast.Proc, pctx *proc.Context, parents []proc.Interface) ([]proc.Interface, error) {
	if len(parents) == 0 {
		return nil, errors.New("no parents")
	}
	switch node := node.(type) {
	case *ast.SequentialProc:
		if len(node.Procs) == 0 {
			return nil, errors.New("sequential proc without procs")
		}
		return compileSequential(custom, node.Procs, pctx, parents)

	case *ast.ParallelProc:
		return compileParallel(custom, node, pctx, parents)

	case *ast.JoinProc:
		if len(parents) != 2 {
			return nil, ErrJoinParents
		}
		assignments, err := compileAssignments(node.Clauses, pctx.TypeContext)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		leftKey, err := CompileExpr(pctx.TypeContext, node.LeftKey)
		if err != nil {
			return nil, err
		}
		rightKey, err := CompileExpr(pctx.TypeContext, node.RightKey)
		if err != nil {
			return nil, err
		}
		join, err := join.New(pctx, parents[0], parents[1], leftKey, rightKey, lhs, rhs)
		if err != nil {
			return nil, err
		}
		return []proc.Interface{join}, nil

	default:
		if len(parents) > 1 {
			return nil, fmt.Errorf("ast type %v cannot have multiple parents", node)
		}
		p, err := compileProc(custom, node, pctx, parents[0])
		return []proc.Interface{p}, err
	}
}
