// Package zson provides fundamental interfaces to the ZSON data format comprising
// Reader, Writer, Parser, and so forth.  The ZSON format includes a type system
// that requries a semantic analysis to parse an input to its structured data
// representation.  To do so, Parser translats a ZSON input to an AST, Analyzer
// performs semantic type analysis to turn the AST into a Value, and Builder
// constructs a zed.Value from a Value.
package zson

import (
	"strings"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/zcode"
)

// Implied returns true for primitive types whose type can be inferred
// syntactically from its value and thus never needs a decorator.
func Implied(typ zed.Type) bool {
	switch typ := typ.(type) {
	case *zed.TypeOfInt64, *zed.TypeOfDuration, *zed.TypeOfTime, *zed.TypeOfFloat64, *zed.TypeOfBool, *zed.TypeOfBytes, *zed.TypeOfString, *zed.TypeOfIP, *zed.TypeOfNet, *zed.TypeOfType, *zed.TypeOfNull:
		return true
	case *zed.TypeRecord:
		for _, c := range typ.Columns {
			if !Implied(c.Type) {
				return false
			}
		}
		return true
	case *zed.TypeArray:
		return Implied(typ.Type)
	case *zed.TypeSet:
		return Implied(typ.Type)
	case *zed.TypeMap:
		return Implied(typ.KeyType) && Implied(typ.ValType)
	}
	return false
}

// SelfDescribing returns true for types whose type name can be entirely derived
// from its typed value, e.g., a record type can be derived from a record value
// because all of the field names and type names are present in the value, but
// an enum type cannot be derived from an enum value because not all the enumerated
// names are present in the value.  In the former case, a decorated typedef can
// use the abbreviated form "(= <name>)", while the letter case, a type def must use
// the longer form "<value> (<name> = (<type>))".
func SelfDescribing(typ zed.Type) bool {
	if Implied(typ) {
		return true
	}
	switch typ := typ.(type) {
	case *zed.TypeRecord, *zed.TypeArray, *zed.TypeSet, *zed.TypeMap:
		return true
	case *zed.TypeAlias:
		return SelfDescribing(typ.Type)
	}
	return false
}

func ParseType(zctx *Context, zson string) (zed.Type, error) {
	zp, err := NewParser(strings.NewReader(zson))
	if err != nil {
		return nil, err
	}
	ast, err := zp.parseType()
	if ast == nil || noEOF(err) != nil {
		return nil, err
	}
	return NewAnalyzer().convertType(zctx, ast)
}

func ParseValue(zctx *Context, zson string) (zed.Value, error) {
	zp, err := NewParser(strings.NewReader(zson))
	if err != nil {
		return zed.Value{}, err
	}
	ast, err := zp.ParseValue()
	if err != nil {
		return zed.Value{}, err
	}
	val, err := NewAnalyzer().ConvertValue(zctx, ast)
	if err != nil {
		return zed.Value{}, err
	}
	return Build(zcode.NewBuilder(), val)
}

func ParseValueFromAST(zctx *Context, ast zed.Value) (zed.Value, error) {
	val, err := NewAnalyzer().ConvertValue(zctx, ast)
	if err != nil {
		return zed.Value{}, err
	}
	return Build(zcode.NewBuilder(), val)
}

func TranslateType(zctx *Context, astType zed.Type) (zed.Type, error) {
	return NewAnalyzer().convertType(zctx, astType)
}
