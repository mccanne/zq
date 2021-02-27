package semantic

import (
	"encoding/json"

	"github.com/brimsec/zq/compiler/ast"
)

func semType(scope *Scope, typ ast.Type) (ast.Type, error) {
	return copyType(typ), nil
}

func copyType(t ast.Type) ast.Type {
	if t == nil {
		panic("copyType nil")
	}
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	copy, err := ast.UnpackJSON(b)
	if err != nil {
		panic(err)
	}
	typ, ok := copy.(ast.Type)
	if !ok {
		panic("copyType not a type")
	}
	return typ
}
