package zson

import (
	"errors"

	"github.com/brimdata/zed/zng"
)

var (
	ErrAliasExists = errors.New("alias exists with different type")
)

// A Context manages the mapping between small-integer descriptor identifiers
// and zng descriptor objects, which hold the binding between an identifier
// and a zng.Type.
type Context struct {
	*zng.Context
}

func NewContext() *Context {
	return &Context{zng.NewContext()}
}
