package semantic

import (
	"github.com/brimsec/zq/compiler/ast"
	"github.com/brimsec/zq/field"
)

type AST struct {
	entry   ast.Proc
	unopt   ast.Proc
	consts  []ast.Proc
	filter  ast.Expression
	sortKey field.Static
	sortRev bool
}

func New(entry ast.Proc) *AST {
	if entry == nil {
		entry = passProc
	}
	return &AST{entry: entry}
}

func (a *AST) Entry() ast.Proc {
	return a.entry
}

func (a *AST) Consts() []ast.Proc {
	return a.consts
}

func (a *AST) Filter() ast.Expression {
	return a.filter
}

// Analyze analysis the AST and prepares it for runtime compilation.
// This is called in lieu of Optimize when optimization is not wanted.
func (a *AST) Analyze() error {
	var err error
	scope := NewScope()
	scope.Enter()
	a.consts, err = semConsts(nil, scope, a.entry)
	if err != nil {
		return err
	}
	a.entry, err = semProc(scope, a.entry)
	a.unopt = a.entry
	return err
}

func (a *AST) Optimize() error {
	// Currently, we only lift the filter into the buffer scanner
	// and push it out to distributed workers.  This could also be
	// used by ZST to read only the columns but we need a bit more
	// data structure here.  Also, this is where we would push
	// analytics and search|cut into the column reader.
	a.filter, a.entry = liftFilter(a.entry)
	return nil
}

func (a *AST) SetInputOrder(sortKey field.Static, sortRev bool) {
	a.sortKey = sortKey
	a.sortRev = sortRev
	SetGroupByProcInputSortDir(a.entry, sortKey, zbufDirInt(sortRev))
}

// IsParallelizable reports whether Parallelize can parallelize p when called
// with the same arguments.
func (a *AST) IsParallelizable() bool {
	_, ok := parallelize(copyProc(a.unopt), 0, a.sortKey, a.sortRev)
	return ok
}

// Parallelize takes a sequential proc AST and tries to
// parallelize it by splitting as much as possible of the sequence
// into n parallel branches. The boolean return argument indicates
// whether the flowgraph could be parallelized.
func (a *AST) Parallelize(n int) bool {
	p, ok := parallelize(copyProc(a.unopt), n, a.sortKey, a.sortRev)
	if ok {
		a.entry = p
	}
	return ok
}
