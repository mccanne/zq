package zed

import (
	"errors"
	"strings"

	"github.com/brimdata/zed/zcode"
)

type TypeEnum struct {
	id      int
	Symbols []string
}

func NewTypeEnum(id int, symbols []string) *TypeEnum {
	return &TypeEnum{id, symbols}
}

func (t *TypeEnum) ID() int {
	return t.id
}

func (t *TypeEnum) Symbol(index int) (string, error) {
	if index < 0 || index >= len(t.Symbols) {
		return "", ErrEnumIndex
	}
	return t.Symbols[index], nil
}

func (t *TypeEnum) Lookup(symbol string) int {
	for k, s := range t.Symbols {
		if s == symbol {
			return k
		}
	}
	return -1
}

func (t *TypeEnum) Marshal(zv zcode.Bytes) (interface{}, error) {
	return TypeUint64.Marshal(zv)
}

func (t *TypeEnum) String() string {
	var b strings.Builder
	b.WriteByte('<')
	for k, s := range t.Symbols {
		if k > 0 {
			b.WriteByte(',')
		}
		b.WriteString(QuotedName(s))
	}
	b.WriteByte('>')
	return b.String()
}

func (t *TypeEnum) Format(zv zcode.Bytes) string {
	id, err := DecodeUint(zv)
	if id >= uint64(len(t.Symbols)) || err != nil {
		if err == nil {
			err = errors.New("enum index out of range")
		}
		return badZng(err, t, zv)
	}
	return t.Symbols[id]
}
