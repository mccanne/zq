package expr

import (
	"github.com/brimdata/zed/zng"
)

type Literal struct {
	zv zng.Value
}

func NewLiteral(zv zng.Value) *Literal {
	return &Literal{zv}
}

func (l *Literal) Eval(*zng.Record) (zng.Value, error) {
	return l.zv, nil
}
