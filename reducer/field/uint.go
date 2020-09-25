package field

import (
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/streamfn"
	"github.com/brimsec/zq/zng"
)

type Uint struct {
	fn *streamfn.Uint64
}

func NewUintStreamfn(op string) Streamfn {
	return &Uint{
		fn: streamfn.NewUint64(op),
	}
}

func (u *Uint) Result() zng.Value {
	return zng.NewUint64(u.fn.State)
}

func (u *Uint) Consume(v zng.Value) error {
	if v, ok := expr.CoerceToUint(v); ok {
		u.fn.Update(v)
		return nil
	}
	return zng.ErrTypeMismatch
}
