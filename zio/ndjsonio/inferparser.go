package ndjsonio

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/byteconv"
	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/builder"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/buger/jsonparser"
)

type inferParser struct {
	zctx *resolver.Context
}

func (p *inferParser) parseObject(b []byte) (zng.Value, error) {
	type kv struct {
		key   []byte
		value []byte
		typ   jsonparser.ValueType
	}
	var kvs []kv
	err := jsonparser.ObjectEach(b, func(key []byte, value []byte, typ jsonparser.ValueType, offset int) error {
		kvs = append(kvs, kv{key, value, typ})
		return nil
	})
	if err != nil {
		return zng.Value{}, err
	}
	if len(kvs) == 0 {
		empty, err := p.zctx.LookupTypeRecord([]zng.Column{})
		if err != nil {
			return zng.Value{}, err
		}
		return zng.Value{Type: empty, Bytes: zcode.Bytes{}}, nil
	}

	// Sort fields lexigraphically ensuring maps with the same
	// columns but different printed order get assigned the same descriptor.
	sort.Slice(kvs, func(i, j int) bool {
		return bytes.Compare(kvs[i].key, kvs[j].key) < 0
	})
	var fields []field.Static
	var zngTypes []zng.Type
	var zngValues []zng.Value
	for _, kv := range kvs {
		fields = append(fields, field.Dotted(string(kv.key)))
		v, err := p.parseValue(kv.value, kv.typ)
		if err != nil {
			return zng.Value{}, err
		}
		zngTypes = append(zngTypes, v.Type)
		zngValues = append(zngValues, v)
	}
	columnBuilder, err := builder.NewColumnBuilder(p.zctx, fields)
	if err != nil {
		return zng.Value{}, err
	}
	typ, err := p.zctx.LookupTypeRecord(columnBuilder.TypedColumns(zngTypes))
	if err != nil {
		return zng.Value{}, err
	}
	for _, v := range zngValues {
		columnBuilder.Append(v.Bytes, zng.IsContainerType(zng.AliasedType(v.Type)))
	}
	zbytes, err := columnBuilder.Encode()
	if err != nil {
		return zng.Value{}, err
	}
	return zng.Value{Type: typ, Bytes: zbytes}, nil
}

func (p *inferParser) parseValue(raw []byte, typ jsonparser.ValueType) (zng.Value, error) {
	switch typ {
	case jsonparser.Array:
		return p.parseArray(raw)
	case jsonparser.Object:
		return p.parseObject(raw)
	case jsonparser.Boolean:
		return p.parseBool(raw)
	case jsonparser.Number:
		return p.parseNumber(raw)
	case jsonparser.Null:
		return p.parseNull()
	case jsonparser.String:
		return p.parseString(raw)
	default:
		return zng.Value{}, fmt.Errorf("unsupported type %v", typ)
	}
}

func typeIndex(typs []zng.Type, typ zng.Type) int {
	for i := range typs {
		if typ == typs[i] {
			return i
		}
	}
	return -1
}

func (p *inferParser) unionType(vals []zng.Value) *zng.TypeUnion {
	var typs []zng.Type
	for i := range vals {
		if index := typeIndex(typs, vals[i].Type); index == -1 {
			typs = append(typs, vals[i].Type)
		}
	}
	if len(typs) <= 1 {
		return nil
	}
	return p.zctx.LookupTypeUnion(typs)
}

func encodeUnionArray(typ *zng.TypeUnion, vals []zng.Value) zcode.Bytes {
	var b zcode.Builder
	for i := range vals {
		b.BeginContainer()
		index := typeIndex(typ.Types, vals[i].Type)
		b.AppendPrimitive(zng.EncodeInt(int64(index)))
		if zng.IsContainerType(vals[i].Type) {
			b.AppendContainer(vals[i].Bytes)
		} else {
			b.AppendPrimitive(vals[i].Bytes)
		}
		b.EndContainer()
	}
	return b.Bytes()
}

func encodeContainer(vals []zng.Value) zcode.Bytes {
	b := zcode.Bytes{}
	for i := range vals {
		if zng.IsContainerType(vals[i].Type) {
			b = zcode.AppendContainer(b, vals[i].Bytes)
		} else {
			b = zcode.AppendPrimitive(b, vals[i].Bytes)
		}
	}
	return b
}

func (p *inferParser) parseArray(raw []byte) (zng.Value, error) {
	var err error
	var vals []zng.Value
	jsonparser.ArrayEach(raw, func(el []byte, typ jsonparser.ValueType, offset int, elErr error) {
		if elErr != nil {
			err = elErr
			return
		}
		val, err := p.parseValue(el, typ)
		if err != nil {
			return
		}
		vals = append(vals, val)
	})
	if err != nil {
		return zng.Value{}, err
	}
	union := p.unionType(vals)
	if union != nil {
		typ := p.zctx.LookupTypeArray(union)
		return zng.Value{typ, encodeUnionArray(union, vals)}, nil
	}
	var typ zng.Type
	if len(vals) == 0 {
		typ = p.zctx.LookupTypeArray(zng.TypeString)
	} else {
		typ = p.zctx.LookupTypeArray(vals[0].Type)
	}
	return zng.Value{typ, encodeContainer(vals)}, nil
}

func (p *inferParser) parseBool(b []byte) (zng.Value, error) {
	boolean, err := jsonparser.GetBoolean(b)
	if err != nil {
		return zng.Value{}, err
	}
	return zng.NewBool(boolean), nil
}

func (p *inferParser) parseNumber(b []byte) (zng.Value, error) {
	d, err := byteconv.ParseFloat64(b)
	if err != nil {
		return zng.Value{}, err
	}
	return zng.NewFloat64(d), nil
}

func (p *inferParser) parseString(b []byte) (zng.Value, error) {
	b, err := jsonparser.Unescape(b, nil)
	if err != nil {
		return zng.Value{}, err
	}
	s, err := zng.TypeString.Parse(b)
	if err != nil {
		return zng.Value{}, err
	}
	return zng.Value{zng.TypeString, s}, nil
}

func (p *inferParser) parseNull() (zng.Value, error) {
	return zng.Value{zng.TypeNull, nil}, nil
}
