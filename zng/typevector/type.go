package typevector

import (
	"github.com/brimsec/zq/zng"
)

type Type []zng.Type

func New(in []zng.Type) Type {
	out := make(Type, 0, len(in))
	for _, t := range in {
		out = append(out, t)
	}
	return out
}

func NewFromValues(vals []zng.Value) Type {
	out := make(Type, 0, len(vals))
	for _, zv := range vals {
		out = append(out, zv.Type)
	}
	return out
}

func (t Type) Equal(to []zng.Type) bool {
	if len(t) != len(to) {
		return false
	}
	for k, typ := range t {
		if typ != to[k] {
			return false
		}
	}
	return true
}

func (t Type) EqualToValues(vals []zng.Value) bool {
	if len(t) != len(vals) {
		return false
	}
	for k, typ := range t {
		if typ != vals[k].Type {
			return false
		}
	}
	return true
}

type Table struct {
	types []Type
}

func NewTable() *Table {
	return &Table{}
}

func (t *Table) Lookup(types []zng.Type) int {
	for k, typ := range t.types {
		if typ.Equal(types) {
			return k
		}
	}
	k := len(t.types)
	t.types = append(t.types, New(types))
	return k
}

func (t *Table) LookupByValues(vals []zng.Value) int {
	for k, typ := range t.types {
		if typ.EqualToValues(vals) {
			return k
		}
	}
	k := len(t.types)
	t.types = append(t.types, NewFromValues(vals))
	return k
}

func (t *Table) Types(id int) []zng.Type {
	return t.types[id]
}

func (t *Table) Length() int {
	return len(t.types)
}
