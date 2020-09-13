package zng

import (
	"errors"

	"github.com/brimsec/zq/zcode"
)

// A RecordVisitor is called for each value in a record encountered by
// Walk. If the visitor returns an error, the walk stops and that
// error will be returned to the caller of Walk(). The sole exception
// is when the visitor returns the special value SkipContainer.
type RecordVisitor func(typ Type, body zcode.Bytes) error

// SkipContainer is used as a return value from RecordVisitors to indicate
// that the container passed in the call should not be visited. It is
// not returned as an error by any function.
var SkipContainer = errors.New("skip this container")

func walkRecord(typ *TypeRecord, body zcode.Bytes, rv RecordVisitor) error {
	if body == nil {
		return nil
	}
	it := zcode.Iter(body)
	for _, col := range typ.Columns {
		if it.Done() {
			return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrMissingField}
		}
		body, container, err := it.Next()
		if err != nil {
			return err
		}
		switch t := AliasedType(col.Type).(type) {
		case *TypeRecord:
			if !container {
				return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkRecord(t, body, rv); err != nil {
				return err
			}
		case *TypeArray:
			if !container {
				return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkVector(t, body, rv); err != nil {
				return err
			}
		case *TypeSet:
			if !container {
				return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkSet(t, body, rv); err != nil {
				return err
			}
		case *TypeUnion:
			if !container {
				return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkUnion(t, body, rv); err != nil {
				return err
			}
		default:
			if container {
				return &RecordTypeError{Name: col.Name, Type: col.Type.String(), Err: ErrNotPrimitive}
			}
			if err := rv(t, body); err != nil && err != SkipContainer {
				return err
			}
		}
	}
	return nil
}

func walkVector(typ *TypeArray, body zcode.Bytes, rv RecordVisitor) error {
	if body == nil {
		return nil
	}
	inner := InnerType(AliasedType(typ))
	it := zcode.Iter(body)
	for !it.Done() {
		body, container, err := it.Next()
		if err != nil {
			return err
		}
		switch t := inner.(type) {
		case *TypeRecord:
			if !container {
				return &RecordTypeError{Name: "<record element>", Type: t.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkRecord(t, body, rv); err != nil {
				return err
			}
		case *TypeArray:
			if !container {
				return &RecordTypeError{Name: "<array element>", Type: t.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkVector(t, body, rv); err != nil {
				return err
			}
		case *TypeSet:
			if !container {
				return &RecordTypeError{Name: "<set element>", Type: t.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkSet(t, body, rv); err != nil {
				return err
			}
		case *TypeUnion:
			if !container {
				return &RecordTypeError{Name: "<union value>", Type: t.String(), Err: ErrNotContainer}
			}
			if err := rv(t, body); err != nil {
				if err == SkipContainer {
					continue
				}
				return err
			}
			if err := walkUnion(t, body, rv); err != nil {
				return err
			}
		default:
			if container {
				return &RecordTypeError{Name: "<array element>", Type: t.String(), Err: ErrNotPrimitive}
			}
			if err := rv(t, body); err != nil && err != SkipContainer {
				return err
			}
		}
	}
	return nil
}

func walkUnion(typ *TypeUnion, body zcode.Bytes, rv RecordVisitor) error {
	if len(body) == 0 {
		return nil
	}
	it := zcode.Iter(body)
	v, container, err := it.Next()
	if err != nil {
		return err
	}
	if container {
		return ErrBadValue
	}
	index, err := DecodeInt(v)
	if err != nil {
		return err
	}
	inner, err := typ.TypeIndex(int(index))
	if err != nil {
		return err
	}
	body, container, err = it.Next()
	if err != nil {
		return err
	}
	switch t := AliasedType(inner).(type) {
	case *TypeRecord:
		if !container {
			return &RecordTypeError{Name: "<record element>", Type: t.String(), Err: ErrNotContainer}
		}

		if err := walkRecord(t, body, rv); err != nil {
			return err
		}
	case *TypeArray:
		if !container {
			return &RecordTypeError{Name: "<array element>", Type: t.String(), Err: ErrNotContainer}
		}
		if err := rv(t, body); err != nil {
			if err == SkipContainer {
				return nil
			}
			return err
		}

		if err := walkVector(t, body, rv); err != nil {
			return err
		}
	case *TypeSet:
		if !container {
			return &RecordTypeError{Name: "<set element>", Type: t.String(), Err: ErrNotContainer}
		}
		if err := rv(t, body); err != nil {
			if err == SkipContainer {
				return nil
			}
			return err
		}

		if err := walkSet(t, body, rv); err != nil {
			return err
		}
	case *TypeUnion:
		if !container {
			return &RecordTypeError{Name: "<union value>", Type: t.String(), Err: ErrNotContainer}
		}
		if err := rv(t, body); err != nil {
			if err == SkipContainer {
				return nil
			}
			return err
		}
		if err := walkUnion(t, body, rv); err != nil {
			return err
		}
	default:
		if container {
			return &RecordTypeError{Name: "<union value>", Type: t.String(), Err: ErrNotPrimitive}
		}
		if err := rv(t, body); err != nil && err != SkipContainer {
			return err
		}
	}
	return nil
}

func walkSet(typ *TypeSet, body zcode.Bytes, rv RecordVisitor) error {
	if body == nil {
		return nil
	}
	inner := AliasedType(InnerType(typ))
	innerContainer := IsContainerType(inner)
	it := zcode.Iter(body)
	for !it.Done() {
		body, container, err := it.Next()
		if err != nil {
			return err
		}
		if container != innerContainer {
			return &RecordTypeError{Name: "<set element>", Type: typ.String(), Err: ErrTypeMismatch}
		}
		if err := rv(inner, body); err != nil && err != SkipContainer {
			return err
		}
	}
	return nil
}
