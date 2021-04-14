package semantic

import (
	"encoding/json"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/compiler/ast/zed"
)

func semType(scope *Scope, typ zed.Type) (zed.Type, error) {
	return copyType(typ), nil
}

func copyType(t zed.Type) zed.Type {
	if t == nil {
		panic("copyType nil")
	}
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	copy, err := dag.UnpackJSON(b)
	if err != nil {
		panic(err)
	}
	typ, ok := copy.(zed.Type)
	if !ok {
		panic("copyType not a type")
	}
	return typ
}
