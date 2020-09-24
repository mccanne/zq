// Package zng implements a data typing system based on the zeek type system.
// All zeek types are defined here and implement the Type interface while instances
// of values implement the Value interface.  All values conform to exactly one type.
// The package provides a fast-path for comparing a value to a byte slice
// without having to create a zeek value from the byte slice.  To exploit this,
// all values include a Comparison method that returns a Predicate function that
// takes a byte slice and a Type and returns a boolean indicating whether the
// the byte slice with the indicated Type matches the value.  The package also
// provides mechanism for coercing values in well-defined and natural ways.
package zng

import (
	"errors"
	"strings"

	"github.com/brimsec/zq/zcode"
)

var (
	ErrUnset      = errors.New("value is unset")
	ErrLenUnset   = errors.New("len(unset) is undefined")
	ErrNotArray   = errors.New("cannot index a non-array")
	ErrIndex      = errors.New("array index out of bounds")
	ErrUnionIndex = errors.New("union index out of bounds")
)

// Resolver is an interface for looking up Type objects from the type id.
type Resolver interface {
	//XXX TypeRecord for now
	Lookup(int) *TypeRecord
}

// The fmt paramter passed to Type.StringOf() must be one of the following
// values, these are used to inform the formatter how containers should be
// encoded and what sort of escaping should be applied to string types.
type OutFmt int

const (
	OutFormatUnescaped = OutFmt(iota)
	OutFormatZNG
	OutFormatZeek
	OutFormatZeekAscii
	OutFormatDebug
)

// A Type is an interface presented by a zeek type.
// Types can be used to infer type compatibility and create new values
// of the underlying type.
type Type interface {
	// String returns the name of the type as defined in the ZNG spec.
	String() string
	// StringOf formats an arbitrary value of this type encoded as zcode.
	// The fmt parameter controls output formatting.  The inContainer
	// parameter indicates if this value is inside a set or vector
	// (which is needed to correctly implement  zeek log escaping rules).
	StringOf(zv zcode.Bytes, fmt OutFmt, inContainer bool) string
	// Marshal is used from Value.MarshalJSON(), it should turn an
	// arbitrary value of this type encoded as zcode into something
	// suitable for passing to json.Marshal()
	Marshal(zcode.Bytes) (interface{}, error)
	// Parse transforms a string representation of the type to its zval
	// encoding.  The string input is provided as a byte slice for
	// efficiency given the common use cases in the system.
	Parse([]byte) (zcode.Bytes, error)
	// ID returns a unique (per resolver.Context) identifier that
	// represents this type.  For an aliased type, this identifier
	// represents the actual underlying type and not the alias itself.
	// Callers that care about the underlying type of a zng.Value for
	// example should prefer to use this instead of using the go
	// .(type) operator on a zng.Type instance.
	ID() int
}

var (
	TypeBool     = &TypeOfBool{}
	TypeByte     = &TypeOfByte{}
	TypeInt16    = &TypeOfInt16{}
	TypeUint16   = &TypeOfUint16{}
	TypeInt32    = &TypeOfInt32{}
	TypeUint32   = &TypeOfUint32{}
	TypeInt64    = &TypeOfInt64{}
	TypeUint64   = &TypeOfUint64{}
	TypeFloat64  = &TypeOfFloat64{}
	TypeString   = &TypeOfString{}
	TypeBytes    = &TypeOfBytes{}
	TypeBstring  = &TypeOfBstring{}
	TypeIP       = &TypeOfIP{}
	TypePort     = &TypeOfPort{}
	TypeNet      = &TypeOfNet{}
	TypeTime     = &TypeOfTime{}
	TypeDuration = &TypeOfDuration{}
	TypeNull     = &TypeOfNull{}
)

const (
	IdBool     = 0
	IdByte     = 1
	IdInt16    = 2
	IdUint16   = 3
	IdInt32    = 4
	IdUint32   = 5
	IdInt64    = 6
	IdUint64   = 7
	IdFloat64  = 8
	IdString   = 9
	IdBytes    = 10
	IdBstring  = 11
	IdEnum     = 12
	IdIP       = 13
	IdPort     = 14
	IdNet      = 15
	IdTime     = 16
	IdDuration = 17
	IdNull     = 18

	IdTypeDef = 23
)

const (
	TypeDefRecord  = 0x80
	TypeDefArray   = 0x81
	TypeDefSet     = 0x82
	TypeDefUnion   = 0x83
	TypeDefAlias   = 0x84
	CtrlEOS        = 0x85
	CtrlCompressed = 0x86
)

type CompressionFormat int

const CompressionFormatLZ4 CompressionFormat = 0x00

func LookupPrimitive(name string) Type {
	switch name {
	case "bool":
		return TypeBool
	case "byte":
		return TypeByte
	case "int16":
		return TypeInt16
	case "uint16":
		return TypeUint16
	case "int32":
		return TypeInt32
	case "uint32":
		return TypeUint32
	case "int64":
		return TypeInt64
	case "uint64":
		return TypeUint64
	case "float64":
		return TypeFloat64
	case "string":
		return TypeString
	case "bytes":
		return TypeBytes
	case "bstring":
		return TypeBstring
	case "ip":
		return TypeIP
	case "port":
		return TypePort
	case "net":
		return TypeNet
	case "time":
		return TypeTime
	case "duration":
		return TypeDuration
	case "null":
		return TypeNull
	}
	return nil
}

func LookupPrimitiveById(id int) Type {
	switch id {
	case IdBool:
		return TypeBool
	case IdByte:
		return TypeByte
	case IdInt16:
		return TypeInt16
	case IdUint16:
		return TypeUint16
	case IdInt32:
		return TypeInt32
	case IdUint32:
		return TypeUint32
	case IdInt64:
		return TypeInt64
	case IdUint64:
		return TypeUint64
	case IdFloat64:
		return TypeFloat64
	case IdString:
		return TypeString
	case IdBytes:
		return TypeBytes
	case IdBstring:
		return TypeBstring
	case IdIP:
		return TypeIP
	case IdPort:
		return TypePort
	case IdNet:
		return TypeNet
	case IdTime:
		return TypeTime
	case IdDuration:
		return TypeDuration
	case IdNull:
		return TypeNull
	}
	return nil
}

// SameType returns true if the two types are equal in that each interface
// points to the same underlying type object.  Because the zeek library
// creates each unique type only once, this pointer comparison works.  If types
// are created outside of the zeek package, then SameType will not work in general
// for them.
func SameType(t1, t2 Type) bool {
	return t1 == t2
}

// Utilities shared by compound types (ie, set and array)

// InnerType returns the element type for set and array types
// or nil if the type is not a set or array.
func InnerType(typ Type) Type {
	switch typ := typ.(type) {
	case *TypeSet:
		return typ.InnerType
	case *TypeArray:
		return typ.Type
	default:
		return nil
	}
}

// ContainedType returns the inner type for set and array types in the first
// return value and the columns of its of type for record types in the second
// return value.  ContainedType returns nil for both return values if the
// type is not a set, array, or record.
func ContainedType(typ Type) (Type, []Column) {
	switch typ := typ.(type) {
	case *TypeSet:
		return typ.InnerType, nil
	case *TypeArray:
		return typ.Type, nil
	case *TypeRecord:
		return nil, typ.Columns
	default:
		return nil, nil
	}
}

func IsUnionType(typ Type) bool {
	_, ok := typ.(*TypeUnion)
	return ok
}

func IsContainerType(typ Type) bool {
	switch typ := typ.(type) {
	case *TypeAlias:
		return IsContainerType(typ.Type)
	case *TypeSet, *TypeArray, *TypeRecord, *TypeUnion:
		return true
	default:
		return false
	}
}

func AliasTypes(typ Type) []*TypeAlias {
	var aliases []*TypeAlias
	switch typ := typ.(type) {
	case *TypeSet:
		aliases = AliasTypes(typ.InnerType)
	case *TypeArray:
		aliases = AliasTypes(typ.Type)
	case *TypeRecord:
		for _, col := range typ.Columns {
			aliases = append(aliases, AliasTypes(col.Type)...)
		}
	case *TypeAlias:
		aliases = append(aliases, AliasTypes(typ.Type)...)
		aliases = append(aliases, typ)
	}
	return aliases
}

func trimInnerTypes(typ string, raw string) string {
	// XXX handle white space, "set [..."... ?
	innerTypes := strings.TrimPrefix(raw, typ+"[")
	innerTypes = strings.TrimSuffix(innerTypes, "]")
	return innerTypes
}
