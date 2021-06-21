package index

import (
	"strings"

	"github.com/brimdata/zed/zio/tzngio"
	"github.com/brimdata/zed/zqe"
	"github.com/brimdata/zed/zson"
	"github.com/segmentio/ksuid"
)

type DefLookup struct {
	DefID  ksuid.KSUID
	Values []string
}

type Query struct {
	Name   string
	Field  string
	Type   string
	Values []string
}

func ParseQuery(name string, patterns []string) (Query, error) {
	if len(patterns) == 0 {
		return Query{}, zqe.E(zqe.Invalid, "no search patterns")
	}
	if name != "" {
		return Query{
			Name:   name,
			Values: patterns,
		}, nil
	}
	if len(patterns) != 1 {
		return Query{}, zqe.E(zqe.Invalid, "standard index supports exactly one search pattern")
	}
	in := patterns[0]

	v := strings.Split(in, "=")
	if len(v) != 2 {
		return Query{}, zqe.E(zqe.Invalid, "malformed standard index query")
	}
	q := Query{Values: []string{v[1]}}
	path := v[0]
	if path[0] == ':' {
		typ, err := zson.ParseType(zson.NewContext(), path[1:])
		if err != nil {
			return Query{}, err
		}
		//XXX this should be a serialized type value and tzngio should go
		q.Type = tzngio.TypeString(typ)
	} else {
		q.Field = path
	}
	return q, nil
}

func (q Query) Matches(r Index) bool {
	switch r.Kind {
	case IndexZed:
		return q.Name == r.Name
	case IndexType:
		return q.Type == r.Value
	case IndexField:
		return q.Field == r.Value
	}
	return false
}
