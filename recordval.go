package zed

import (
	"bytes"
	"errors"
	"math"
	"net"

	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
)

var (
	ErrMissingField   = errors.New("record missing a field")
	ErrExtraField     = errors.New("record with extra field")
	ErrNotContainer   = errors.New("expected container type, got primitive")
	ErrNotPrimitive   = errors.New("expected primitive type, got container")
	ErrTypeIDExists   = errors.New("zng type ID exists")
	ErrTypeIDInvalid  = errors.New("zng type ID out of range")
	ErrBadValue       = errors.New("malformed zng value")
	ErrBadFormat      = errors.New("malformed zng record")
	ErrTypeMismatch   = errors.New("type/value mismatch")
	ErrColumnMismatch = errors.New("zng record mismatch between columns in type and columns in value")
	ErrCorruptColumns = errors.New("wrong number of columns in zng record value")
)

type RecordTypeError struct {
	Name string
	Type string
	Err  error
}

func (r *RecordTypeError) Error() string { return r.Name + " (" + r.Type + "): " + r.Err.Error() }
func (r *RecordTypeError) Unwrap() error { return r.Err }

// A Record wraps a zng.Value and provides helper methods for accessing
// and iterating over the record's fields.
type Record struct {
	Value
	nonvolatile bool
	ts          nano.Ts
	tsValid     bool
}

func NewRecord(typ Type, bytes zcode.Bytes) *Record {
	return &Record{
		Value:       Value{typ, bytes},
		nonvolatile: true,
	}
}

func NewRecordCheck(typ Type, bytes zcode.Bytes) (*Record, error) {
	r := NewRecord(typ, bytes)
	if err := r.TypeCheck(); err != nil {
		return nil, err
	}
	return r, nil
}

// NewVolatileRecord creates a record from a zcode.Bytes and marks
// it volatile so that Keep() must be called to make it safe.
// This is useful for readers that allocate records whose Bytes field points
// into a reusable buffer allowing the scanner to filter these records
// without having the Bytes buffer copied to safe memory, i.e., when the scanner
// matches a record, it will call Keep() to make a safe copy.
func NewVolatileRecord(typ Type, bytes zcode.Bytes) *Record {
	return &Record{
		Value: Value{typ, bytes},
	}
}

// ZvalIter returns a zcode.Iter iterator over the receiver's values.
func (r *Record) ZvalIter() zcode.Iter {
	return r.Bytes.Iter()
}

// FieldIter returns a fieldIter iterator over the receiver's values.
func (r *Record) FieldIter() fieldIter {
	return fieldIter{
		stack: []iterInfo{{
			iter: r.ZvalIter(),
			typ:  TypeRecordOf(r.Type),
		}},
	}
}

func (r *Record) Keep() *Record {
	if r.nonvolatile {
		return r
	}
	bytes := make(zcode.Bytes, len(r.Bytes))
	copy(bytes, r.Bytes)
	return &Record{
		Value:       Value{r.Type, bytes},
		nonvolatile: true,
		ts:          r.ts,
		tsValid:     r.tsValid,
	}
}

func (r *Record) CopyBytes() {
	if r.nonvolatile {
		return
	}
	bytes := make(zcode.Bytes, len(r.Bytes))
	copy(bytes, r.Bytes)
	r.Bytes = bytes
	r.nonvolatile = true
}

func (r *Record) HasField(field string) bool {
	return TypeRecordOf(r.Type).HasField(field)
}

// Walk traverses a record in depth-first order, calling a
// RecordVisitor on the way.
func (r *Record) Walk(rv Visitor) error {
	return walkRecord(TypeRecordOf(r.Type), r.Bytes, rv)
}

// TypeCheck checks that the Bytes field is structurally consistent
// with this value's Type.  It does not check that the actual leaf
// values when parsed are type compatible with the leaf types.
func (r *Record) TypeCheck() error {
	return r.Walk(func(typ Type, body zcode.Bytes) error {
		if typset, ok := typ.(*TypeSet); ok {
			if err := checkSet(typset, body); err != nil {
				return err
			}
			return SkipContainer
		}
		if typ, ok := typ.(*TypeEnum); ok {
			if err := checkEnum(typ, body); err != nil {
				return err
			}
			return SkipContainer
		}
		return nil
	})
}

func checkSet(typ *TypeSet, body zcode.Bytes) error {
	if body == nil {
		return nil
	}
	it := body.Iter()
	var prev zcode.Bytes
	for !it.Done() {
		tagAndBody, _, err := it.NextTagAndBody()
		if err != nil {
			return err
		}
		if prev != nil {
			switch bytes.Compare(prev, tagAndBody) {
			case 0:
				err := errors.New("duplicate element")
				return &RecordTypeError{Name: "<set element>", Type: typ.String(), Err: err}
			case 1:
				err := errors.New("elements not sorted")
				return &RecordTypeError{Name: "<set element>", Type: typ.String(), Err: err}
			}
		}
		prev = tagAndBody
	}
	return nil
}

func checkEnum(typ *TypeEnum, body zcode.Bytes) error {
	if body == nil {
		return nil
	}
	selector, err := DecodeUint(body)
	if err != nil {
		return err
	}
	if int(selector) >= len(typ.Symbols) {
		return errors.New("enum selector out of range")
	}
	return nil
}

// Slice returns the encoded zcode.Bytes corresponding to the indicated
// column or an error if a problem was encountered.  If the encoded bytes
// result is nil without error, then that columnn is unset in this record value.
func (r *Record) Slice(column int) (zcode.Bytes, error) {
	var zv zcode.Bytes
	for i, it := 0, r.Bytes.Iter(); i <= column; i++ {
		if it.Done() {
			return nil, ErrMissing
		}
		var err error
		zv, _, err = it.Next()
		if err != nil {
			return nil, err
		}
	}
	return zv, nil
}

func (r *Record) Columns() []Column {
	return TypeRecordOf(r.Type).Columns
}

// Value returns the indicated column as a Value.  If the column doesn't
// exist or another error occurs, the nil Value is returned.
func (r *Record) ValueByColumn(col int) Value {
	zv, err := r.Slice(col)
	if err != nil {
		return Value{}
	}
	return Value{r.Columns()[col].Type, zv}
}

func (r *Record) ValueByField(field string) (Value, error) {
	col, ok := r.ColumnOfField(field)
	if !ok {
		return Value{}, ErrMissing
	}
	return r.ValueByColumn(col), nil
}

func (r *Record) ColumnOfField(field string) (int, bool) {
	return TypeRecordOf(r.Type).ColumnOfField(field)
}

func (r *Record) TypeOfColumn(col int) Type {
	return TypeRecordOf(r.Type).Columns[col].Type
}

func (r *Record) Access(field string) (Value, error) {
	col, ok := r.ColumnOfField(field)
	if !ok {
		return Value{}, ErrMissing
	}
	return r.ValueByColumn(col), nil
}

func (r *Record) Deref(path field.Path) (Value, error) {
	v := r.Value
	for _, f := range path {
		typ := TypeRecordOf(v.Type)
		if typ == nil {
			return Value{}, errors.New("field access on non-record value")
		}
		var err error
		v, err = NewVolatileRecord(typ, v.Bytes).Access(f)
		if err != nil {
			return Value{}, err
		}
	}
	return v, nil
}

func (r *Record) AccessString(field string) (string, error) {
	v, err := r.Access(field)
	if err != nil {
		return "", err
	}
	switch AliasOf(v.Type).(type) {
	case *TypeOfString, *TypeOfBstring:
		return DecodeString(v.Bytes)
	default:
		return "", ErrTypeMismatch
	}
}

func (r *Record) AccessBool(field string) (bool, error) {
	v, err := r.Access(field)
	if err != nil {
		return false, err
	}
	if _, ok := AliasOf(v.Type).(*TypeOfBool); !ok {
		return false, ErrTypeMismatch
	}
	return DecodeBool(v.Bytes)
}

func (r *Record) AccessInt(field string) (int64, error) {
	v, err := r.Access(field)
	if err != nil {
		return 0, err
	}
	switch AliasOf(v.Type).(type) {
	case *TypeOfUint8:
		b, err := DecodeUint(v.Bytes)
		return int64(b), err
	case *TypeOfInt16, *TypeOfInt32, *TypeOfInt64:
		return DecodeInt(v.Bytes)
	case *TypeOfUint16, *TypeOfUint32:
		v, err := DecodeUint(v.Bytes)
		return int64(v), err
	case *TypeOfUint64:
		v, err := DecodeUint(v.Bytes)
		if v > math.MaxInt64 {
			return 0, errors.New("conversion from uint64 to signed int results in overflow")
		}
		return int64(v), err
	}
	return 0, ErrTypeMismatch
}

func (r *Record) AccessIP(field string) (net.IP, error) {
	v, err := r.Access(field)
	if err != nil {
		return nil, err
	}
	if _, ok := AliasOf(v.Type).(*TypeOfIP); !ok {
		return nil, ErrTypeMismatch
	}
	return DecodeIP(v.Bytes)
}

func (r *Record) AccessTime(field string) (nano.Ts, error) {
	v, err := r.Access(field)
	if err != nil {
		return 0, err
	}
	if _, ok := AliasOf(v.Type).(*TypeOfTime); !ok {
		return 0, ErrTypeMismatch
	}
	return DecodeTime(v.Bytes)
}

func (r *Record) AccessTimeByColumn(colno int) (nano.Ts, error) {
	zv, err := r.Slice(colno)
	if err != nil {
		return 0, err
	}
	return DecodeTime(zv)
}

// Ts returns the value of the receiver's "ts" field.  If the field is absent,
// is null, or has a type other than TypeOfTime, Ts returns nano.MinTs.
func (r *Record) Ts() nano.Ts {
	if !r.tsValid {
		r.ts, _ = r.AccessTime("ts")
		r.tsValid = true
	}
	return r.ts
}

func (r *Record) String() string {
	return r.Value.String()
}

// MarshalJSON implements json.Marshaler.
func (r *Record) MarshalJSON() ([]byte, error) {
	return r.Value.MarshalJSON()
}
