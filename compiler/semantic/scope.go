package semantic

import (
	"github.com/brimsec/zq/compiler/ast"
)

// A Scope is a stack of bindings that map identifiers to literals,
// generator variables, functions etc.  Currently, we only handle iterators
// but this will change soone as we add support for richer Z script semantics.
type Scope struct {
	stack []Binder
}

func NewScope() *Scope {
	return &Scope{}
}

func (s *Scope) tos() Binder {
	return s.stack[len(s.stack)-1]
}

func (s *Scope) Enter() {
	s.stack = append(s.stack, NewBinder())
}

func (s *Scope) Exit() {
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *Scope) Bind(name string, ref ast.Proc) {
	s.tos().Define(name, ref)
}

func (s *Scope) Lookup(name string) ast.Proc {
	for k := len(s.stack) - 1; k >= 0; k-- {
		if e, ok := s.stack[k][name]; ok {
			e.refcnt++
			return e.proc
		}
	}
	return nil
}

type entry struct {
	proc   ast.Proc
	refcnt int
}

//XXX for now, the semantic binder connects var names to type consts and consts
// XXX total hack for now... this is pointing to TypeProc and ConstProc.
// the const and types tables should be in semantic.AST and they should be
// in a top-level Z program object (instead of a generic Proc) that comes
// from the parser
type Binder map[string]entry

func NewBinder() Binder {
	return make(map[string]entry)
}

func (b Binder) Define(name string, ref ast.Proc) {
	b[name] = entry{proc: ref}
}
