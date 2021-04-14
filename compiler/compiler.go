package compiler

import (
	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/compiler/kernel"
	"github.com/brimdata/zed/compiler/parser"
	"github.com/brimdata/zed/compiler/semantic"
	"github.com/brimdata/zed/expr"
	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/proc"
	"github.com/brimdata/zed/zbuf"
	"github.com/brimdata/zed/zson"
)

var _ zbuf.Filter = (*Runtime)(nil)

type Runtime struct {
	zctx    *zson.Context
	scope   *kernel.Scope
	sem     *semantic.AST
	outputs []proc.Interface
}

func New(zctx *zson.Context, parserAST ast.Proc) (*Runtime, error) {
	return NewWithSortedInput(zctx, parserAST, nil, false)
}

func NewWithZ(zctx *zson.Context, z string) (*Runtime, error) {
	p, err := ParseProc(z)
	if err != nil {
		return nil, err
	}
	return New(zctx, p)
}

func NewWithSortedInput(zctx *zson.Context, parserAST ast.Proc, sortKey field.Static, sortRev bool) (*Runtime, error) {
	sem := semantic.New(parserAST)
	if err := sem.Analyze(); err != nil {
		return nil, err
	}
	if sortKey != nil {
		sem.SetInputOrder(sortKey, sortRev)
	}
	scope := kernel.NewScope()
	// enter the global scope
	scope.Enter()
	if err := kernel.LoadConsts(zctx, scope, sem.Consts()); err != nil {
		return nil, err
	}
	return &Runtime{
		zctx:  zctx,
		scope: scope,
		sem:   sem,
	}, nil
}

func (r *Runtime) Outputs() []proc.Interface {
	return r.outputs
}

func (r *Runtime) Entry() dag.Op {
	//XXX need to prepend consts depending on context
	return r.sem.Entry()
}

func (r *Runtime) AsFilter() (expr.Filter, error) {
	if r == nil {
		return nil, nil
	}
	f := r.sem.Filter()
	if f == nil {
		return nil, nil
	}
	return kernel.CompileFilter(r.zctx, r.scope, f)
}

func (r *Runtime) AsBufferFilter() (*expr.BufferFilter, error) {
	if r == nil {
		return nil, nil
	}
	f := r.sem.Filter()
	if f == nil {
		return nil, nil
	}
	return kernel.CompileBufferFilter(f)
}

// AsProc returns the lifted filter and any consts if present as a proc so that,
// for instance, the root worker (or a sub-worker) can push the filter over the
// net to the source scanner.
func (r *Runtime) AsProc() dag.Op {
	if r == nil {
		return nil
	}
	f := r.sem.Filter()
	if f == nil {
		return nil
	}
	p := &dag.Filter{
		Kind: "Filter",
		Expr: f,
	}
	consts := r.sem.Consts()
	if len(consts) == 0 {
		return p
	}
	var ops []dag.Op
	for _, p := range consts {
		ops = append(ops, p)
	}
	ops = append(ops, p)
	return &dag.Sequential{
		Kind: "Sequential",
		Ops:  ops,
	}
}

// This must be called before the zbuf.Filter interface will work.
func (r *Runtime) Optimize() error {
	return r.sem.Optimize()
}

func (r *Runtime) IsParallelizable() bool {
	return r.sem.IsParallelizable()
}

func (r *Runtime) Parallelize(n int) bool {
	return r.sem.Parallelize(n)
}

// ParseProc() is an entry point for use from external go code,
// mostly just a wrapper around Parse() that casts the return value.
func ParseProc(z string) (ast.Proc, error) {
	parsed, err := parser.ParseZ(z)
	if err != nil {
		return nil, err
	}
	return ast.UnpackMapAsProc(parsed)
}

func ParseExpression(expr string) (ast.Expr, error) {
	m, err := parser.ParseZByRule("Expr", expr)
	if err != nil {
		return nil, err
	}
	return ast.UnpackMapAsExpr(m)
}

// MustParseProc is functionally the same as ParseProc but panics if an error
// is encountered.
func MustParseProc(query string) ast.Proc {
	proc, err := ParseProc(query)
	if err != nil {
		panic(err)
	}
	return proc
}

func (r *Runtime) Compile(custom kernel.Hook, pctx *proc.Context, inputs []proc.Interface) error {
	var err error
	r.outputs, err = kernel.Compile(custom, r.sem.Entry(), pctx, r.scope, inputs)
	return err
}

func CompileAssignments(dsts []field.Static, srcs []field.Static) ([]field.Static, []expr.Evaluator) {
	return kernel.CompileAssignments(dsts, srcs)
}

func CompileProc(p ast.Proc, pctx *proc.Context, inputs []proc.Interface) (*Runtime, error) {
	r, err := New(pctx.Zctx, p)
	if err != nil {
		return nil, err
	}
	if err := r.Compile(nil, pctx, inputs); err != nil {
		return nil, err
	}
	return r, nil
}

func CompileZ(z string, pctx *proc.Context, inputs []proc.Interface) ([]proc.Interface, error) {
	p, err := ParseProc(z)
	if err != nil {
		return nil, err
	}
	runtime, err := CompileProc(p, pctx, inputs)
	if err != nil {
		return nil, err
	}
	return runtime.Outputs(), nil
}
