package reducer

import (
	"github.com/mccanne/zq/pkg/zeek"
	"github.com/mccanne/zq/pkg/zq"
)

type LastProto struct {
	target string
	field  string
}

func (lp *LastProto) Target() string {
	return lp.target
}

func (lp *LastProto) Instantiate() Interface {
	return &Last{Field: lp.field}
}

func NewLastProto(target, field string) *LastProto {
	return &LastProto{target, field}
}

type Last struct {
	Reducer
	Field  string
	record *zq.Record
}

func (l *Last) Consume(r *zq.Record) {
	if _, ok := r.ColumnOfField(l.Field); !ok {
		return
	}
	l.record = r
}

func (l *Last) Result() zeek.Value {
	r := l.record
	if r == nil {
		return &zeek.Unset{}
	}
	return r.ValueByField(l.Field)
}
