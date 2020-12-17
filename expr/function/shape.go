package function

import (
	"github.com/brimsec/zq/expr/result"
	"github.com/brimsec/zq/zng"
)

type reshape struct {
	result.Buffer
}

func (r *reshape) Call(args []zng.Value) (zng.Value, error) {
	// arg 0 is the value
	// check it's type record, error if not
	// arg 1 is a type
	// check it's type typ, error if not
	// check it's a type value of record type, error if not

	return zng.Value{}, nil
}
