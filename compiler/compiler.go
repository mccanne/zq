package compiler

import (
	"github.com/brimsec/zq/compiler/ast"
	"github.com/brimsec/zq/compiler/kernel"
	"github.com/brimsec/zq/compiler/semantic"
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/proc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zql"
)

var _ zbuf.Filter = (*Runtime)(nil)

type Runtime struct {
	// Runtime implements zbuf.Filter via kernel.Filter
	zctx    *resolver.Context
	scope   *kernel.Scope
	sem     *semantic.AST
	outputs []proc.Interface
}

// XXX Filter wraps an ast.BooleanExpr and implements the zbuf.Filter interface
// so that scanners can generate filters and buffer filters from an AST without
// importing compiler (and causing an import loop).

func New(zctx *resolver.Context, parserAST ast.Proc) (*Runtime, error) {
	return NewWithSortedInput(zctx, parserAST, nil, false)
}

func NewWithZ(zctx *resolver.Context, z string) (*Runtime, error) {
	p, err := ParseProc(z)
	if err != nil {
		return nil, err
	}
	return New(zctx, p)
}

func NewWithSortedInput(zctx *resolver.Context, parserAST ast.Proc, sortKey field.Static, sortRev bool) (*Runtime, error) {
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

func (r *Runtime) Entry() ast.Proc {
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

// AsProc returns the lifted filter if present as a proc so that, for instance,
// the root worker (or a sub-worker) can push the filter over the net to the
// source scanner.
func (r *Runtime) AsProc() ast.Proc {
	if r == nil {
		return nil
	}
	f := r.sem.Filter()
	if f == nil {
		return nil
	}
	p := ast.FilterToProc(f)
	consts := r.sem.Consts()
	if len(consts) == 0 {
		return p
	}
	var procs []ast.Proc
	for _, p := range consts {
		procs = append(procs, p)
	}
	procs = append(procs, p)
	return &ast.SequentialProc{
		Op:    "SequentialProc",
		Procs: procs,
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
	parsed, err := zql.ParseZ(z)
	if err != nil {
		return nil, err
	}
	return ast.UnpackMapAsProc(parsed)
}

func ParseExpression(expr string) (ast.Expression, error) {
	m, err := zql.ParseZByRule("Expr", expr)
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

//XXX this is used by proctest.  proctest should go away.
func CompileProc(p ast.Proc, pctx *proc.Context, inputs []proc.Interface) (*Runtime, error) {
	r, err := New(pctx.TypeContext, p)
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
