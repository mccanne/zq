package zjsonio

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

func encodeUnion(zctx *zson.Context, union *zng.TypeUnion, bytes zcode.Bytes) (interface{}, error) {
	// encode nil val as JSON null since
	// zng.Escape() returns "" for nil
	if bytes == nil {
		return nil, nil
	}
	inner, index, b, err := union.SplitZng(bytes)
	if err != nil {
		return nil, err
	}
	val, err := encodeValue(zctx, inner, b)
	if err != nil {
		return nil, err
	}
	return []interface{}{strconv.Itoa(int(index)), val}, nil
}

func encodeMap(zctx *zson.Context, typ *zng.TypeMap, v zcode.Bytes) (interface{}, error) {
	// encode nil val as JSON null since
	// zng.Escape() returns "" for nil
	if v == nil {
		return nil, nil
	}
	var out []interface{}
	it := zcode.Bytes(v).Iter()
	for !it.Done() {
		key, _, err := it.Next()
		if err != nil {
			return nil, err
		}
		pair := make([]interface{}, 2)
		pair[0], err = encodeValue(zctx, typ.KeyType, key)
		if err != nil {
			return nil, err
		}
		val, _, err := it.Next()
		if err != nil {
			return nil, err
		}
		pair[1], err = encodeValue(zctx, typ.ValType, val)
		if err != nil {
			return nil, err
		}
		out = append(out, pair)
	}
	return out, nil
}

func encodePrimitive(zctx *zson.Context, typ zng.Type, v zcode.Bytes) (interface{}, error) {
	// encode nil val as JSON null since
	// zng.Escape() returns "" for nil
	var fld interface{}
	if v == nil {
		return fld, nil
	}
	if typ == zng.TypeType {
		typ, err := zctx.LookupByValue(v)
		if err != nil {
			return nil, err
		}
		if zng.TypeID(typ) < zng.IDTypeDef {
			return typ.String(), nil
		}
		if alias, ok := typ.(*zng.TypeAlias); ok {
			return alias.Name, nil
		}
		return strconv.Itoa(zng.TypeID(typ)), nil
	}
	if zng.IsStringy(typ.ID()) {
		return string(v), nil
	}
	return typ.Format(v), nil
}

func encodeValue(zctx *zson.Context, typ zng.Type, val zcode.Bytes) (interface{}, error) {
	switch typ := typ.(type) {
	case *zng.TypeAlias:
		return encodeValue(zctx, typ.Type, val)
	case *zng.TypeUnion:
		return encodeUnion(zctx, typ, val)
	case *zng.TypeEnum:
		return encodePrimitive(zctx, zng.TypeUint64, val)
	case *zng.TypeRecord:
		return encodeRecord(zctx, typ, val)
	case *zng.TypeArray:
		return encodeContainer(zctx, typ.Type, val)
	case *zng.TypeSet:
		return encodeContainer(zctx, typ.Type, val)
	case *zng.TypeMap:
		return encodeMap(zctx, typ, val)
	default:
		return encodePrimitive(zctx, typ, val)
	}
}

func encodeRecord(zctx *zson.Context, typ *zng.TypeRecord, val zcode.Bytes) (interface{}, error) {
	if val == nil {
		return nil, nil
	}
	// We start out with a slice that contains nothing instead of nil
	// so that an empty container encodes as a JSON empty array [].
	out := []interface{}{}
	k := 0
	for it := val.Iter(); !it.Done(); k++ {
		zv, _, err := it.Next()
		if err != nil {
			return nil, err
		}
		v, err := encodeValue(zctx, typ.Columns[k].Type, zv)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func encodeContainer(zctx *zson.Context, typ zng.Type, bytes zcode.Bytes) (interface{}, error) {
	if bytes == nil {
		return nil, nil
	}
	// We start out with a slice that contains nothing instead of nil
	// so that an empty container encodes as a JSON empty array [].
	out := []interface{}{}
	for it := bytes.Iter(); !it.Done(); {
		b, _, err := it.Next()
		if err != nil {
			return nil, err
		}
		v, err := encodeValue(zctx, typ, b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func decodeRecord(b *zcode.Builder, typ *zng.TypeRecord, v interface{}) error {
	values, ok := v.([]interface{})
	if !ok {
		return errors.New("ZJSON record value must be a JSON array")
	}
	cols := typ.Columns
	b.BeginContainer()
	for k, val := range values {
		if k >= len(cols) {
			return &zng.RecordTypeError{Name: "<record>", Type: typ.String(), Err: zng.ErrExtraField}
		}
		// each column either a string value or an array of string values
		if val == nil {
			// this is an unset column
			b.AppendNull()
			continue
		}
		if err := decodeValue(b, cols[k].Type, val); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func decodePrimitive(builder *zcode.Builder, typ zng.Type, v interface{}) error {
	if zng.IsContainerType(typ) && !zng.IsUnionType(typ) {
		return zng.ErrNotPrimitive
	}
	text, ok := v.(string)
	if !ok {
		return errors.New("ZJSON primitive value is not a JSON string")
	}
	val := zson.Primitive{
		Type: typ,
		Text: text,
	}
	err := zson.BuildPrimitive(builder, val)
	return err
}

func decodeContainerBody(b *zcode.Builder, typ zng.Type, body interface{}, which string) error {
	items, ok := body.([]interface{})
	if !ok {
		return fmt.Errorf("bad json for ZJSON %s value", which)
	}
	for _, item := range items {
		if err := decodeValue(b, typ, item); err != nil {
			return err
		}
	}
	return nil
}

func decodeContainer(b *zcode.Builder, typ zng.Type, body interface{}, which string) error {
	if body == nil {
		b.AppendNull()
		return nil
	}
	b.BeginContainer()
	err := decodeContainerBody(b, typ, body, which)
	b.EndContainer()
	return err
}

func decodeUnion(builder *zcode.Builder, typ *zng.TypeUnion, body interface{}) error {
	if body == nil {
		builder.AppendNull()
		return nil
	}
	tuple, ok := body.([]interface{})
	if !ok {
		return errors.New("bad json for ZJSON union value")
	}
	if len(tuple) != 2 {
		return errors.New("ZJSON union value not an array of two elements")
	}
	istr, ok := tuple[0].(string)
	if !ok {
		return errors.New("bad type index for ZJSON union value ")
	}
	index, err := strconv.Atoi(istr)
	if err != nil {
		return fmt.Errorf("bad type index for ZJSON union value: %w", err)
	}
	inner, err := typ.TypeIndex(index)
	if err != nil {
		return fmt.Errorf("bad type index for ZJSON union value: %w", err)
	}
	builder.BeginContainer()
	builder.AppendPrimitive(zng.EncodeInt(int64(index)))
	if err := decodeValue(builder, inner, tuple[1]); err != nil {
		return err
	}
	builder.EndContainer()
	return nil
}

func decodeMap(b *zcode.Builder, typ *zng.TypeMap, body interface{}) error {
	if body == nil {
		b.AppendNull()
		return nil
	}
	items, ok := body.([]interface{})
	if !ok {
		return errors.New("bad json for ZJSON union value")
	}
	b.BeginContainer()
	for _, item := range items {
		pair, ok := item.([]interface{})
		if !ok || len(pair) != 2 {
			return errors.New("ZJSON map value must be an array of two-element arrays")
		}
		if err := decodeValue(b, typ.KeyType, pair[0]); err != nil {
			return err
		}
		if err := decodeValue(b, typ.ValType, pair[1]); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func decodeValue(b *zcode.Builder, typ zng.Type, body interface{}) error {
	switch typ := typ.(type) {
	case *zng.TypeAlias:
		return decodeValue(b, typ.Type, body)
	case *zng.TypeUnion:
		return decodeUnion(b, typ, body)
	case *zng.TypeMap:
		return decodeMap(b, typ, body)
	case *zng.TypeEnum:
		return decodeEnum(b, typ, body)
	case *zng.TypeRecord:
		return decodeRecord(b, typ, body)
	case *zng.TypeArray:
		err := decodeContainer(b, typ.Type, body, "array")
		return err
	case *zng.TypeSet:
		if body == nil {
			b.AppendNull()
			return nil
		}
		b.BeginContainer()
		err := decodeContainerBody(b, typ.Type, body, "set")
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		return err
	default:
		return decodePrimitive(b, typ, body)
	}
}

func decodeEnum(b *zcode.Builder, typ *zng.TypeEnum, body interface{}) error {
	s, ok := body.(string)
	if !ok {
		return errors.New("ZJSON enum index value is not a JSON string")
	}
	index, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("ZJSON enum index value is not a string integer")
	}
	b.AppendPrimitive(zng.EncodeUint(uint64(index)))
	return nil
}
