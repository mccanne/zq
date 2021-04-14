package semantic

import (
	"github.com/brimdata/zed/compiler/ast/dag"
)

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

func (s *Scope) Bind(name string, ref dag.Op) {
	s.tos().Define(name, ref)
}

func (s *Scope) Lookup(name string) dag.Op {
	for k := len(s.stack) - 1; k >= 0; k-- {
		if e, ok := s.stack[k][name]; ok {
			e.refcnt++
			return e.op
		}
	}
	return nil
}

type entry struct {
	op     dag.Op
	refcnt int
}

type Binder map[string]*entry

func NewBinder() Binder {
	return make(map[string]*entry)
}

func (b Binder) Define(name string, ref dag.Op) {
	b[name] = &entry{op: ref}
}
