package function

import (
	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

type fields struct {
	zctx  *zson.Context
	typ   zng.Type
	bytes zcode.Bytes
}

func fieldNames(typ *zng.TypeRecord) []string {
	var out []string
	for _, c := range typ.Columns {
		if typ, ok := zng.AliasOf(c.Type).(*zng.TypeRecord); ok {
			for _, subfield := range fieldNames(typ) {
				out = append(out, c.Name+"."+subfield)
			}
		} else {
			out = append(out, c.Name)
		}
	}
	return out
}

func (f *fields) Call(args []zng.Value) (zng.Value, error) {
	zvSubject := args[0]
	typ := isRecordType(zvSubject, f.zctx)
	if typ == nil {
		return zng.Missing, nil
	}
	bytes := f.bytes[:0]
	for _, field := range fieldNames(typ) {
		bytes = zcode.AppendPrimitive(bytes, zcode.Bytes(field))
	}
	f.bytes = bytes
	return zng.Value{f.typ, bytes}, nil
}

func isRecordType(zv zng.Value, zctx *zson.Context) *zng.TypeRecord {
	if typ, ok := zng.AliasOf(zv.Type).(*zng.TypeRecord); ok {
		return typ
	}
	if zv.Type == zng.TypeType {
		typ, err := zctx.LookupByValue(zv.Bytes)
		if err != nil {
			return nil
		}
		if typ, ok := zng.AliasOf(typ).(*zng.TypeRecord); ok {
			return typ
		}
	}
	return nil
}
