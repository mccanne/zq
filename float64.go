package zed

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/brimdata/zed/zcode"
)

type TypeOfFloat64 struct{}

func NewFloat64(f float64) Value {
	return Value{TypeFloat64, EncodeFloat64(f)}
}

func AppendFloat64(zb zcode.Bytes, d float64) zcode.Bytes {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(d))
	return append(zb, buf...)
}

func EncodeFloat64(d float64) zcode.Bytes {
	return AppendFloat64(nil, d)
}

func DecodeFloat64(zv zcode.Bytes) (float64, error) {
	if len(zv) != 8 {
		return 0, errors.New("byte encoding of double not 8 bytes")
	}
	bits := binary.LittleEndian.Uint64(zv)
	return math.Float64frombits(bits), nil
}

func (t *TypeOfFloat64) ID() int {
	return IDFloat64
}

func (t *TypeOfFloat64) String() string {
	return "float64"
}

func (t *TypeOfFloat64) Marshal(zv zcode.Bytes) (interface{}, error) {
	return DecodeFloat64(zv)
}

func (t *TypeOfFloat64) Format(zv zcode.Bytes) string {
	d, err := DecodeFloat64(zv)
	if err != nil {
		return badZng(err, t, zv)
	}
	if d == float64(int64(d)) {
		return fmt.Sprintf("%d.", int64(d))
	}
	return strconv.FormatFloat(d, 'g', -1, 64)
}
