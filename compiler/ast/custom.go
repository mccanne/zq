package ast

import (
	"github.com/brimdata/zed/field"
)

// These ast types are to support the custom procs used by the indexer.
// We should have the indexer use genric components and figure out what
// custom functionality should be transferred into the Z core.  This will
// allow us to get rid of the custom hook that originally went in for
// reasons that are no longer relevant.  Having the definitions here allows
// the semantic pass to handle them properly.  See issue #2258.

type FieldCutter struct {
	Field field.Static
	Out   field.Static
}

func (*FieldCutter) ProcAST() {}

type TypeSplitter struct {
	Key      field.Static
	TypeName string
}

func (t *TypeSplitter) ProcAST() {}
