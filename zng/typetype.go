package zng

import (
	"github.com/brimsec/zq/zcode"
)

type TypeOfType struct{}

func NewType(s string) Value {
	return Value{TypeType, EncodeString(s)}
}

func EncodeType(s string) zcode.Bytes {
	return zcode.Bytes(s)
}

func DecodeType(zv zcode.Bytes) (string, error) {
	if zv == nil {
		return "", ErrUnset
	}
	return string(zv), nil
}

func (t *TypeOfType) Parse(in []byte) (zcode.Bytes, error) {
	return zcode.Bytes(in), nil
}

func (t *TypeOfType) ID() int {
	return IdType
}

func (t *TypeOfType) String() string {
	return "type"
}

func (t *TypeOfType) StringOf(zv zcode.Bytes, fmt OutFmt, inContainer bool) string {
	return string(zv)
}

func (t *TypeOfType) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.StringOf(zv, OutFormatUnescaped, false), nil
}
