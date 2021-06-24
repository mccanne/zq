package zed

import (
	"fmt"

	"github.com/brimdata/zed/zcode"
)

type TypeAlias struct {
	id   int
	Name string
	Type Type
}

func NewTypeAlias(id int, name string, typ Type) *TypeAlias {
	return &TypeAlias{
		id:   id,
		Name: name,
		Type: typ,
	}
}

func (t *TypeAlias) ID() int {
	return t.Type.ID()
}

func (t *TypeAlias) AliasID() int {
	return t.id
}

func (t *TypeAlias) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Type.Marshal(zv)
}

func (t *TypeAlias) String() string {
	return fmt.Sprintf("%s=(%s)", t.Name, t.Type)
}

func (t *TypeAlias) Format(zv zcode.Bytes) string {
	return t.Type.Format(zv)
}

func AliasOf(typ Type) Type {
	alias, ok := typ.(*TypeAlias)
	if ok {
		return AliasOf(alias.Type)
	}
	return typ
}
