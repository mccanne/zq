package zed

import (
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
)

type TypeOfDuration struct{}

func NewDuration(d nano.Duration) Value {
	return Value{TypeDuration, EncodeDuration(d)}
}

func EncodeDuration(d nano.Duration) zcode.Bytes {
	return EncodeInt(int64(d))
}

func AppendDuration(bytes zcode.Bytes, d nano.Duration) zcode.Bytes {
	return AppendInt(bytes, int64(d))
}

func DecodeDuration(zv zcode.Bytes) (nano.Duration, error) {
	i, err := DecodeInt(zv)
	return nano.Duration(i), err
}

func (t *TypeOfDuration) ID() int {
	return IDDuration
}

func (t *TypeOfDuration) String() string {
	return "duration"
}

func (t *TypeOfDuration) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Format(zv), nil
}

func (t *TypeOfDuration) Format(zv zcode.Bytes) string {
	d, err := DecodeDuration(zv)
	if err != nil {
		return badZng(err, t, zv)
	}
	return d.String()
}
