package zng

import (
	"errors"
	"fmt"
	"sync"

	"github.com/brimdata/zed/zcode"
)

var (
	ErrAliasExists = errors.New("alias exists with different type")
)

//XXX
type Context struct {
	mu       sync.RWMutex
	byID     []Type
	toType   map[string]Type
	toValue  map[Type]zcode.Bytes
	typedefs map[string]*TypeAlias
}

func NewContext() *Context {
	return &Context{
		byID:     make([]Type, IDTypeDef, 2*IDTypeDef),
		toType:   make(map[string]Type),
		toValue:  make(map[Type]zcode.Bytes),
		typedefs: make(map[string]*TypeAlias),
	}
}

func (c *Context) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID = c.byID[:IDTypeDef]
	c.toType = make(map[string]Type)
	c.toValue = make(map[Type]zcode.Bytes)
	c.typedefs = make(map[string]*TypeAlias)
}

func (c *Context) nextIDWithLock() int {
	return len(c.byID)
}

func (c *Context) LookupType(id int) (Type, error) {
	if id < 0 {
		return nil, fmt.Errorf("type id (%d) cannot be negative", id)
	}
	if id < IDTypeDef {
		return LookupPrimitiveByID(id), nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id >= len(c.byID) {
		return nil, fmt.Errorf("type id (%d) not in type context (size %d)", id, len(c.byID))
	}
	if typ := c.byID[id]; typ != nil {
		return typ, nil
	}
	return nil, fmt.Errorf("no type found for type id %d", id)
}

func (c *Context) Lookup(id int) *TypeRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id >= len(c.byID) {
		return nil
	}
	typ := c.byID[id]
	if typ != nil {
		if typ, ok := typ.(*TypeRecord); ok {
			return typ
		}
	}
	return nil
}

// LookupTypeRecord returns a TypeRecord within this context that binds with the
// indicated columns.  Subsequent calls with the same columns will return the
// same record pointer.  If the type doesn't exist, it's created, stored,
// and returned.  The closure of types within the columns must all be from
// this type context.  If you want to use columns from a different type context,
// use TranslateTypeRecord.
func (c *Context) LookupTypeRecord(columns []Column) (*TypeRecord, error) {
	// First check for duplicate columns
	names := make(map[string]struct{})
	for _, col := range columns {
		_, ok := names[col.Name]
		if ok {
			return nil, fmt.Errorf("duplicate field %s", col.Name)
		}
		names[col.Name] = struct{}{}
	}
	tv := EncodeTypeValue(&TypeRecord{Columns: columns})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeRecord), nil
	}
	dup := make([]Column, 0, len(columns))
	typ := NewTypeRecord(c.nextIDWithLock(), append(dup, columns...))
	c.enterWithLock(tv, typ)
	return typ, nil
}

func (c *Context) MustLookupTypeRecord(columns []Column) *TypeRecord {
	r, err := c.LookupTypeRecord(columns)
	if err != nil {
		panic(err)
	}
	return r
}

func (c *Context) LookupTypeSet(inner Type) *TypeSet {
	tv := EncodeTypeValue(&TypeSet{Type: inner})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeSet)
	}
	typ := NewTypeSet(c.nextIDWithLock(), inner)
	c.enterWithLock(tv, typ)
	return typ
}

func (c *Context) LookupTypeMap(keyType, valType Type) *TypeMap {
	tv := EncodeTypeValue(&TypeMap{KeyType: keyType, ValType: valType})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeMap)
	}
	typ := NewTypeMap(c.nextIDWithLock(), keyType, valType)
	c.enterWithLock(tv, typ)
	return typ
}

func (c *Context) LookupTypeArray(inner Type) *TypeArray {
	tv := EncodeTypeValue(&TypeArray{Type: inner})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeArray)
	}
	typ := NewTypeArray(c.nextIDWithLock(), inner)
	c.enterWithLock(tv, typ)
	return typ
}

func (c *Context) LookupTypeUnion(types []Type) *TypeUnion {
	tv := EncodeTypeValue(&TypeUnion{Types: types})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeUnion)
	}
	typ := NewTypeUnion(c.nextIDWithLock(), types)
	c.enterWithLock(tv, typ)
	return typ
}

func (c *Context) LookupTypeEnum(elemType Type, elements []Element) *TypeEnum {
	tv := EncodeTypeValue(&TypeEnum{Type: elemType, Elements: elements})
	c.mu.Lock()
	defer c.mu.Unlock()
	if typ, ok := c.toType[string(tv)]; ok {
		return typ.(*TypeEnum)
	}
	typ := NewTypeEnum(c.nextIDWithLock(), elemType, elements)
	c.enterWithLock(tv, typ)
	return typ
}

func (c *Context) LookupTypeDef(name string) *TypeAlias {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.typedefs[name]
}

func (c *Context) LookupTypeAlias(name string, target Type) (*TypeAlias, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if alias, ok := c.typedefs[name]; ok {
		if alias.Type == target {
			return alias, nil
		}
	}
	typ := NewTypeAlias(c.nextIDWithLock(), name, target)
	c.typedefs[name] = typ
	c.enterWithLock(EncodeTypeValue(typ), typ)
	return typ, nil
}

// AddColumns returns a new zbuf.Record with columns equal to the given
// record along with new rightmost columns as indicated with the given values.
// If any of the newly provided columns already exists in the specified value,
// an error is returned.
func (c *Context) AddColumns(r *Record, newCols []Column, vals []Value) (*Record, error) {
	oldCols := TypeRecordOf(r.Type).Columns
	outCols := make([]Column, len(oldCols), len(oldCols)+len(newCols))
	copy(outCols, oldCols)
	for _, col := range newCols {
		if r.HasField(string(col.Name)) {
			return nil, fmt.Errorf("field already exists: %s", col.Name)
		}
		outCols = append(outCols, col)
	}
	zv := make(zcode.Bytes, len(r.Bytes))
	copy(zv, r.Bytes)
	for _, val := range vals {
		zv = val.Encode(zv)
	}
	typ, err := c.LookupTypeRecord(outCols)
	if err != nil {
		return nil, err
	}
	return NewRecord(typ, zv), nil
}

// LookupByValue returns the Type indicated by a binary-serialized type value.
// This provides a means to translate a type-context-independent serialized
// encoding for an arbitrary type into the reciever Context.
func (c *Context) LookupByValue(tv zcode.Bytes) (Type, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	typ, ok := c.toType[string(tv)]
	if ok {
		return typ, nil
	}
	c.mu.Unlock()
	typ, err := c.decodeTypeValue(tv)
	c.mu.Lock()
	if err != nil {
		return nil, err
	}
	c.toValue[typ] = tv
	c.toType[string(tv)] = typ
	return typ, nil
}

// TranslateType takes a type from another context and creates and returns that
// type in this context.
func (c *Context) TranslateType(ext Type) (Type, error) {
	return c.LookupByValue(EncodeTypeValue(ext))
}

func (t *Context) TranslateTypeRecord(ext *TypeRecord) (*TypeRecord, error) {
	typ, err := t.TranslateType(ext)
	if err != nil {
		return nil, err
	}
	if typ, ok := typ.(*TypeRecord); ok {
		return typ, nil
	}
	return nil, errors.New("TranslateTypeRecord: system error parsing TypeRecord")
}

func (c *Context) enterWithLock(tv zcode.Bytes, typ Type) {
	c.toValue[typ] = tv
	c.toType[string(tv)] = typ
	c.byID = append(c.byID, typ)
}

func (c *Context) LookupTypeValue(typ Type) Value {
	c.mu.Lock()
	bytes, ok := c.toValue[typ]
	c.mu.Unlock()
	if ok {
		return Value{TypeType, bytes}
	}
	// In general, this shouldn't happen except for a foreign
	// type that wasn't initially created in this context.
	// In this case, we round-trip through the context-independent
	// type value to populate this context with the needed type state.
	typ, err := c.LookupByValue(EncodeTypeValue(typ))
	if err != nil {
		// This shouldn't happen...
		return Missing
	}
	return c.LookupTypeValue(typ)
}

func (c *Context) decodeTypeValue(tv zcode.Bytes) (Type, zcode.Bytes) {
	if len(tv) == 0 {
		return nil, nil
	}
	id := tv[0]
	tv = tv[1:]
	switch id {
	case IDTypeDef:
		name, tv := decodeName(tv)
		if tv == nil {
			return nil, nil
		}
		var typ Type
		typ, tv = c.decodeTypeValue(tv)
		if tv == nil {
			return nil, nil
		}
		alias, err := c.LookupTypeAlias(name, typ)
		if err != nil {
			return nil, nil
		}
		return alias, tv
	case IDTypeName:
		name, tv := decodeName(tv)
		if tv == nil {
			return nil, nil
		}
		typ := c.LookupTypeDef(name)
		if typ == nil {
			return nil, nil
		}
		return typ, tv
	case IDRecord:
		n, tv := decodeInt(tv)
		if tv == nil {
			return nil, nil
		}
		//XXX limit check max cols
		cols := make([]Column, 0, n)
		for k := 0; k < n; k++ {
			var name string
			name, tv = decodeName(tv)
			if tv == nil {
				return nil, nil
			}
			var typ Type
			typ, tv = c.decodeTypeValue(tv)
			if tv == nil {
				return nil, nil
			}
			cols = append(cols, Column{name,typ})
		}
		typ, err := c.LookupTypeRecord(cols)
		if err != nil {
			return nil, nil
		}
		return typ, tv
	case IDArray:
		inner, tv := c.decodeTypeValue(tv)
		if tv == nil {
			return nil, nil
		}
		typ := c.LookupTypeArray(inner)
		if typ == nil {
			return nil, nil
		}
		return typ, tv
	case IDSet:
		inner, tv := c.decodeTypeValue(tv)
		if tv == nil {
			return nil, nil
		}
		typ := c.LookupTypeSet(inner)
		if typ == nil {
			return nil, nil
		}
		return typ, tv
	case IDMap:
		keyType, tv := c.decodeTypeValue(tv)
		if tv == nil {
			return nil, nil
		}
		valType, tv := c.decodeTypeValue(tv)
		if tv == nil {
			return nil, nil
		}
		typ := c.LookupTypeMap(keyType, valType)
		if typ == nil {
			return nil, nil
		}
		return typ, tv
	case IDUnion:
		n, tv := decodeInt(tv)
		if tv == nil {
			return nil, nil
		}
		//XXX limit check n
		types := make([]Type, 0, n)
		for k := 0; k < n; k++ {
			var typ Type
			typ, tv = c.decodeTypeValue(tv)
			types = append(types, typ)
		}
		typ := c.LookupTypeUnion(types)
		if typ == nil {
			return nil, nil
		}
		return typ, tv
	case IDEnum:
		//XXX this is not quite compatible with the current enum format,
		// but we're going to simplify it so, here...
		n, tv := decodeInt(tv)
		if tv == nil {
			return nil, nil
		}
		//XXX limit check n
		for k := 0; k < n; k++ {
			var symbol string
			symbol, tv = decodeName(tv)
			if tv == nil {
				return nil, nil
			}
			b.WriteString(QuotedName(symbol))
		}
		b.WriteByte('>')
	default:
		if id < 0 || id > IDTypeDef {
			b.WriteString(fmt.Sprintf("<ERR bad type ID %d in type value>", id))
			return nil
		}
		typ := LookupPrimitiveByID(int(id))
		b.WriteString(typ.String())
	}
	return tv
}
