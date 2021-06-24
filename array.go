package zed

import (
	"fmt"
	"strings"

	"github.com/brimdata/zed/zcode"
)

type TypeArray struct {
	id   int
	Type Type
}

func NewTypeArray(id int, typ Type) *TypeArray {
	return &TypeArray{id, typ}
}

func (t *TypeArray) ID() int {
	return t.id
}

func (t *TypeArray) String() string {
	return fmt.Sprintf("[%s]", t.Type)
}

func (t *TypeArray) Marshal(zv zcode.Bytes) (interface{}, error) {
	// start out with zero-length container so we get "[]" instead of nil
	vals := []Value{}
	it := zv.Iter()
	for !it.Done() {
		val, _, err := it.Next()
		if err != nil {
			return nil, err
		}
		vals = append(vals, Value{t.Type, val})
	}
	return vals, nil
}

func (t *TypeArray) Format(zv zcode.Bytes) string {
	var b strings.Builder
	sep := ""
	b.WriteByte('[')
	it := zv.Iter()
	for !it.Done() {
		val, _, err := it.Next()
		if err != nil {
			return badZng(err, t, zv)
		}
		b.WriteString(sep)
		if val == nil {
			b.WriteString("null")
		} else {
			b.WriteString(t.Type.Format(val))
		}
		sep = ","
	}
	b.WriteByte(']')
	return b.String()
}
