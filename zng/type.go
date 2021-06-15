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

	"github.com/brimdata/zed/zcode"
)

var (
	ErrLenUnset   = errors.New("len(unset) is undefined")
	ErrNotArray   = errors.New("cannot index a non-array")
	ErrIndex      = errors.New("array index out of bounds")
	ErrUnionIndex = errors.New("union index out of bounds")
	ErrEnumIndex  = errors.New("enum index out of bounds")
)

// A Type is an interface presented by a zeek type.
// Types can be used to infer type compatibility and create new values
// of the underlying type.
type Type interface {
	Marshal(zcode.Bytes) (interface{}, error)
	// ID returns a unique (per zson.Context) identifier that
	// represents this type.  For an aliased type, this identifier
	// represents the actual underlying type and not the alias itself.
	// Callers that care about the underlying type of a zng.Value for
	// example should prefer to use this instead of using the go
	// .(type) operator on a zng.Type instance.
	ID() int
	String() string
	Format(zv zcode.Bytes) string
}

var (
	TypeUint8    = &TypeOfUint8{}
	TypeUint16   = &TypeOfUint16{}
	TypeUint32   = &TypeOfUint32{}
	TypeUint64   = &TypeOfUint64{}
	TypeInt8     = &TypeOfInt8{}
	TypeInt16    = &TypeOfInt16{}
	TypeInt32    = &TypeOfInt32{}
	TypeInt64    = &TypeOfInt64{}
	TypeDuration = &TypeOfDuration{}
	TypeTime     = &TypeOfTime{}
	// XXX add TypeFloat16
	// XXX add TypeFloat32
	TypeFloat64 = &TypeOfFloat64{}
	// XXX add TypeDecimal
	TypeBool    = &TypeOfBool{}
	TypeBytes   = &TypeOfBytes{}
	TypeString  = &TypeOfString{}
	TypeBstring = &TypeOfBstring{}
	TypeIP      = &TypeOfIP{}
	TypeNet     = &TypeOfNet{}
	TypeType    = &TypeOfType{}
	TypeError   = &TypeOfError{}
	TypeNull    = &TypeOfNull{}
)

const (
	IDUint8    = 0
	IDUint16   = 1
	IDUint32   = 2
	IDUint64   = 3
	IDInt8     = 4
	IDInt16    = 5
	IDInt32    = 6
	IDInt64    = 7
	IDDuration = 8
	IDTime     = 9
	IDFloat16  = 10
	IDFloat32  = 11
	IDFloat64  = 12
	IDDecimal  = 13
	IDBool     = 14
	IDBytes    = 15
	IDString   = 16
	IDBstring  = 17
	IDIP       = 18
	IDNet      = 19
	IDType     = 20
	IDError    = 21
	IDNull     = 22

	IDTypeDef = 23

	IDTypeName = 24
	IDRecord   = 25
	IDArray    = 26
	IDSet      = 27
	IDUnion    = 28
	IDEnum     = 29
	IDMap      = 30
)

var promote = []int{
	IDInt8,    // IDUint8    = 0
	IDInt16,   // IDUint16   = 1
	IDInt32,   // IDUint32   = 2
	IDInt64,   // IDUint64   = 3
	IDInt8,    // IDInt8     = 4
	IDInt16,   // IDInt16    = 5
	IDInt32,   // IDInt32    = 6
	IDInt64,   // IDInt64    = 7
	IDInt64,   // IDDuration = 8
	IDInt64,   // IDTime     = 9
	IDFloat16, // IDFloat32  = 10
	IDFloat32, // IDFloat32  = 11
	IDFloat64, // IDFloat64  = 12
	IDDecimal, // IDDecimal  = 13
}

// Promote type to the largest signed type where the IDs must both
// satisfy IsNumber.
func PromoteInt(aid, bid int) int {
	id := promote[aid]
	if bid := promote[bid]; bid > id {
		id = bid
	}
	return id
}

// True iff the type id is encoded as a zng signed or unsigened integer zcode.Bytes.
func IsInteger(id int) bool {
	return id <= IDInt64
}

// True iff the type id is encoded as a zng signed or unsigned integer zcode.Bytes,
// float32 zcode.Bytes, or float64 zcode.Bytes.
func IsNumber(id int) bool {
	return id <= IDDecimal
}

// True iff the type id is encoded as a float encoding.
// XXX add IDDecimal here when we implement coercible math with it.
func IsFloat(id int) bool {
	return id >= IDFloat16 && id <= IDFloat64
}

// True iff the type id is encoded as a number encoding and is signed.
func IsSigned(id int) bool {
	return id >= IDInt8 && id <= IDTime
}

// True iff the type id is encoded as a string zcode.Bytes.
func IsStringy(id int) bool {
	return id == IDString || id == IDBstring || id == IDError || id == IDType
}

const (
	CtrlValueEscape   = 0xf5
	TypeDefRecord     = 0xf6
	TypeDefArray      = 0xf7
	TypeDefSet        = 0xf8
	TypeDefUnion      = 0xf9
	TypeDefEnum       = 0xfa
	TypeDefMap        = 0xfb
	TypeDefAlias      = 0xfc
	CtrlCompressed    = 0xfd
	CtrlAppMessage    = 0xfe
	CtrlEOS           = 0xff
	AppEncodingZNG    = 0
	AppEncodingJSON   = 1
	AppEncodingZSON   = 2
	AppEncodingString = 3
	AppEncodingBinary = 4
)

type CompressionFormat int

const CompressionFormatLZ4 CompressionFormat = 0x00

func LookupPrimitive(name string) Type {
	switch name {
	case "uint8":
		return TypeUint8
	case "uint16":
		return TypeUint16
	case "uint32":
		return TypeUint32
	case "uint64":
		return TypeUint64
	case "int8":
		return TypeInt8
	case "int16":
		return TypeInt16
	case "int32":
		return TypeInt32
	case "int64":
		return TypeInt64
	case "duration":
		return TypeDuration
	case "time":
		return TypeTime
	case "float64":
		return TypeFloat64
	case "bool":
		return TypeBool
	case "bytes":
		return TypeBytes
	case "string":
		return TypeString
	case "bstring":
		return TypeBstring
	case "ip":
		return TypeIP
	case "net":
		return TypeNet
	case "type":
		return TypeType
	case "error":
		return TypeError
	case "null":
		return TypeNull
	}
	return nil
}

func LookupPrimitiveByID(id int) Type {
	switch id {
	case IDBool:
		return TypeBool
	case IDInt8:
		return TypeInt8
	case IDUint8:
		return TypeUint8
	case IDInt16:
		return TypeInt16
	case IDUint16:
		return TypeUint16
	case IDInt32:
		return TypeInt32
	case IDUint32:
		return TypeUint32
	case IDInt64:
		return TypeInt64
	case IDUint64:
		return TypeUint64
	case IDFloat64:
		return TypeFloat64
	case IDBytes:
		return TypeBytes
	case IDString:
		return TypeString
	case IDBstring:
		return TypeBstring
	case IDIP:
		return TypeIP
	case IDNet:
		return TypeNet
	case IDTime:
		return TypeTime
	case IDDuration:
		return TypeDuration
	case IDType:
		return TypeType
	case IDError:
		return TypeError
	case IDNull:
		return TypeNull
	}
	return nil
}

// SameType returns true if the two types are equal in that each interface
// points to the same underlying type object.  Because the zeek library
// creates each unique type only once, this pointer comparison works.  If types
// are created outside of the zeek package, then SameType will not work in general
// for them.
func xSameType(t1, t2 Type) bool {
	return t1 == t2
}

// Utilities shared by complex types (ie, set and array)

// InnerType returns the element type for the underlying set or array type or
// nil if the underlying type is not a set or array.
func InnerType(typ Type) Type {
	switch typ := AliasOf(typ).(type) {
	case *TypeSet:
		return typ.Type
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
		return typ.Type, nil
	case *TypeArray:
		return typ.Type, nil
	case *TypeRecord:
		return nil, typ.Columns
		// XXX enum?
	default:
		return nil, nil
	}
}

func IsUnionType(typ Type) bool {
	_, ok := typ.(*TypeUnion)
	return ok
}

func IsRecordType(typ Type) bool {
	_, ok := AliasOf(typ).(*TypeRecord)
	return ok
}

func TypeRecordOf(typ Type) *TypeRecord {
	t, _ := AliasOf(typ).(*TypeRecord)
	return t
}

func IsContainerType(typ Type) bool {
	switch typ := typ.(type) {
	case *TypeAlias:
		return IsContainerType(typ.Type)
	case *TypeSet, *TypeArray, *TypeRecord, *TypeUnion, *TypeMap:
		return true
	default:
		return false
	}
}

func IsPrimitiveType(typ Type) bool {
	return !IsContainerType(typ)
}

//XXX get rid of this?
func AliasTypes(typ Type) []*TypeAlias {
	var aliases []*TypeAlias
	switch typ := typ.(type) {
	case *TypeSet:
		aliases = AliasTypes(typ.Type)
	case *TypeArray:
		aliases = AliasTypes(typ.Type)
	case *TypeRecord:
		for _, col := range typ.Columns {
			aliases = append(aliases, AliasTypes(col.Type)...)
		}
	case *TypeUnion:
		for _, typ := range typ.Types {
			aliases = append(aliases, AliasTypes(typ)...)
		}
	case *TypeEnum:
		aliases = AliasTypes(typ.Type)
	case *TypeMap:
		keyAliases := AliasTypes(typ.KeyType)
		valAliases := AliasTypes(typ.KeyType)
		aliases = append(keyAliases, valAliases...)
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

// ReferencedID returns the underlying type from the given referential type.
// e.g., aliases and enums both refer to other underlying types and the
// Value's Bytes field is encoded according to the underlying type.
// XXX we initially had this as a method on Type but it was removed in favor
// of TypeAlias.ID() returning the underlying type where code that cared
// has to check if the type is an alias and use TypeAlias.AliasID().  Now that
// we have enums we need to implement a similar workaround.  It seems like we
// should add back the Type method that we took out.
func ReferencedID(typ Type) int {
	if typ, ok := typ.(*TypeEnum); ok {
		return typ.Type.ID()
	}
	return typ.ID()
}

func TypeID(typ Type) int {
	if alias, ok := typ.(*TypeAlias); ok {
		return alias.id
	}
	return typ.ID()
}
