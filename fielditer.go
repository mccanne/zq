package zed

import (
	"errors"

	"github.com/brimdata/zed/zcode"
)

var (
	ErrExhausted = errors.New("called Next() on iterator after last record")
	ErrMismatch  = errors.New("mismatch between record type and value")
)

type iterInfo struct {
	iter   zcode.Iter
	typ    *TypeRecord
	offset int
	field  []string
}

type fieldIter struct {
	stack []iterInfo
}

func (r *fieldIter) Done() bool {
	return len(r.stack) == 0
}

func (r *fieldIter) Next() ([]string, Value, error) {
	if len(r.stack) == 0 {
		return nil, Value{}, ErrExhausted
	}
	info := &r.stack[len(r.stack)-1]

	zv, container, err := info.iter.Next()
	if err != nil {
		return nil, Value{}, err
	}

	col := info.typ.Columns[info.offset]
	fullname := append(info.field, col.Name)

	recType, isRecord := AliasOf(col.Type).(*TypeRecord)
	if isRecord {
		if !container {
			return nil, Value{}, ErrMismatch
		}
		r.stack = append(r.stack, iterInfo{zv.Iter(), recType, 0, fullname})
		return r.Next()
	}

	// we're at a leaf value, assemble it
	val := Value{col.Type, zv}

	// and advance our position, stepping out of records as needed.
	info.offset++
	for info.offset >= len(info.typ.Columns) {
		if !info.iter.Done() {
			return nil, Value{}, ErrMismatch
		}
		r.stack = r.stack[:len(r.stack)-1]
		if len(r.stack) == 0 {
			break
		}
		info = &r.stack[len(r.stack)-1]
		info.offset++
	}

	return fullname, val, nil
}
