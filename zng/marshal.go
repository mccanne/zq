package zng

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zcode"
)

var (
	errNotStruct = errors.New("not a struct or struct ptr")

	marshalerType   = reflect.TypeOf((*Marshaler)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

type TypeContext interface {
	LookupTypeRecord([]Column) (*TypeRecord, error)
	LookupTypeArray(Type) *TypeArray
}

type Marshaler interface {
	MarshalZNG(TypeContext, *zcode.Builder) (Type, error)
}

type MarshalContext struct {
	zctx TypeContext
	// more stuff will go here in a subsequent PR
}

func Marshal(zctx TypeContext, b *zcode.Builder, v interface{}) (Type, error) {
	return encodeAny(zctx, b, reflect.ValueOf(v))
}

func MarshalRecord(zctx TypeContext, v interface{}) (*Record, error) {
	var b zcode.Builder
	typ, err := Marshal(zctx, &b, v)
	if err != nil {
		return nil, err
	}
	recType, ok := typ.(*TypeRecord)
	if !ok {
		return nil, errors.New("not a record")
	}
	body, err := b.Bytes().ContainerBody()
	if err != nil {
		return nil, err
	}
	return NewRecord(recType, body), nil
}

func MarshalCustomRecord(zctx TypeContext, names []string, fields []interface{}) (*Record, error) {
	if len(names) != len(fields) {
		return nil, errors.New("fields and columns don't match")
	}
	var cols []Column
	var b zcode.Builder
	for k, field := range fields {
		typ, err := Marshal(zctx, &b, field)
		if err != nil {
			return nil, err
		}
		cols = append(cols, Column{names[k], typ})
	}
	recType, err := zctx.LookupTypeRecord(cols)
	if err != nil {
		return nil, err
	}
	return NewRecord(recType, b.Bytes()), nil
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

func encodeAny(zctx TypeContext, b *zcode.Builder, v reflect.Value) (Type, error) {
	if !v.IsValid() {
		b.AppendPrimitive(nil)
		return TypeNull, nil
	}
	if v.Type().Implements(marshalerType) {
		return v.Interface().(Marshaler).MarshalZNG(zctx, b)
	}
	if v, ok := v.Interface().(nano.Ts); ok {
		b.AppendPrimitive(EncodeTime(v))
		return TypeTime, nil
	}
	switch v.Kind() {
	case reflect.Array:
		return encodeArray(zctx, b, v)
	case reflect.Slice:
		if v.IsNil() {
			return encodeNil(zctx, b, v.Type())
		}
		return encodeArray(zctx, b, v)
	case reflect.Struct:
		return encodeRecord(zctx, b, v)
	case reflect.Ptr:
		if v.IsNil() {
			return encodeNil(zctx, b, v.Type())
		}
		return encodeAny(zctx, b, v.Elem())
	case reflect.String:
		b.AppendPrimitive(EncodeString(v.String()))
		return TypeString, nil
	case reflect.Bool:
		b.AppendPrimitive(EncodeBool(v.Bool()))
		return TypeBool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		zt, err := lookupType(zctx, v.Type())
		if err != nil {
			return nil, err
		}
		b.AppendPrimitive(EncodeInt(v.Int()))
		return zt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		zt, err := lookupType(zctx, v.Type())
		if err != nil {
			return nil, err
		}
		b.AppendPrimitive(EncodeUint(v.Uint()))
		return zt, nil
	// XXX add float32 to zng?
	case reflect.Float64, reflect.Float32:
		b.AppendPrimitive(EncodeFloat64(v.Float()))
		return TypeFloat64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func encodeNil(zctx TypeContext, b *zcode.Builder, t reflect.Type) (Type, error) {
	typ, err := lookupType(zctx, t)
	if err != nil {
		return nil, err
	}
	if IsContainerType(typ) {
		b.AppendContainer(nil)
	} else {
		b.AppendPrimitive(nil)
	}
	return typ, nil
}

func encodeRecord(zctx TypeContext, b *zcode.Builder, sval reflect.Value) (Type, error) {
	b.BeginContainer()
	var columns []Column
	stype := sval.Type()
	for i := 0; i < stype.NumField(); i++ {
		field := stype.Field(i)
		name := fieldName(field)
		typ, err := encodeAny(zctx, b, sval.Field(i))
		if err != nil {
			return nil, err
		}
		columns = append(columns, Column{name, typ})
	}
	b.EndContainer()
	return zctx.LookupTypeRecord(columns)
}

func isIP(typ reflect.Type) bool {
	return typ.Name() == "IP" && typ.PkgPath() == "net"
}

func encodeArray(zctx TypeContext, b *zcode.Builder, arrayVal reflect.Value) (Type, error) {
	if isIP(arrayVal.Type()) {
		b.AppendPrimitive(EncodeIP(arrayVal.Bytes()))
		return TypeIP, nil
	}
	len := arrayVal.Len()
	b.BeginContainer()
	var innerType Type
	for i := 0; i < len; i++ {
		item := arrayVal.Index(i)
		typ, err := encodeAny(zctx, b, item)
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

func lookupType(zctx TypeContext, typ reflect.Type) (Type, error) {
	switch typ.Kind() {
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
		return TypeString, nil
	case reflect.Bool:
		return TypeBool, nil
	case reflect.Int, reflect.Int64:
		return TypeInt64, nil
	case reflect.Int32:
		return TypeInt32, nil
	case reflect.Int16:
		return TypeInt16, nil
	case reflect.Int8:
		return TypeInt8, nil
	case reflect.Uint, reflect.Uint64:
		return TypeUint64, nil
	case reflect.Uint32:
		return TypeUint32, nil
	case reflect.Uint16:
		return TypeUint16, nil
	case reflect.Uint8:
		return TypeUint8, nil
	case reflect.Float64, reflect.Float32:
		return TypeUint64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", typ.Kind())
	}
}

func lookupTypeRecord(zctx TypeContext, structType reflect.Type) (Type, error) {
	var columns []Column
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		name := fieldName(field)
		fieldType, err := lookupType(zctx, field.Type)
		if err != nil {
			return nil, err
		}
		columns = append(columns, Column{name, fieldType})
	}
	return zctx.LookupTypeRecord(columns)
}

type Unmarshaler interface {
	UnmarshalZNG(TypeContext, Type, zcode.Bytes) error
}

func Unmarshal(zctx TypeContext, typ Type, zv zcode.Bytes, v interface{}) error {
	return decodeAny(zctx, typ, zv, reflect.ValueOf(v))
}

func UnmarshalRecord(zctx TypeContext, rec *Record, v interface{}) error {
	return decodeAny(zctx, rec.Type, rec.Raw, reflect.ValueOf(v))
}

func incompatTypeError(zt Type, v reflect.Value) error {
	return fmt.Errorf("incompatible type translation: zng type %v go type %v go kind %v", zt, v.Type(), v.Kind())
}

func decodeAny(zctx TypeContext, typ Type, zv zcode.Bytes, v reflect.Value) error {
	if v.Type().Implements(unmarshalerType) {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return v.Interface().(Unmarshaler).UnmarshalZNG(zctx, typ, zv)
	}
	if _, ok := v.Interface().(nano.Ts); ok {
		if typ != TypeTime {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeTime(zv)
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
		if typ != TypeString {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeString(zv)
		v.SetString(x)
		return err
	case reflect.Bool:
		if typ != TypeBool {
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeBool(zv)
		v.SetBool(x)
		return err
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch typ {
		case TypeInt8, TypeInt16, TypeInt32, TypeInt64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeInt(zv)
		v.SetInt(x)
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch typ {
		case TypeUint8, TypeUint16, TypeUint32, TypeUint64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeUint(zv)
		v.SetUint(x)
		return err
	case reflect.Float32, reflect.Float64:
		// TODO: TypeFloat32 when it lands
		switch typ {
		case TypeFloat64:
		default:
			return incompatTypeError(typ, v)
		}
		if zv == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := DecodeFloat64(zv)
		v.SetFloat(x)
		return err
	default:
		return fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func decodeIP(typ Type, zv zcode.Bytes, v reflect.Value) error {
	if typ != TypeIP {
		return incompatTypeError(typ, v)
	}
	if zv == nil {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}
	x, err := DecodeIP(zv)
	v.Set(reflect.ValueOf(x))
	return err
}

func decodeRecord(zctx TypeContext, typ Type, zv zcode.Bytes, sval reflect.Value) error {
	recType, ok := typ.(*TypeRecord)
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
			return ErrMismatch
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

func decodeArray(zctx TypeContext, typ Type, zv zcode.Bytes, arrVal reflect.Value) error {
	arrType, ok := typ.(*TypeArray)
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
