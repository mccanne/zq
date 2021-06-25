package zson

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
)

var (
	errNotStruct = errors.New("not a struct or struct ptr")

	marshalerTypeZNG   = reflect.TypeOf((*ZNGMarshaler)(nil)).Elem()
	unmarshalerTypeZNG = reflect.TypeOf((*ZNGUnmarshaler)(nil)).Elem()
)

func Marshal(v interface{}) (string, error) {
	return NewMarshaler().Marshal(v)
}

type MarshalContext struct {
	*MarshalZNGContext
	formatter *Formatter
}

func NewMarshaler() *MarshalContext {
	return NewMarshalerIndent(0)
}

func NewMarshalerIndent(indent int) *MarshalContext {
	return &MarshalContext{
		MarshalZNGContext: NewZNGMarshaler(),
		formatter:         NewFormatter(indent),
	}
}

func NewMarshalerWithContext(zctx *Context) *MarshalContext {
	return &MarshalContext{
		MarshalZNGContext: NewZNGMarshalerWithContext(zctx),
	}
}

func (m *MarshalContext) Marshal(v interface{}) (string, error) {
	zv, err := m.MarshalZNGContext.Marshal(v)
	if err != nil {
		return "", err
	}
	return m.formatter.Format(zv)
}

func (m *MarshalContext) MarshalCustom(names []string, fields []interface{}) (string, error) {
	rec, err := m.MarshalZNGContext.MarshalCustom(names, fields)
	if err != nil {
		return "", err
	}
	return m.formatter.FormatRecord(rec)
}

type UnmarshalContext struct {
	*UnmarshalZNGContext
	zctx     *Context
	analyzer Analyzer
	builder  *zcode.Builder
}

func NewUnmarshaler() *UnmarshalContext {
	return &UnmarshalContext{
		UnmarshalZNGContext: NewZNGUnmarshaler(),
		zctx:                NewContext(),
		analyzer:            NewAnalyzer(),
		builder:             zcode.NewBuilder(),
	}
}

func Unmarshal(zson string, v interface{}) error {
	return NewUnmarshaler().Unmarshal(zson, v)
}

func (u *UnmarshalContext) Unmarshal(zson string, v interface{}) error {
	parser, err := NewParser(strings.NewReader(zson))
	if err != nil {
		return err
	}
	ast, err := parser.ParseValue()
	if err != nil {
		return err
	}
	val, err := u.analyzer.ConvertValue(u.zctx, ast)
	if err != nil {
		return err
	}
	zv, err := Build(u.builder, val)
	if err != nil {
		return nil
	}
	return u.UnmarshalZNGContext.Unmarshal(zv, v)
}

type ZNGMarshaler interface {
	MarshalZNG(*MarshalZNGContext) (zed.Type, error)
}

func MarshalZNG(v interface{}) (zed.Value, error) {
	return NewZNGMarshaler().Marshal(v)
}

type MarshalZNGContext struct {
	*Context
	zcode.Builder
	decorator func(string, string) string
	bindings  map[string]string
}

func NewZNGMarshaler() *MarshalZNGContext {
	return NewZNGMarshalerWithContext(NewContext())
}

func NewZNGMarshalerWithContext(zctx *Context) *MarshalZNGContext {
	return &MarshalZNGContext{
		Context: zctx,
	}
}

// MarshalValue marshals v into the value that is being built and is
// typically called by a custom marshaler.
func (m *MarshalZNGContext) MarshalValue(v interface{}) (zed.Type, error) {
	return m.encodeValue(reflect.ValueOf(v))
}

func (m *MarshalZNGContext) Marshal(v interface{}) (zed.Value, error) {
	m.Builder.Reset()
	typ, err := m.encodeValue(reflect.ValueOf(v))
	if err != nil {
		return zed.Value{}, err
	}
	bytes := m.Builder.Bytes()
	it := bytes.Iter()
	if it.Done() {
		return zed.Value{}, errors.New("no value found")
	}
	bytes, _, err = it.Next()
	if err != nil {
		return zed.Value{}, err
	}
	return zed.Value{typ, bytes}, nil
}

func (m *MarshalZNGContext) MarshalRecord(v interface{}) (*zed.Record, error) {
	m.Builder.Reset()
	typ, err := m.encodeValue(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	if !zed.IsRecordType(typ) {
		return nil, errors.New("not a record")
	}
	body, err := m.Builder.Bytes().ContainerBody()
	if err != nil {
		return nil, err
	}
	return zed.NewRecord(typ, body), nil
}

func (m *MarshalZNGContext) MarshalCustom(names []string, fields []interface{}) (*zed.Record, error) {
	if len(names) != len(fields) {
		return nil, errors.New("fields and columns don't match")
	}
	m.Builder.Reset()
	var cols []zed.Column
	for k, field := range fields {
		typ, err := m.encodeValue(reflect.ValueOf(field))
		if err != nil {
			return nil, err
		}
		cols = append(cols, zed.Column{names[k], typ})
	}
	// XXX issue #1836
	// Since this can be the inner loop here and nowhere else do we call
	// LookupTypeRecord on the inner loop, now may be the time to put an
	// efficient cache ahead of formatting the columns into a string,
	// e.g., compute a has in place across the field names then do a
	// closed-address exact match for the values in the slot.
	recType, err := m.Context.LookupTypeRecord(cols)
	if err != nil {
		return nil, err
	}
	return zed.NewRecord(recType, m.Builder.Bytes()), nil
}

const (
	tagName = "zng"
	tagSep  = ","
)

func fieldName(f reflect.StructField) string {
	tag := f.Tag.Get(tagName)
	if tag == "" {
		tag = f.Tag.Get("json")
	}
	if tag != "" {
		s := strings.SplitN(tag, tagSep, 2)
		if len(s) > 0 && s[0] != "" {
			return s[0]
		}
	}
	return f.Name
}

func typeSimple(name, path string) string {
	return name
}

func typePackage(name, path string) string {
	a := strings.Split(path, "/")
	return fmt.Sprintf("%s.%s", a[len(a)-1], name)
}

func typeFull(name, path string) string {
	return fmt.Sprintf("%s.%s", path, name)
}

type TypeStyle int

const (
	StyleNone TypeStyle = iota
	StyleSimple
	StylePackage
	StyleFull
)

// Decorate informs the marshaler to add type decorations to the resulting ZNG
// in the form of type aliases that are named in the sytle indicated, e.g.,
// for a `struct Foo` in `package bar` at import path `github.com/acme/bar:
// the corresponding name would be `Foo` for TypeSimple, `bar.Foo` for TypePackage,
// and `github.com/acme/bar.Foo`for TypeFull.  This mechanism works in conjunction
// with Bindings.  Typically you would want just one or the other, but if a binding
// doesn't exist for a given Go type, then a ZSON type name will be created according
// to the decorator setting (which may be TypeNone).
func (m *MarshalZNGContext) Decorate(style TypeStyle) {
	switch style {
	default:
		m.decorator = nil
	case StyleSimple:
		m.decorator = typeSimple
	case StylePackage:
		m.decorator = typePackage
	case StyleFull:
		m.decorator = typeFull
	}
}

// NamedBindings informs the Marshaler to encode the given types with the
// corresponding ZSON type names.  For example, to serialize a `bar.Foo`
// value decoroated with the ZSON type name "SpecialFoo", simply call
// NamedBindings with the value []Binding{{"SpecialFoo", &bar.Foo{}}.
// Subsequent calls to NamedBindings
// add additional such bindings leaving the existing bindings in place.
// During marshaling, if no binding is found for a particular Go value,
// then the marshaler's decorator setting applies.
func (m *MarshalZNGContext) NamedBindings(bindings []Binding) error {
	if m.bindings == nil {
		m.bindings = make(map[string]string)
	}
	for _, b := range bindings {
		name, err := typeNameOfValue(b.Template)
		if err != nil {
			return err
		}
		m.bindings[name] = b.Name
	}
	return nil
}

var nanoTsType = reflect.TypeOf(nano.Ts(0))
var zngValueType = reflect.TypeOf(zed.Value{})

func (m *MarshalZNGContext) encodeValue(v reflect.Value) (zed.Type, error) {
	typ, err := m.encodeAny(v)
	if err != nil {
		return nil, err
	}
	if m.decorator != nil || m.bindings != nil {
		// Don't create aliases for interface types as this is just
		// one value for that interface and it's the underlying concrete
		// types that implement the interface that we want to alias.
		if !v.IsValid() || v.Kind() == reflect.Interface {
			return typ, nil
		}
		name := v.Type().Name()
		kind := v.Kind().String()
		if name != "" && name != kind {
			// We do not want to further decorate nano.Ts as
			// it's already been converted to a Zed time;
			// likewise for zed.Value, which gets encoded as
			// itself and its own alias type if it has one.
			if t := v.Type(); t == nanoTsType || t == zngValueType {
				return typ, nil
			}
			path := v.Type().PkgPath()
			var alias string
			if m.bindings != nil {
				alias = m.bindings[typeFull(name, path)]
			}
			if alias == "" && m.decorator != nil {
				alias = m.decorator(name, path)
			}
			if alias != "" {
				return m.Context.LookupTypeAlias(alias, typ)
			}
		}
	}
	return typ, nil
}

func (m *MarshalZNGContext) encodeAny(v reflect.Value) (zed.Type, error) {
	if !v.IsValid() {
		m.Builder.AppendPrimitive(nil)
		return zed.TypeNull, nil
	}
	if v.Type().Implements(marshalerTypeZNG) {
		return v.Interface().(ZNGMarshaler).MarshalZNG(m)
	}
	if ts, ok := v.Interface().(nano.Ts); ok {
		m.Builder.AppendPrimitive(zed.EncodeTime(ts))
		return zed.TypeTime, nil
	}
	if zv, ok := v.Interface().(zed.Value); ok {
		typ, err := m.TranslateType(zv.Type)
		if err != nil {
			return nil, err
		}
		if zed.IsContainerType(typ) {
			m.Builder.AppendContainer(zv.Bytes)
		} else {
			m.Builder.AppendPrimitive(zv.Bytes)
		}
		return typ, nil
	}
	switch v.Kind() {
	case reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return m.encodeArrayBytes(v)
		}
		return m.encodeArray(v)
	case reflect.Slice:
		if v.IsNil() {
			return m.encodeNil(v.Type())
		}
		if isIP(v.Type()) {
			m.Builder.AppendPrimitive(zed.EncodeIP(v.Bytes()))
			return zed.TypeIP, nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return m.encodeSliceBytes(v)
		}
		return m.encodeArray(v)
	case reflect.Struct:
		return m.encodeRecord(v)
	case reflect.Ptr:
		if v.IsNil() {
			return m.encodeNil(v.Type())
		}
		return m.encodeValue(v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			return m.encodeNil(v.Type())
		}
		return m.encodeValue(v.Elem())
	case reflect.String:
		m.Builder.AppendPrimitive(zed.EncodeString(v.String()))
		return zed.TypeString, nil
	case reflect.Bool:
		m.Builder.AppendPrimitive(zed.EncodeBool(v.Bool()))
		return zed.TypeBool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		zt, err := m.lookupType(v.Type())
		if err != nil {
			return nil, err
		}
		m.Builder.AppendPrimitive(zed.EncodeInt(v.Int()))
		return zt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		zt, err := m.lookupType(v.Type())
		if err != nil {
			return nil, err
		}
		m.Builder.AppendPrimitive(zed.EncodeUint(v.Uint()))
		return zt, nil
	// XXX add float32 to zng?
	case reflect.Float64, reflect.Float32:
		m.Builder.AppendPrimitive(zed.EncodeFloat64(v.Float()))
		return zed.TypeFloat64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func (m *MarshalZNGContext) encodeNil(t reflect.Type) (zed.Type, error) {
	typ, err := m.lookupType(t)
	if err != nil {
		return nil, err
	}
	if zed.IsContainerType(typ) {
		m.Builder.AppendContainer(nil)
	} else {
		m.Builder.AppendPrimitive(nil)
	}
	return typ, nil
}

func (m *MarshalZNGContext) encodeRecord(sval reflect.Value) (zed.Type, error) {
	m.Builder.BeginContainer()
	var columns []zed.Column
	stype := sval.Type()
	for i := 0; i < stype.NumField(); i++ {
		sf := stype.Field(i)
		isUnexported := sf.PkgPath != ""
		if sf.Anonymous {
			t := sf.Type
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			if isUnexported && t.Kind() != reflect.Struct {
				// Ignore embedded fields of unexported non-struct types.
				continue
			}
			// Do not ignore embedded fields of unexported struct types
			// since they may have exported fields.
		} else if isUnexported {
			// Ignore unexported non-embedded fields.
			continue
		}
		field := stype.Field(i)
		name := fieldName(field)
		typ, err := m.encodeValue(sval.Field(i))
		if err != nil {
			return nil, err
		}
		columns = append(columns, zed.Column{name, typ})
	}
	m.Builder.EndContainer()
	return m.Context.LookupTypeRecord(columns)
}

func (m *MarshalZNGContext) encodeSliceBytes(sliceVal reflect.Value) (zed.Type, error) {
	m.Builder.AppendPrimitive(sliceVal.Bytes())
	return zed.TypeBytes, nil
}

func (m *MarshalZNGContext) encodeArrayBytes(arrayVal reflect.Value) (zed.Type, error) {
	n := arrayVal.Len()
	bytes := make([]byte, 0, n)
	for k := 0; k < n; k++ {
		v := arrayVal.Index(k)
		bytes = append(bytes, v.Interface().(uint8))
	}
	m.Builder.AppendPrimitive(bytes)
	return zed.TypeBytes, nil
}

func (m *MarshalZNGContext) encodeArray(arrayVal reflect.Value) (zed.Type, error) {
	len := arrayVal.Len()
	m.Builder.BeginContainer()
	var innerType zed.Type
	for i := 0; i < len; i++ {
		item := arrayVal.Index(i)
		typ, err := m.encodeValue(item)
		if err != nil {
			return nil, err
		}
		// XXX See bug #2575
		innerType = typ
	}
	m.Builder.EndContainer()
	if innerType == nil {
		// if slice was empty, look up the type without a value
		var err error
		innerType, err = m.lookupType(arrayVal.Type().Elem())
		if err != nil {
			return nil, err
		}
	}
	return m.Context.LookupTypeArray(innerType), nil
}

func (m *MarshalZNGContext) lookupType(typ reflect.Type) (zed.Type, error) {
	switch typ.Kind() {
	case reflect.Array, reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return zed.TypeBytes, nil
		}
		typ, err := m.lookupType(typ.Elem())
		if err != nil {
			return nil, err
		}
		return m.Context.LookupTypeArray(typ), nil
	case reflect.Struct:
		return m.lookupTypeRecord(typ)
	case reflect.Ptr:
		return m.lookupType(typ.Elem())
	case reflect.String:
		return zed.TypeString, nil
	case reflect.Bool:
		return zed.TypeBool, nil
	case reflect.Int, reflect.Int64:
		return zed.TypeInt64, nil
	case reflect.Int32:
		return zed.TypeInt32, nil
	case reflect.Int16:
		return zed.TypeInt16, nil
	case reflect.Int8:
		return zed.TypeInt8, nil
	case reflect.Uint, reflect.Uint64:
		return zed.TypeUint64, nil
	case reflect.Uint32:
		return zed.TypeUint32, nil
	case reflect.Uint16:
		return zed.TypeUint16, nil
	case reflect.Uint8:
		return zed.TypeUint8, nil
	case reflect.Float64, reflect.Float32:
		return zed.TypeUint64, nil
	default:
		return nil, fmt.Errorf("unsupported type: %v", typ.Kind())
	}
}

func (m *MarshalZNGContext) lookupTypeRecord(structType reflect.Type) (zed.Type, error) {
	var columns []zed.Column
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		name := fieldName(field)
		fieldType, err := m.lookupType(field.Type)
		if err != nil {
			return nil, err
		}
		columns = append(columns, zed.Column{name, fieldType})
	}
	return m.Context.LookupTypeRecord(columns)
}

type ZNGUnmarshaler interface {
	UnmarshalZNG(*UnmarshalZNGContext, zed.Value) error
}

type UnmarshalZNGContext struct {
	binder binder
}

func NewZNGUnmarshaler() *UnmarshalZNGContext {
	return &UnmarshalZNGContext{}
}

func UnmarshalZNG(zv zed.Value, v interface{}) error {
	return NewZNGUnmarshaler().decodeAny(zv, reflect.ValueOf(v))
}

func UnmarshalZNGRecord(rec *zed.Record, v interface{}) error {
	return NewZNGUnmarshaler().decodeAny(rec.Value, reflect.ValueOf(v))
}

func incompatTypeError(zt zed.Type, v reflect.Value) error {
	return fmt.Errorf("incompatible type translation: zng type %v go type %v go kind %v", zt, v.Type(), v.Kind())
}

func (u *UnmarshalZNGContext) Unmarshal(zv zed.Value, v interface{}) error {
	return u.decodeAny(zv, reflect.ValueOf(v))
}

// Bindings informs the unmarshaler that ZSON values with a type name equal
// to any of the three variations of Go type mame (full path, package.Type,
// or just Type) may be used to inform the deserialization of a ZSON value
// into a Go interface value.  If full path names are not used, it is up to
// the entitity that marshaled the original ZSON to ensure that no type-name
// conflicts arise, e.g., when using the TypeSimple decorator style, you cannot
// have a type called bar.Foo and another type baz.Foo as the simple type
// decorator will be "Foo" in both cases and thus create a name conflict.
func (u *UnmarshalZNGContext) Bind(templates ...interface{}) error {
	for _, t := range templates {
		if err := u.binder.enterTemplate(t); err != nil {
			return err
		}
	}
	return nil
}

func (u *UnmarshalZNGContext) NamedBindings(bindings []Binding) error {
	for _, b := range bindings {
		if err := u.binder.enterBinding(b); err != nil {
			return err
		}
	}
	return nil
}

func (u *UnmarshalZNGContext) decodeAny(zv zed.Value, v reflect.Value) error {
	if !v.IsValid() {
		return errors.New("cannot unmarshal into value provided")
	}
	if v.Type().Implements(unmarshalerTypeZNG) {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return v.Interface().(ZNGUnmarshaler).UnmarshalZNG(u, zv)
	}
	if v.CanAddr() {
		pv := v.Addr()
		if pv.CanInterface() && pv.Type().Implements(unmarshalerTypeZNG) {
			return pv.Interface().(ZNGUnmarshaler).UnmarshalZNG(u, zv)
		}
	}
	if _, ok := v.Interface().(nano.Ts); ok {
		if zv.Type != zed.TypeTime {
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeTime(zv.Bytes)
		v.Set(reflect.ValueOf(x))
		return err
	}
	if _, ok := v.Interface().(zed.Value); ok {
		// For zed.Values we simply set the reflect value to the
		// zed.Value that has been decoded.
		v.Set(reflect.ValueOf(zv))
		return nil
	}
	switch v.Kind() {
	case reflect.Array:
		return u.decodeArray(zv, v)
	case reflect.Slice:
		if isIP(v.Type()) {
			return u.decodeIP(zv, v)
		}
		return u.decodeArray(zv, v)
	case reflect.Struct:
		return u.decodeRecord(zv, v)
	case reflect.Interface:
		// This is an interface value.  If the underlying zng data
		// has a type name (via alias), then we'll see if there's a
		// binding fot it and unmarshal into an instance of Template
		// in the binding.
		typ, err := u.lookupType(zv.Type)
		if err != nil {
			return err
		}
		if typ == nil {
			// If typ is nil, then the value must be of ZNG type null
			// and ZNG type values can only have value null.  So, we
			// set it to null of the type given for the marshaled-into
			// value and return.
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		concrete := reflect.New(typ)
		if err := u.decodeAny(zv, concrete.Elem()); err != nil {
			return err
		}
		// For empty interface, we pull the value pointed-at into the
		// empty-interface value if it's not a struct (i.e., a scalar or
		// a slice)  For normal interfaces, we set the pointer to be
		// the pointer to the new object as it must be type-compatible.
		if v.NumMethod() == 0 && concrete.Elem().Kind() != reflect.Struct {
			v.Set(concrete.Elem())
		} else {
			v.Set(concrete)
		}
		return nil
	case reflect.Ptr:
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return u.decodeAny(zv, v.Elem())
	case reflect.String:
		// XXX We bundle string, bstring, type, error all into string.
		// See issue #1853.
		switch zed.AliasOf(zv.Type) {
		case zed.TypeString, zed.TypeBstring, zed.TypeType, zed.TypeError:
		default:
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeString(zv.Bytes)
		v.SetString(x)
		return err
	case reflect.Bool:
		if zed.AliasOf(zv.Type) != zed.TypeBool {
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeBool(zv.Bytes)
		v.SetBool(x)
		return err
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch zed.AliasOf(zv.Type) {
		case zed.TypeInt8, zed.TypeInt16, zed.TypeInt32, zed.TypeInt64:
		default:
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeInt(zv.Bytes)
		v.SetInt(x)
		return err
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch zed.AliasOf(zv.Type) {
		case zed.TypeUint8, zed.TypeUint16, zed.TypeUint32, zed.TypeUint64:
		default:
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeUint(zv.Bytes)
		v.SetUint(x)
		return err
	case reflect.Float32, reflect.Float64:
		// TODO: zed.TypeFloat32 when it lands
		switch zed.AliasOf(zv.Type) {
		case zed.TypeFloat64:
		default:
			return incompatTypeError(zv.Type, v)
		}
		if zv.Bytes == nil {
			v.Set(reflect.Zero(v.Type()))
			return nil
		}
		x, err := zed.DecodeFloat64(zv.Bytes)
		v.SetFloat(x)
		return err
	default:
		return fmt.Errorf("unsupported type: %v", v.Kind())
	}
}

func isIP(typ reflect.Type) bool {
	return typ.Name() == "IP" && typ.PkgPath() == "net"
}

func (u *UnmarshalZNGContext) decodeIP(zv zed.Value, v reflect.Value) error {
	if zed.AliasOf(zv.Type) != zed.TypeIP {
		return incompatTypeError(zv.Type, v)
	}
	if zv.Bytes == nil {
		v.Set(reflect.Zero(v.Type()))
		return nil
	}
	x, err := zed.DecodeIP(zv.Bytes)
	v.Set(reflect.ValueOf(x))
	return err
}

func (u *UnmarshalZNGContext) decodeRecord(zv zed.Value, sval reflect.Value) error {
	recType, ok := zed.AliasOf(zv.Type).(*zed.TypeRecord)
	if !ok {
		return fmt.Errorf("cannot unmarshal Zed type %q into Go struct", FormatType(zv.Type))
	}
	nameToField := make(map[string]int)
	stype := sval.Type()
	for i := 0; i < stype.NumField(); i++ {
		field := stype.Field(i)
		name := fieldName(field)
		nameToField[name] = i
	}
	for i, it := 0, zv.Iter(); !it.Done(); i++ {
		if i >= len(recType.Columns) {
			return zed.ErrMismatch
		}
		itzv, _, err := it.Next()
		if err != nil {
			return err
		}
		name := recType.Columns[i].Name
		if fieldIdx, ok := nameToField[name]; ok {
			typ := recType.Columns[i].Type
			if err := u.decodeAny(zed.Value{typ, itzv}, sval.Field(fieldIdx)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (u *UnmarshalZNGContext) decodeArray(zv zed.Value, arrVal reflect.Value) error {
	typ := zed.AliasOf(zv.Type)
	if typ == zed.TypeBytes && arrVal.Type().Elem().Kind() == reflect.Uint8 {
		if zv.Bytes == nil {
			return nil
		}
		if arrVal.Kind() == reflect.Array {
			return u.decodeArrayBytes(zv, arrVal)
		}
		// arrVal is a slice here.
		arrVal.SetBytes(zv.Bytes)
		return nil
	}
	arrType, ok := zed.AliasOf(zv.Type).(*zed.TypeArray)
	if !ok {
		return errors.New("not an array")
	}
	if zv.Bytes == nil {
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
		if err := u.decodeAny(zed.Value{arrType.Type, itzv}, arrVal.Index(i)); err != nil {
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

func (u *UnmarshalZNGContext) decodeArrayBytes(zv zed.Value, arrayVal reflect.Value) error {
	if len(zv.Bytes) != arrayVal.Len() {
		return errors.New("ZNG bytes value length differs from Go array")
	}
	for k, b := range zv.Bytes {
		arrayVal.Index(k).Set(reflect.ValueOf(b))
	}
	return nil
}

type Binding struct {
	Name     string      // user-defined name
	Template interface{} // zero-valued entity used as template for new such objects
}

type binding struct {
	key      string
	template reflect.Type
}

type binder map[string][]binding

func (b binder) lookup(name string) reflect.Type {
	if b == nil {
		return nil
	}
	for _, binding := range b[name] {
		if binding.key == name {
			return binding.template
		}
	}
	return nil
}

func (b *binder) enter(key string, typ reflect.Type) error {
	if *b == nil {
		*b = make(map[string][]binding)
	}
	slot := (*b)[key]
	entry := binding{
		key:      key,
		template: typ,
	}
	(*b)[key] = append(slot, entry)
	return nil
}

func (b *binder) enterTemplate(template interface{}) error {
	typ, err := typeOfTemplate(template)
	if err != nil {
		return err
	}
	pkgPath := typ.PkgPath()
	path := strings.Split(pkgPath, "/")
	pkgName := path[len(path)-1]

	// e.g., Foo
	typeName := typ.Name()
	// e.g., bar.Foo
	dottedName := fmt.Sprintf("%s.%s", pkgName, typeName)
	// e.g., github.com/acme/pkg/bar.Foo
	fullName := fmt.Sprintf("%s.%s", pkgPath, typeName)

	if err := b.enter(typeName, typ); err != nil {
		return err
	}
	if err := b.enter(dottedName, typ); err != nil {
		return err
	}
	return b.enter(fullName, typ)
}

func (b *binder) enterBinding(binding Binding) error {
	typ, err := typeOfTemplate(binding.Template)
	if err != nil {
		return err
	}
	return b.enter(binding.Name, typ)
}

func typeOfTemplate(template interface{}) (reflect.Type, error) {
	v := reflect.ValueOf(template)
	if !v.IsValid() {
		return nil, errors.New("invalid template")
	}
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	return v.Type(), nil
}

func typeNameOfValue(value interface{}) (string, error) {
	typ, err := typeOfTemplate(value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", typ.PkgPath(), typ.Name()), nil
}

func (u *UnmarshalZNGContext) lookupType(typ zed.Type) (reflect.Type, error) {
	switch typ := typ.(type) {
	case *zed.TypeAlias:
		if template := u.binder.lookup(typ.Name); template != nil {
			return template, nil
		}
		// Ignore aliases for which there are no bindings.
		// If an interface type being marshaled into doesn't
		// have a binding, then a type mismatch will be caught
		// by reflect when the Set() method is called on the
		// value and the concrete value doesn't implement the
		// interface.
		return u.lookupType(typ.Type)
	case *zed.TypeRecord:
		return u.lookupTypeRecord(typ)
	case *zed.TypeArray:
		elemType, err := u.lookupType(typ.Type)
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(elemType), nil
	case *zed.TypeSet:
		elemType, err := u.lookupType(typ.Type)
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(elemType), nil
	case *zed.TypeUnion, *zed.TypeEnum:
		// For now just return nil here. The layer above will flag
		// a type error.  At some point, we can create Go-native data structures
		// in package zng for representing a union or enum as a standalone
		// entity.  See issue #1853.
		return nil, nil
	case *zed.TypeMap:
		keyType, err := u.lookupType(typ.KeyType)
		if err != nil {
			return nil, err
		}
		valType, err := u.lookupType(typ.ValType)
		if err != nil {
			return nil, err
		}
		return reflect.MapOf(keyType, valType), nil
	default:
		return u.lookupPrimitiveType(typ)
	}
}

func (u *UnmarshalZNGContext) lookupTypeRecord(typ *zed.TypeRecord) (reflect.Type, error) {
	return nil, errors.New("unmarshaling records into interface value requires type binding")
}

func (u *UnmarshalZNGContext) lookupPrimitiveType(typ zed.Type) (reflect.Type, error) {
	var v interface{}
	switch typ := typ.(type) {
	// XXX We should have counterparts for bstring, error, and type type.
	// See issue #1853.
	case *zed.TypeOfString, *zed.TypeOfBstring, *zed.TypeOfError, *zed.TypeOfType:
		v = ""
	case *zed.TypeOfBool:
		v = false
	case *zed.TypeOfUint8:
		v = uint8(0)
	case *zed.TypeOfUint16:
		v = uint16(0)
	case *zed.TypeOfUint32:
		v = uint32(0)
	case *zed.TypeOfUint64:
		v = uint64(0)
	case *zed.TypeOfInt8:
		v = int8(0)
	case *zed.TypeOfInt16:
		v = int16(0)
	case *zed.TypeOfInt32:
		v = int32(0)
	case *zed.TypeOfInt64:
		v = int64(0)
	// TODO: zed.TypeFloat32 when it lands
	case *zed.TypeOfFloat64:
		v = float64(0)
	case *zed.TypeOfIP:
		v = net.IP{}
	case *zed.TypeOfNet:
		v = net.IPNet{}
	case *zed.TypeOfTime:
		v = time.Time{}
	case *zed.TypeOfDuration:
		v = time.Duration(0)
	case *zed.TypeOfNull:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown zng type: %v", typ)
	}
	return reflect.TypeOf(v), nil
}
