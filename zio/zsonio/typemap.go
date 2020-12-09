package zsonio

import (
	"github.com/brimsec/zq/zng"
)

type typemap map[zng.Type]string

func (t typemap) exists(typ zng.Type) bool {
	_, ok := t[typ]
	return ok
}

func (t typemap) known(typ zng.Type) bool {
	if _, ok := t[typ]; ok {
		return true
	}
	if _, ok := typ.(*zng.TypeOfType); ok {
		return true
	}
	if _, ok := typ.(*zng.TypeAlias); ok {
		return false
	}
	return typ.ID() < zng.IdTypeDef
}
