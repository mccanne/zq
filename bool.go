package zed

import (
	"github.com/brimdata/zed/zcode"
)

type TypeOfBool struct{}

var False = Value{TypeBool, []byte{0}}
var True = Value{TypeBool, []byte{1}}

func IsTrue(zv zcode.Bytes) bool {
	return zv[0] != 0
}

func NewBool(b bool) Value {
	return Value{TypeBool, EncodeBool(b)}
}

func AppendBool(zb zcode.Bytes, b bool) zcode.Bytes {
	if b {
		return append(zb, 1)
	}
	return append(zb, 0)
}

func EncodeBool(b bool) zcode.Bytes {
	return AppendBool(nil, b)
}

func DecodeBool(zv zcode.Bytes) (bool, error) {
	if zv == nil {
		return false, nil
	}
	if zv[0] != 0 {
		return true, nil
	}
	return false, nil
}

func (t *TypeOfBool) ID() int {
	return IDBool
}

func (t *TypeOfBool) String() string {
	return "bool"
}

func (t *TypeOfBool) Marshal(zv zcode.Bytes) (interface{}, error) {
	return DecodeBool(zv)
}

func (t *TypeOfBool) Format(zv zcode.Bytes) string {
	b, err := DecodeBool(zv)
	if err != nil {
		return badZng(err, t, zv)
	}
	if b {
		return "true"
	}
	return "false"
}
