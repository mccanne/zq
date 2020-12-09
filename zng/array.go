package zng

import (
	"fmt"
	"strings"

	"github.com/brimsec/zq/zcode"
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

//XXX get rid of this when we implement full ZNG
func (t *TypeArray) SetID(id int) {
	t.id = id
}

func (t *TypeArray) String() string {
	return fmt.Sprintf("array[%s]", t.Type)
}

func (t *TypeArray) Decode(zv zcode.Bytes) ([]Value, error) {
	if zv == nil {
		return nil, nil
	}
	return parseContainer(t, t.Type, zv)
}

func (t *TypeArray) Parse(in []byte) (zcode.Bytes, error) {
	return ParseContainer(t, in)
}

func (t *TypeArray) StringOf(zv zcode.Bytes, fmt OutFmt, _ bool) string {
	if len(zv) == 0 && (fmt == OutFormatZeek || fmt == OutFormatZeekAscii) {
		return "(empty)"
	}

	var b strings.Builder
	separator := byte(',')
	if fmt == OutFormatZNG {
		b.WriteByte('[')
		separator = ';'
	}

	first := true
	it := zv.Iter()
	for !it.Done() {
		val, _, err := it.Next()
		if err != nil {
			//XXX
			b.WriteString("ERR")
			break
		}
		if first {
			first = false
		} else {
			b.WriteByte(separator)
		}
		if val == nil {
			b.WriteByte('-')
		} else {
			b.WriteString(t.Type.StringOf(val, fmt, true))
		}
	}

	if fmt == OutFormatZNG {
		if !first {
			b.WriteByte(';')
		}
		b.WriteByte(']')
	}
	return b.String()
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

func (t *TypeArray) ZSON() string {
	return fmt.Sprintf("[%s]", t.Type.ZSON())
}

func (t *TypeArray) ZSONOf(zv zcode.Bytes) string {
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
			b.WriteString(t.Type.ZSONOf(val))
		}
		sep = ","
	}
	b.WriteByte(']')
	return b.String()
}
