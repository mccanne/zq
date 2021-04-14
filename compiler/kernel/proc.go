package kernel

import (
	"errors"
	"fmt"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/expr"
	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/proc"
	"github.com/brimdata/zed/proc/combine"
	"github.com/brimdata/zed/proc/fuse"
	"github.com/brimdata/zed/proc/head"
	"github.com/brimdata/zed/proc/join"
	"github.com/brimdata/zed/proc/merge"
	"github.com/brimdata/zed/proc/pass"
	"github.com/brimdata/zed/proc/put"
	"github.com/brimdata/zed/proc/rename"
	"github.com/brimdata/zed/proc/shape"
	"github.com/brimdata/zed/proc/sort"
	"github.com/brimdata/zed/proc/split"
	"github.com/brimdata/zed/proc/switcher"
	"github.com/brimdata/zed/proc/tail"
	"github.com/brimdata/zed/proc/top"
	"github.com/brimdata/zed/proc/uniq"
	"github.com/brimdata/zed/zbuf"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

var ErrJoinParents = errors.New("join requires two upstream parallel query paths")

type Hook func(dag.Op, *proc.Context, proc.Interface) (proc.Interface, error)

func isContainerOp(node dag.Op) bool {
	if _, ok := node.(*dag.Sequential); ok {
		return true
	}
	if _, ok := node.(*dag.Parallel); ok {
		return true
	}
	return false
}

func compileProc(custom Hook, node dag.Op, pctx *proc.Context, scope *Scope, parent proc.Interface) (proc.Interface, error) {
	if custom != nil {
		// XXX custom should take scope
		p, err := custom(node, pctx, parent)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, err
		}
	}
	switch v := node.(type) {
	case *dag.Summarize:
		return compileGroupBy(pctx, scope, parent, v)
	case *dag.Cut:
		assignments, err := compileAssignments(v.Args, pctx.Zctx, scope)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		cutter, err := expr.NewCutter(pctx.Zctx, lhs, rhs)
		if err != nil {
			return nil, err
		}
		cutter.AllowPartialCuts()
		return proc.FromFunction(pctx, parent, cutter), nil
	case *dag.Pick:
		assignments, err := compileAssignments(v.Args, pctx.Zctx, scope)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		cutter, err := expr.NewCutter(pctx.Zctx, lhs, rhs)
		if err != nil {
			return nil, err
		}
		return proc.FromFunction(pctx, parent, &picker{cutter}), nil
	case *dag.Drop:
		if len(v.Args) == 0 {
			return nil, errors.New("drop: no fields given")
		}
		fields := make([]field.Static, 0, len(v.Args))
		for _, e := range v.Args {
			field, ok := e.(*dag.Path)
			if !ok {
				return nil, errors.New("drop: arg not a field")
			}
			fields = append(fields, field.Name)
		}
		dropper := expr.NewDropper(pctx.Zctx, fields)
		return proc.FromFunction(pctx, parent, dropper), nil
	case *dag.Sort:
		fields, err := CompileExprs(pctx.Zctx, scope, v.Args)
		if err != nil {
			return nil, err
		}
		sort, err := sort.New(pctx, parent, fields, v.SortDir, v.NullsFirst)
		if err != nil {
			return nil, fmt.Errorf("compiling sort: %w", err)
		}
		return sort, nil
	case *dag.Head:
		limit := v.Count
		if limit == 0 {
			limit = 1
		}
		return head.New(parent, limit), nil
	case *dag.Tail:
		limit := v.Count
		if limit == 0 {
			limit = 1
		}
		return tail.New(parent, limit), nil
	case *dag.Uniq:
		return uniq.New(pctx, parent, v.Cflag), nil
	case *dag.Pass:
		return pass.New(parent), nil
	case *dag.Filter:
		f, err := CompileFilter(pctx.Zctx, scope, v.Expr)
		if err != nil {
			return nil, fmt.Errorf("compiling filter: %w", err)
		}
		return proc.FromFunction(pctx, parent, filterFunction(f)), nil
	case *dag.Top:
		fields, err := CompileExprs(pctx.Zctx, scope, v.Args)
		if err != nil {
			return nil, fmt.Errorf("compiling top: %w", err)
		}
		return top.New(parent, v.Limit, fields, v.Flush), nil
	case *dag.Put:
		clauses, err := compileAssignments(v.Args, pctx.Zctx, scope)
		if err != nil {
			return nil, err
		}
		put, err := put.New(pctx, parent, clauses)
		if err != nil {
			return nil, err
		}
		return put, nil
	case *dag.Rename:
		var srcs, dsts []field.Static
		for _, fa := range v.Args {
			dst, err := compileLval(fa.LHS)
			if err != nil {
				return nil, err
			}
			// We call CompileLval on the RHS because renames are
			// restricted to dotted field name expressions.
			src, err := compileLval(fa.RHS)
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
		renamer := rename.NewFunction(pctx.Zctx, srcs, dsts)
		return proc.FromFunction(pctx, parent, renamer), nil
	case *dag.Fuse:
		return fuse.New(pctx, parent)
	case *dag.Shape:
		return shape.New(pctx, parent)
	case *dag.Join:
		return nil, ErrJoinParents
	default:
		return nil, fmt.Errorf("unknown AST proc type: %v", v)

	}
}

type filterFunction expr.Filter

func (f filterFunction) Apply(rec *zng.Record) (*zng.Record, error) {
	if f(rec) {
		return rec, nil
	}
	return nil, nil
}

func (_ filterFunction) String() string { return "filter" }

func (_ filterFunction) Warning() string { return "" }

type picker struct{ *expr.Cutter }

func (_ *picker) String() string { return "pick" }

func compileAssignments(assignments []dag.Assignment, zctx *zson.Context, scope *Scope) ([]expr.Assignment, error) {
	keys := make([]expr.Assignment, 0, len(assignments))
	for _, assignment := range assignments {
		a, err := CompileAssignment(zctx, scope, &assignment)
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

func enteringJoin(ops []dag.Op) bool {
	var ok bool
	if len(ops) > 0 {
		_, ok = ops[0].(*dag.Join)
	}
	return ok
}

func mergeInfo(op dag.Op) (field.Static, bool) {
	if par, ok := op.(*dag.Parallel); ok {
		return par.MergeBy, par.MergeReverse
	}
	swi := op.(*dag.Switch)
	return swi.MergeBy, swi.MergeReverse
}

func compileSequential(custom Hook, nodes []dag.Op, pctx *proc.Context, scope *Scope, parents []proc.Interface) ([]proc.Interface, error) {
	node := nodes[0]
	parents, err := Compile(custom, node, pctx, scope, parents)
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
		orderField, orderReverse := mergeInfo(node)
		if orderField != nil {
			cmp := zbuf.NewCompareFn(orderField, orderReverse)
			parent = merge.New(pctx, parents, cmp)
		} else {
			parent = combine.New(pctx, parents)
		}
		parents = []proc.Interface{parent}
	}
	return compileSequential(custom, nodes[1:], pctx, scope, parents)
}

func compileParallel(custom Hook, parallel *dag.Parallel, c *proc.Context, scope *Scope, parents []proc.Interface) ([]proc.Interface, error) {
	n := len(parallel.Ops)
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
		return nil, fmt.Errorf("proc.CompileProc: %d parents for parallel proc with %d branches", len(parents), len(parallel.Ops))
	}
	var procs []proc.Interface
	for k := 0; k < n; k++ {
		proc, err := Compile(custom, parallel.Ops[k], c, scope, []proc.Interface{parents[k]})
		if err != nil {
			return nil, err
		}
		procs = append(procs, proc...)
	}
	return procs, nil
}

func compileSwitch(custom Hook, swtch *dag.Switch, pctx *proc.Context, scope *Scope, parents []proc.Interface) ([]proc.Interface, error) {
	n := len(swtch.Cases)
	if len(parents) == 1 {
		// Single parent: insert a switcher and wire to each branch.
		switcher := switcher.New(parents[0])
		parents = []proc.Interface{}
		for _, c := range swtch.Cases {
			f, err := CompileFilter(pctx.Zctx, scope, c.Expr)
			if err != nil {
				return nil, fmt.Errorf("compiling switch case filter: %w", err)
			}
			sc := switcher.NewProc(f)
			parents = append(parents, sc)
		}
	}
	if len(parents) != n {
		return nil, fmt.Errorf("proc.compileSwitch: %d parents for switch proc with %d branches", len(parents), len(swtch.Cases))
	}
	var procs []proc.Interface
	for k := 0; k < n; k++ {
		proc, err := Compile(custom, swtch.Cases[k].Op, pctx, scope, []proc.Interface{parents[k]})
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
func Compile(custom Hook, op dag.Op, pctx *proc.Context, scope *Scope, parents []proc.Interface) ([]proc.Interface, error) {
	if len(parents) == 0 {
		return nil, errors.New("no parents")
	}
	switch op := op.(type) {
	case *dag.Sequential:
		if len(op.Ops) == 0 {
			return nil, errors.New("sequential proc without procs")
		}
		return compileSequential(custom, op.Ops, pctx, scope, parents)

	case *dag.Parallel:
		return compileParallel(custom, op, pctx, scope, parents)

	case *dag.Switch:
		return compileSwitch(custom, op, pctx, scope, parents)

	case *dag.Join:
		if len(parents) != 2 {
			return nil, ErrJoinParents
		}
		assignments, err := compileAssignments(op.Args, pctx.Zctx, scope)
		if err != nil {
			return nil, err
		}
		lhs, rhs := splitAssignments(assignments)
		leftKey, err := compileExpr(pctx.Zctx, scope, op.LeftKey)
		if err != nil {
			return nil, err
		}
		rightKey, err := compileExpr(pctx.Zctx, scope, op.RightKey)
		if err != nil {
			return nil, err
		}
		inner := true
		leftParent := parents[0]
		rightParent := parents[1]
		switch op.Style {
		case "left":
			inner = false
		case "right":
			inner = false
			leftKey, rightKey = rightKey, leftKey
			leftParent, rightParent = rightParent, leftParent
		case "inner":
		default:
			return nil, fmt.Errorf("unknown kind of join: '%s'", op.Style)
		}
		join, err := join.New(pctx, inner, leftParent, rightParent, leftKey, rightKey, lhs, rhs)
		if err != nil {
			return nil, err
		}
		return []proc.Interface{join}, nil

	default:
		if len(parents) > 1 {
			return nil, fmt.Errorf("ast type %v cannot have multiple parents", op)
		}
		p, err := compileProc(custom, op, pctx, scope, parents[0])
		return []proc.Interface{p}, err
	}
}

func LoadConsts(zctx *zson.Context, scope *Scope, ops []dag.Op) error {
	for _, p := range ops {
		switch p := p.(type) {
		case *dag.Const:
			e, err := compileExpr(zctx, scope, p.Expr)
			if err != nil {
				return err
			}
			typ, err := zctx.LookupTypeRecord([]zng.Column{})
			if err != nil {
				return err
			}
			rec := zng.NewRecord(typ, nil)
			zv, err := e.Eval(rec)
			if err != nil {
				if err == zng.ErrMissing {
					err = fmt.Errorf("cannot resolve const '%s' at compile time", p.Name)
				}
				return err
			}
			scope.Bind(p.Name, &zv)

		case *dag.TypeProc:
			name := p.Name
			typ, err := zson.TranslateType(zctx, p.Type)
			if err != nil {
				return err
			}
			alias, err := zctx.LookupTypeAlias(name, typ)
			if err != nil {
				return err
			}
			zv := zng.NewTypeType(alias)
			scope.Bind(name, &zv)

		default:
			return fmt.Errorf("kernel.LoadConsts: not a const: '%T'", p)
		}
	}
	return nil
}
