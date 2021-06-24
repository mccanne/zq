package zed

import (
	"errors"
	"fmt"

	"github.com/brimdata/zed/zcode"
)

// ErrMissing is returned by entities that fail because a referenced field
// was missing or because an argument to the entity had a missing value.
// This is used at sites in the code where it is unknown whether the outcome
// should result in a runtime exit or in continued execution with a
// Missing value embedded in the Z results.
var ErrMissing = errors.New(missing)

const missing = "missing"

// Missing is value that represents the error condition that a field
// referenced was not present.  The Missing value can be propagated through
// functions and expressions and each operator must clearly defined its
// semantics with respect to the Missing value.  For example, "true AND MISSING"
// is MISSING.
var Missing = NewError(ErrMissing)

type TypeOfError struct{}

func NewErrorf(format string, args ...interface{}) Value {
	msg := fmt.Sprintf(format, args...)
	return Value{TypeError, zcode.Bytes(msg)}
}

func NewError(err error) Value {
	return Value{TypeError, zcode.Bytes(err.Error())}
}

func EncodeError(err error) zcode.Bytes {
	return zcode.Bytes(err.Error())
}

func DecodeError(zv zcode.Bytes) (error, error) {
	if zv == nil {
		return nil, nil
	}
	return errors.New(string(zv)), nil
}

func (t *TypeOfError) ID() int {
	return IDError
}

func (t *TypeOfError) String() string {
	return "error"
}

func (t *TypeOfError) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Format(zv), nil
}

func (t *TypeOfError) Format(zv zcode.Bytes) string {
	return QuotedString(zv, false)
}

func IsMissing(zv Value) bool {
	return zv.Type == TypeError && string(zv.Bytes) == missing
}
