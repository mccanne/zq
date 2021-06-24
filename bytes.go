package zed

import (
	"encoding/hex"

	"github.com/brimdata/zed/zcode"
)

type TypeOfBytes struct{}

func NewBytes(b []byte) Value {
	return Value{TypeBytes, EncodeBytes(b)}
}

func EncodeBytes(b []byte) zcode.Bytes {
	return zcode.Bytes(b)
}

func DecodeBytes(zv zcode.Bytes) ([]byte, error) {
	return []byte(zv), nil
}

func (t *TypeOfBytes) ID() int {
	return IDBytes
}

func (t *TypeOfBytes) String() string {
	return "bytes"
}

func (t *TypeOfBytes) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Format(zv), nil
}

func (t *TypeOfBytes) Format(zv zcode.Bytes) string {
	return "0x" + hex.EncodeToString(zv)
}
