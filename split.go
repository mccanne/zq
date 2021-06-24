package zed

import (
	"fmt"

	"github.com/brimdata/zed/zcode"
)

func Split(elemType Type, b zcode.Bytes) ([]Value, error) {
	// We test for nil explicitly and initialize vals to a non-empty
	// slice of size 0 so that we can differentiate between a non-nil
	// but empty Z container vs a nil Z container value.
	if b == nil {
		return nil, nil
	}
	vals := []Value{}
	for it := b.Iter(); !it.Done(); {
		zv, _, err := it.Next()
		if err != nil {
			return nil, fmt.Errorf("parsing element type '%s' value %q: %w", elemType, b, err)
		}
		vals = append(vals, Value{elemType, zv})
	}
	return vals, nil
}
