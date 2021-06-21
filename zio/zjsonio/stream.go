package zjsonio

import (
	"errors"
	"strconv"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

type Stream struct {
	zctx     *zson.Context
	encoder  encoder
	typetype map[zng.Type]bool
}

func NewStream() *Stream {
	return &Stream{
		zctx:     zson.NewContext(),
		encoder:  make(encoder),
		typetype: make(map[zng.Type]bool),
	}
}

func (s *Stream) Transform(r *zng.Record) (Object, error) {
	typ, err := s.zctx.TranslateType(r.Type)
	if err != nil {
		return Object{}, err
	}
	var types []zed.Type
	id, t := s.typeID(typ)
	if t != nil {
		types = append(types, t)
	}
	if s.hasTypeType(typ) {
		types = s.appendTypeValues(types, r.Value)
	}
	v, err := encodeValue(s.zctx, typ, r.Bytes)
	if err != nil {
		return Object{}, err
	}
	values, ok := v.([]interface{})
	if !ok {
		return Object{}, errors.New("internal error: zng record body must be a container")
	}
	return Object{
		Schema: id,
		Types:  types,
		Values: values,
	}, nil
}

func (s *Stream) typeID(typ zng.Type) (string, zed.Type) {
	if id, ok := s.encoder[typ]; ok {
		return id, nil
	}
	if zng.TypeID(typ) < zng.IDTypeDef {
		id := typ.String()
		s.encoder[typ] = id
		return id, nil
	}
	t := s.encoder.encodeType(s.zctx, typ)
	id, ok := s.encoder[typ]
	if !ok {
		id = strconv.Itoa(zng.TypeID(typ))
		s.encoder[typ] = id
		t = &zed.TypeDef{
			Kind: "typedef",
			Name: id,
			Type: t,
		}
	}
	return id, t
}

func (s *Stream) hasTypeType(typ zng.Type) bool {
	b, ok := s.typetype[typ]
	if ok {
		return b
	}
	switch t := typ.(type) {
	case *zng.TypeAlias:
		b = s.hasTypeType(t.Type)
	case *zng.TypeRecord:
		for _, col := range t.Columns {
			if s.hasTypeType(col.Type) {
				b = true
				break
			}
		}
	case *zng.TypeArray:
		b = s.hasTypeType(t.Type)
	case *zng.TypeSet:
		b = s.hasTypeType(t.Type)
	case *zng.TypeMap:
		b = s.hasTypeType(t.KeyType)
		if !b {
			b = s.hasTypeType(t.ValType)
		}
	case *zng.TypeUnion:
		for _, typ := range t.Types {
			if s.hasTypeType(typ) {
				b = true
				break
			}
		}
	case *zng.TypeEnum:
		b = s.hasTypeType(t.Type)
	case *zng.TypeOfType:
		b = true
	default:
		b = false
	}
	s.typetype[typ] = b
	return b
}

func (s *Stream) appendTypeValues(types []zed.Type, zv zng.Value) []zed.Type {
	zng.Walk(zv.Type, zv.Bytes, func(t zng.Type, bytes zcode.Bytes) error {
		typ, err := s.zctx.TranslateType(t)
		if err != nil {
			return err
		}
		if !s.typetype[typ] {
			return zng.SkipContainer
		}
		if typ == zng.TypeType {
			typ, err := s.zctx.LookupByValue(bytes)
			if err != nil {
				// this shouldn't happen
				return nil
			}
			if _, t := s.typeID(typ); t != nil {
				types = append(types, t)
			}
		}
		return nil
	})
	return types
}
