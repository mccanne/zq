package resolver

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zng"
)

var (
	errNotStruct = errors.New("not a struct or struct ptr")

	marshalerType   = reflect.TypeOf((*Marshaler)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

type Marshaler interface {
	MarshalZNG(*Context, *zcode.Builder) (zng.Type, error)
}

func Marshal(zctx *Context, b *zcode.Builder, v interface{}) (zng.Type, error) {
	return encodeAny(zctx, b, reflect.ValueOf(v), false)
}

func MarshalValue(zctx *Context, v interface{}) (zng.Value, error) {
	var b zcode.Builder
	typ, err := encodeAny(zctx, &b, reflect.ValueOf(v), true)
	if err != nil {
		return zng.Value{}, err
	}
	return zng.Value{typ, b.Bytes()}, nil
}

func MarshalRecord(zctx *Context, v interface{}) (*zng.Record, error) {
	var b zcode.Builder
	typ, err := Marshal(zctx, &b, v)
	if err != nil {
		return nil, err
	}
	recType, ok := typ.(*zng.TypeRecord)
	if !ok {
		return nil, errors.New("not a record")
	}
	body, err := b.Bytes().ContainerBody()
	if err != nil {
		return nil, err
	}
	return zng.NewRecord(recType, body), nil
}

func MarshalCustomRecord(zctx *Context, names []string, fields []interface{}) (*zng.Record, error) {
	if len(names) != len(fields) {
		return nil, errors.New("fields and columns don't match")
	}
	var cols []zng.Column
	var b zcode.Builder
	for k, field := range fields {
		typ, err := Marshal(zctx, &b, field)
		if err != nil {
			return nil, err
		}
		cols = append(cols, zng.Column{names[k], typ})
	}
	recType, err := zctx.LookupTypeRecord(cols)
	if err != nil {
		return nil, err
	}
	return zng.NewRecord(recType, b.Bytes()), nil
}

const (
	tagName = "zng"
	tagSep  = ","
)

func fieldName(f reflect.StructField) string {
	tag := f.Tag.Get(tagName)
	if tag != "" {
		s := strings.SplitN(tag, tagSep, 2)
		if len(s) > 0 && s[0] != "" {
			return s[0]
		}
	}
	return f.Name
}

func encodeAny(zctx *Context, b *zcode.Builder, v reflect.Value, types bool) (zng.Type, error) {
	typ, err := encodeAnyValue(zctx, b, v, types)
	if err != nil {
		return nil, err
	}
	if types && v.IsValid() {
		name := v.Type().Name()
		kind := v.Kind().String()
		if name != "" && name != kind {
			path := strings.Split(v.Type().PkgPath(), "/")
			module := path[len(path)-1]
			return zctx.LookupTypeAlias(module+"."+name, typ)
		}
	}
	return typ, nil
}

func encodeAnyValue(zctx *Context, b *zcode.Builder, v reflect.Value, types bool) (zng.Type, error) {
	if !v.IsValid() {
		b.AppendPrimitive(nil)
		return zng.TypeNull, nil
	}
	if v.Type().Implements(marshalerType) {
		return v.Interface().(Marshaler).MarshalZNG(zctx, b)
	}
	if v, ok := v.Interface().(nano.Ts); ok {
		b.AppendPrimitive(zng.EncodeTime(v))
		return zng.TypeTime, nil
	}
	switch v.Kind() {
	case reflect.Array:
		return encodeArray(zctx, b, v, types)
	case reflect.Slice:
		if v.IsNil() {
			return encodeNil(zctx, b, v.Type())
		}
		return encodeArray(zctx, b, v, types)
	case reflect.Struct:
		return encodeRecord(zctx, b, v, types)
	case reflect.Ptr:
		if v.IsNil() {
			return encodeNil(zctx, b, v.Type())
		}
		return encodeAny(zctx, b, v.Elem(), types)
	case reflect.String:
		b.AppendPrimitive(zng.EncodeString(v.String()))
		return zng.TypeString, nil
	case reflect.Bool:
		b.AppendPrimitive(zng.EncodeBool(v.Bool()))
		return zng.TypeBool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		zt, err := lookupType(zctx, v.Type())
		if err != nil {
			return nil, err
		}
		b.AppendPrimitive(zng.EncodeInt(v.Int()))
		return zt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		zt, err := lookupType(zctx, v.Type())
		if err != nil {
			return nil, err
		}
		b.AppendPrimitive(zng.EncodeUint(v.Uint()))
		return zt, nil
	// XXX add float32 to zng?
	case reflect.Float64, reflect.Float32:
		b.AppendPrimitive(zng.EncodeFloat64(v.Float()))
		return zng.TypeFloat64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func encodeNil(zctx *Context, b *zcode.Builder, t reflect.Type) (zng.Type, error) {
	typ, err := lookupType(zctx, t)
	if err != nil {
		return nil, err
	}
	if zng.IsContainerType(typ) {
		b.AppendContainer(nil)
	} else {
		b.AppendPrimitive(nil)
	}
	return typ, nil
}

func encodeRecord(zctx *Context, b *zcode.Builder, sval reflect.Value, types bool) (zng.Type, error) {
	b.BeginContainer()
	var columns []zng.Column
	stype := sval.Type()
	for i := 0; i < stype.NumField(); i++ {
		field := stype.Field(i)
		name := fieldName(field)
		typ, err := encodeAny(zctx, b, sval.Field(i), types)
		if err != nil {
			return nil, err
		}
		columns = append(columns, zng.Column{name, typ})
	}
	b.EndContainer()
	return zctx.LookupTypeRecord(columns)
}

func isIP(typ reflect.Type) bool {
	return typ.Name() == "IP" && typ.PkgPath() == "net"
}

func encodeArray(zctx *Context, b *zcode.Builder, arrayVal reflect.Value, types bool) (zng.Type, error) {
	if isIP(arrayVal.Type()) {
		b.AppendPrimitive(zng.EncodeIP(arrayVal.Bytes()))
		return zng.TypeIP, nil
	}
	len := arrayVal.Len()
	b.BeginContainer()
	var innerType zng.Type
	for i := 0; i < len; i++ {
		item := arrayVal.Index(i)
		typ, err := encodeAny(zctx, b, item, types)
		if err != nil {
			return nil, err
		}
		innerType = typ
	}
	b.EndContainer()
	if innerType == nil {
		// if slice was empty, look up the type without a value
		var err error
		innerType, err = lookupType(zctx, arrayVal.Type().Elem())
		if err != nil {
			return nil, err
		}
	}
	return zctx.LookupTypeArray(innerType), nil
}

func lookupType(zctx *Context, typ reflect.Type) (zng.Type, error) {
	switch typ.Kind() {
	//case reflect.Interface:
	//	return lookupTypeAlias(zctx, typ)
	case reflect.Array, reflect.Slice:
		typ, err := lookupType(zctx, typ.Elem())
		if err != nil {
			return nil, err
		}
		return zctx.LookupTypeArray(typ), nil
	case reflect.Struct:
		return lookupTypeRecord(zctx, typ)
	case reflect.Ptr:
		return lookupType(zctx, typ.Elem())
	case reflect.String:
		return zng.TypeString, nil
	case reflect.Bool:
		return zng.TypeBool, nil
	case reflect.Int, reflect.Int64:
		return zng.TypeInt64, nil
	case reflect.Int32:
		return zng.TypeInt32, nil
	case reflect.Int16:
		return zng.TypeInt16, nil
	case reflect.Int8:
		return zng.TypeInt8, nil
	case reflect.Uint, reflect.Uint64:
		return zng.TypeUint64, nil
	case reflect.Uint32:
		return zng.TypeUint32, nil
	case reflect.Uint16:
		return zng.TypeUint16, nil
	case reflect.Uint8:
		return zng.TypeUint8, nil
	case reflect.Float64, reflect.Float32:
		return zng.TypeUint64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", typ.Kind())
	}
}

func lookupTypeRecord(zctx *Context, structType reflect.Type) (zng.Type, error) {
	var columns []zng.Column
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		name := fieldName(field)
		fieldType, err := lookupType(zctx, field.Type)
		if err != nil {
			return nil, err
		}
		columns = append(columns, zng.Column{name, fieldType})
	}
	return zctx.LookupTypeRecord(columns)
}

type Unmarshaler interface {
	UnmarshalZNG(*Context, zng.Type, zcode.Bytes) error
}

func Unmarshal(zctx *Context, typ zng.Type, zv zcode.Bytes, v interface{}) error {
	return decodeAny(zctx, typ, zv, reflect.ValueOf(v))
}

func UnmarshalRecord(zctx *Context, rec *zng.Record, v interface{}) error {
	return decodeAny(zctx, rec.Type, rec.Raw, reflect.ValueOf(v))
}

func incompatTypeError(zt zng.Type, v reflect.Value) error {
	return fmt.Errorf("incompatible type translation: zng type %v go type %v go kind %v", zt, v.Type(), v.Kind())
}

func decodeAny(zctx *Context, typ zng.Type, zv zcode.Bytes, v reflect.Value) error {
	if v.Type().Implements(unmarshalerType) {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return v.Interface().(Unmarshaler).UnmarshalZNG(zctx, typ, zv)
	}
	if _, ok := v.Interface().(nano.Ts); ok {
		if typ != zng.TypeTime {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeTime(zv)
		v.Set(reflect.ValueOf(x))
		return err
	}
	switch v.Kind() {
	case reflect.Array:
		return decodeArray(zctx, typ, zv, v)
	case reflect.Slice:
		if isIP(v.Type()) {
			return decodeIP(typ, zv, v)
		}
		return decodeArray(zctx, typ, zv, v)
	case reflect.Struct:
		return decodeRecord(zctx, typ, zv, v)
	case reflect.Ptr:
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
		err := decodeAny(zctx, typ, zv, v)
		return err
	case reflect.String:
		if typ != zng.TypeString {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeString(zv)
		v.SetString(x)
		return err
	case reflect.Bool:
		if typ != zng.TypeBool {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeBool(zv)
		v.SetBool(x)
		return err
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch typ {
		case zng.TypeInt8, zng.TypeInt16, zng.TypeInt32, zng.TypeInt64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeInt(zv)
		v.SetInt(x)
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch typ {
		case zng.TypeUint8, zng.TypeUint16, zng.TypeUint32, zng.TypeUint64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeUint(zv)
		v.SetUint(x)
		return err
	case reflect.Float32, reflect.Float64:
		// TODO: zng.TypeFloat32 when it lands
		switch typ {
		case zng.TypeFloat64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zng.DecodeFloat64(zv)
		v.SetFloat(x)
		return err
	default:
		return fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func decodeIP(typ zng.Type, zv zcode.Bytes, v reflect.Value) error {
	if typ != zng.TypeIP {
		return incompatTypeError(typ, v)
	}
	if zv == nil {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}
	x, err := zng.DecodeIP(zv)
	v.Set(reflect.ValueOf(x))
	return err
}

func decodeRecord(zctx *Context, typ zng.Type, zv zcode.Bytes, sval reflect.Value) error {
	recType, ok := typ.(*zng.TypeRecord)
	if !ok {
		return errors.New("not a record")
	}
	nameToField := make(map[string]int)
	stype := sval.Type()
	for i := 0; i < stype.NumField(); i++ {
		if !sval.Field(i).CanSet() {
			continue
		}
		field := stype.Field(i)
		name := fieldName(field)
		nameToField[name] = i
	}
	for i, it := 0, zv.Iter(); !it.Done(); i++ {
		if i >= len(recType.Columns) {
			return zng.ErrMismatch
		}
		itzv, _, err := it.Next()
		if err != nil {
			return err
		}
		name := recType.Columns[i].Name
		if fieldIdx, ok := nameToField[name]; ok {
			typ := recType.Columns[i].Type
			if err := decodeAny(zctx, typ, itzv, sval.Field(fieldIdx)); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeArray(zctx *Context, typ zng.Type, zv zcode.Bytes, arrVal reflect.Value) error {
	arrType, ok := typ.(*zng.TypeArray)
	if !ok {
		return errors.New("not an array")
	}
	if zv == nil {
		return nil
	}
	i := 0
	for it := zv.Iter(); !it.Done(); i++ {
		itzv, _, err := it.Next()
		if err != nil {
			return err
		}
		if i >= arrVal.Cap() {
			newcap := arrVal.Cap() + arrVal.Cap()/2
			if newcap < 4 {
				newcap = 4
			}
			newArr := reflect.MakeSlice(arrVal.Type(), arrVal.Len(), newcap)
			reflect.Copy(newArr, arrVal)
			arrVal.Set(newArr)
		}
		if i >= arrVal.Len() {
			arrVal.SetLen(i + 1)
		}
		if err := decodeAny(zctx, arrType.Type, itzv, arrVal.Index(i)); err != nil {
			return err
		}
	}
	switch {
	case i == 0:
		arrVal.Set(reflect.MakeSlice(arrVal.Type(), 0, 0))
	case i < arrVal.Len():
		arrVal.SetLen(i)
	}
	return nil
}
