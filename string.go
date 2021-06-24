package zed

import (
	"github.com/brimdata/zed/zcode"
)

type TypeOfString struct{}

func NewString(s string) Value {
	return Value{TypeString, EncodeString(s)}
}

func EncodeString(s string) zcode.Bytes {
	return zcode.Bytes(s)
}

func DecodeString(zv zcode.Bytes) (string, error) {
	return string(zv), nil
}

func (t *TypeOfString) ID() int {
	return IDString
}

func (t *TypeOfString) String() string {
	return "string"
}

func (t *TypeOfString) Marshal(zv zcode.Bytes) (interface{}, error) {
	return string(zv), nil
}

func (t *TypeOfString) Format(zv zcode.Bytes) string {
	return QuotedString(zv, false)
}
