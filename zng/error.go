package zng

import (
	"errors"

	"github.com/brimsec/zq/zcode"
)

type TypeOfError struct{}

func NewError(err error) Value {
	return Value{TypeError, zcode.Bytes(err.Error())}
}

func EncodeError(err error) zcode.Bytes {
	return zcode.Bytes(err.Error())
}

func DecodeError(zv zcode.Bytes) (error, error) {
	if zv == nil {
		return nil, ErrUnset
	}
	return errors.New(string(zv)), nil
}

func (t *TypeOfError) Parse(in []byte) (zcode.Bytes, error) {
	return zcode.Bytes(in), nil
}

func (t *TypeOfError) ID() int {
	return IdError
}

func (t *TypeOfError) String() string {
	return "error"
}

func (t *TypeOfError) StringOf(zv zcode.Bytes, fmt OutFmt, inContainer bool) string {
	return string(zv)
}

func (t *TypeOfError) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.StringOf(zv, OutFormatUnescaped, false), nil
}
