package semantic

import (
	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/field"
)

type AST struct {
	ast     ast.Proc
	dag     dag.Op
	unopt   dag.Op
	consts  []dag.Op
	filter  dag.Expr
	sortKey field.Static
	sortRev bool
}

func New(entry ast.Proc) *AST {
	if entry == nil {
		entry = &ast.Pass{"Pass"}
	}
	return &AST{ast: entry}
}

func (a *AST) Entry() dag.Op {
	return a.dag
}

func (a *AST) Consts() []dag.Op {
	return a.consts
}

func (a *AST) Filter() dag.Expr {
	return a.filter
}

// Analyze analysis the AST and prepares it for runtime compilation.
func (a *AST) Analyze() error {
	var err error
	scope := NewScope()
	scope.Enter()
	a.consts, err = semConsts(nil, scope, a.ast)
	if err != nil {
		return err
	}
	a.dag, err = semProc(scope, a.ast)
	if err != nil {
		return err
	}
	a.unopt = a.dag
	return nil
}

func (a *AST) Optimize() error {
	// Currently, we only lift the filter into the buffer scanner
	// and push it out to distributed workers.  This could also be
	// used by ZST to read only the columns but we need a bit more
	// data structure here.  Also, this is where we would push
	// analytics and search|cut into the column reader.
	a.filter, a.dag = liftFilter(a.dag)
	return nil
}

func (a *AST) SetInputOrder(sortKey field.Static, sortRev bool) {
	a.sortKey = sortKey
	a.sortRev = sortRev
	SetGroupByProcInputSortDir(a.dag, sortKey, zbufDirInt(sortRev))
}

// IsParallelizable reports whether Parallelize can parallelize p when called
// with the same arguments.
func (a *AST) IsParallelizable() bool {
	_, ok := parallelize(copyOp(a.unopt), 0, a.sortKey, a.sortRev)
	return ok
}

// Parallelize takes a sequential proc AST and tries to
// parallelize it by splitting as much as possible of the sequence
// into n parallel branches. The boolean return argument indicates
// whether the flowgraph could be parallelized.
func (a *AST) Parallelize(n int) bool {
	p, ok := parallelize(copyOp(a.unopt), n, a.sortKey, a.sortRev)
	if ok {
		a.dag = p
	}
	return ok
}
