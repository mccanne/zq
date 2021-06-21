package agg

import (
	"fmt"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

type fuse struct {
	shapes   map[*zng.TypeRecord]struct{}
	partials []zng.Value
}

func newFuse() *fuse {
	return &fuse{
		shapes: make(map[*zng.TypeRecord]struct{}),
	}
}

func (f *fuse) Consume(v zng.Value) error {
	// only works for record types, e.g., fuse(foo.x) where foo.x is a record
	typ, ok := v.Type.(*zng.TypeRecord)
	if !ok {
		return nil
	}
	f.shapes[typ] = struct{}{}
	return nil
}

func (f *fuse) Result(zctx *zson.Context) (zng.Value, error) {
	if len(f.shapes)+len(f.partials) == 0 {
		// empty input
		return zng.Value{zng.TypeNull, nil}, nil
	}
	schema, err := NewSchema(zctx)
	if err != nil {
		return zng.Value{}, err
	}
	schema.unify = true

	for _, p := range f.partials {
		typ, err := zctx.LookupByValue(p.Bytes)
		if err != nil {
			return zng.Value{}, fmt.Errorf("invalid partial value: %w", err)
		}
		recType, ok := typ.(*zng.TypeRecord)
		if !ok {
			return zng.Value{}, fmt.Errorf("unexpected partial type %s", typ)
		}
		schema.Mixin(recType)
	}
	for typ := range f.shapes {
		schema.Mixin(typ)
	}
	return zctx.LookupTypeValue(schema.Type), nil
}

func (f *fuse) ConsumeAsPartial(p zng.Value) error {
	if p.Type != zng.TypeType {
		return ErrBadValue
	}
	f.partials = append(f.partials, p)
	return nil
}

func (f *fuse) ResultAsPartial(zctx *zson.Context) (zng.Value, error) {
	return f.Result(zctx)
}
