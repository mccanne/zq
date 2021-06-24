package zed

import (
	"time"

	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
)

type TypeOfTime struct{}

func NewTime(ts nano.Ts) Value {
	return Value{TypeTime, EncodeTime(ts)}
}

func EncodeTime(t nano.Ts) zcode.Bytes {
	var b [8]byte
	n := zcode.EncodeCountedVarint(b[:], int64(t))
	return b[:n]
}

func AppendTime(bytes zcode.Bytes, t nano.Ts) zcode.Bytes {
	return AppendInt(bytes, int64(t))
}

func DecodeTime(zv zcode.Bytes) (nano.Ts, error) {
	if zv == nil {
		return 0, nil
	}
	return nano.Ts(zcode.DecodeCountedVarint(zv)), nil
}

func (t *TypeOfTime) ID() int {
	return IDTime
}

func (t *TypeOfTime) String() string {
	return "time"
}

func (t *TypeOfTime) Marshal(zv zcode.Bytes) (interface{}, error) {
	ts, err := DecodeTime(zv)
	if err != nil {
		return nil, err
	}
	return ts.Time().Format(time.RFC3339Nano), nil
}

func (t *TypeOfTime) Format(zv zcode.Bytes) string {
	ts, err := DecodeTime(zv)
	if err != nil {
		return badZng(err, t, zv)
	}
	b := ts.Time().Format(time.RFC3339Nano)
	return string(b)
}
