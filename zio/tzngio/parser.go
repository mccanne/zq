package tzngio

import (
	"encoding/base64"
	"errors"
	"net"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/pkg/byteconv"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrUnterminated = errors.New("tzng syntax error: unterminated container")
	ErrSyntax       = errors.New("tzng syntax error")
)

type Parser struct {
	zcode.Builder
}

func NewParser() *Parser {
	return &Parser{*zcode.NewBuilder()}
}

// Parse decodes a zng value in text format using the type information
// in the descriptor.  Once parsed, the resulting zcode.Bytes has
// the nested data structure encoded independently of the data type.
func (p *Parser) Parse(typ *zng.TypeRecord, zng []byte) (zcode.Bytes, error) {
	p.Reset()
	if zng[0] != leftbracket {
		return nil, ErrSyntax
	}
	rest, err := p.ParseContainer(typ, zng)
	if err != nil {
		return nil, err
	}
	if len(rest) > 0 {
		return nil, ErrSyntax
	}
	return p.Bytes().ContainerBody()
}

const (
	semicolon    = byte(';')
	leftbracket  = byte('[')
	rightbracket = byte(']')
	backslash    = byte('\\')
)

// ParseContainer parses the given byte array representing a container
// in the zng format.
func (p *Parser) ParseContainer(typ zng.Type, b []byte) ([]byte, error) {
	realType := zng.AliasOf(typ)
	// This is hokey.
	var keyType, valType zng.Type
	if typ, ok := realType.(*zng.TypeMap); ok {
		keyType = typ.KeyType
		valType = typ.ValType
	}
	p.BeginContainer()
	// skip leftbracket
	b = b[1:]
	childType, columns := zng.ContainedType(realType)
	if childType == nil && columns == nil && keyType == nil {
		return nil, zng.ErrNotPrimitive
	}

	k := 0
	for {
		if len(b) == 0 {
			return nil, ErrUnterminated
		}
		if b[0] == rightbracket {
			if _, ok := realType.(*zng.TypeSet); ok {
				p.TransformContainer(zng.NormalizeSet)
			}
			if _, ok := realType.(*zng.TypeMap); ok {
				p.TransformContainer(zng.NormalizeMap)
			}
			p.EndContainer()
			return b[1:], nil
		}
		if columns != nil {
			if k >= len(columns) {
				return nil, &zng.RecordTypeError{Name: "<record>", Type: typ.ZSON(), Err: zng.ErrExtraField}
			}
			childType = columns[k].Type
			k++
		}
		if keyType != nil {
			if (k & 1) == 0 {
				childType = keyType
			} else {
				childType = valType
			}
			k++
		}
		rest, err := p.ParseField(childType, b)
		if err != nil {
			return nil, err
		}
		b = rest
	}
}

// ParseField parses the given byte array representing any value
// in the zng format.
func (p *Parser) ParseField(typ zng.Type, b []byte) ([]byte, error) {
	realType := zng.AliasOf(typ)
	var err error
	var index int
	if len(b) >= 2 && b[0] == '-' && b[1] == ';' {
		if zng.IsContainerType(realType) {
			p.AppendContainer(nil)
		} else {
			p.AppendPrimitive(nil)
		}
		return b[2:], nil
	}
	if utyp, ok := realType.(*zng.TypeUnion); ok {
		var childType zng.Type
		childType, index, b, err = utyp.SplitTzng(b)
		if err != nil {
			return nil, err
		}
		p.BeginContainer()
		defer p.EndContainer()
		p.AppendPrimitive(zng.EncodeInt(int64(index)))
		return p.ParseField(childType, b)
	}
	if b[0] == leftbracket {
		return p.ParseContainer(typ, b)
	}
	if zng.IsContainerType(realType) {
		return nil, zng.ErrNotContainer
	}

	// We don't actually need to handle escapes here, type.Parse()
	// will take care of that.  The important thing is just figuring
	// out the proper boundary between individual records (skipping
	// over an escaped semicolon without being tricked by something
	// like \\; which could be an escaped backslash followed by a
	// real semicolon)
	i := 0
	for ; i < len(b); i++ {
		if b[i] == semicolon {
			break
		}
		if b[i] == backslash {
			i++
		}
	}
	if i == len(b) {
		return nil, ErrUnterminated
	}

	zv, err := ParseValue(realType, b[:i])
	if err != nil {
		return nil, err
	}
	p.AppendPrimitive(zv)
	return b[i+1:], nil
}

func ParseContainer(typ zng.Type, in []byte) (zcode.Bytes, error) {
	p := NewParser()
	_, err := p.ParseContainer(typ, in)
	if err != nil {
		return nil, err
	}
	return p.Bytes().ContainerBody()
}

func ParseMap(t *zng.TypeMap, in []byte) (zcode.Bytes, error) {
	return ParseContainer(t, in)
}

func ParseRecord(t *zng.TypeRecord, in []byte) (zcode.Bytes, error) {
	return ParseContainer(t, in)
}

func ParseSet(t *zng.TypeSet, in []byte) (zcode.Bytes, error) {
	return ParseContainer(t, in)
}

func ParseUnion(t *zng.TypeUnion, in []byte) (zcode.Bytes, error) {
	return ParseContainer(t, in)
}

func ParseBstring(in []byte) (zcode.Bytes, error) {
	normalized := norm.NFC.Bytes(zng.UnescapeBstring(in))
	return normalized, nil
}

func ParseString(in []byte) (zcode.Bytes, error) {
	normalized := norm.NFC.Bytes(UnescapeString(in))
	return normalized, nil
}

func ParseBool(in []byte) (zcode.Bytes, error) {
	b, err := byteconv.ParseBool(in)
	if err != nil {
		return nil, err
	}
	return zng.EncodeBool(b), nil
}

func ParseBytes(in []byte) (zcode.Bytes, error) {
	s := string(in)
	if s == "" {
		return []byte{}, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return zcode.Bytes(b), nil
}

func ParseDuration(in []byte) (zcode.Bytes, error) {
	d, err := nano.Parse(in) // zeek-style, full 64-bit ns fractional number
	if err != nil {
		return nil, err
	}
	return zng.EncodeDuration(nano.Duration(d)), nil
}

func ParseIP(in []byte) (zcode.Bytes, error) {
	ip, err := byteconv.ParseIP(in)
	if err != nil {
		return nil, err
	}
	return zng.EncodeIP(ip), nil
}

func ParseNet(in []byte) (zcode.Bytes, error) {
	_, subnet, err := net.ParseCIDR(string(in))
	if err != nil {
		return nil, err
	}
	return zng.EncodeNet(subnet), nil
}

func ParseTime(in []byte) (zcode.Bytes, error) {
	ts, err := nano.Parse(in)
	if err != nil {
		return nil, err
	}
	return zng.EncodeTime(ts), nil
}

func ParseValue(typ zng.Type, in []byte) (zcode.Bytes, error) {
	switch typ := typ.(type) {
	case *zng.TypeAlias:
		return ParseValue(typ.Type, in)
	case *zng.TypeRecord:
		return ParseRecord(typ, in)
	case *zng.TypeArray, *zng.TypeSet, *zng.TypeUnion, *zng.TypeMap:
		return ParseContainer(typ, in)
	case *zng.TypeEnum:
		return ParseValue(zng.TypeUint64, in)
	case *zng.TypeOfBool:
		return ParseBool(in)
	case *zng.TypeOfBytes:
		return ParseBytes(in)
	case *zng.TypeOfBstring:
		return ParseBstring(in)
	case *zng.TypeOfDuration:
		return ParseDuration(in)
	case *zng.TypeOfIP:
		return ParseIP(in)
	case *zng.TypeOfNet:
		return ParseNet(in)
	case *zng.TypeOfString:
		return ParseString(in)
	case *zng.TypeOfTime:
		return ParseTime(in)
	default:
		primitive := zed.Primitive{
			Kind: "Primitive",
			Type: typ.ZSON(),
			Text: string(in),
		}
		zv, err := zson.ParsePrimitive(primitive.Type, primitive.Text)
		if err != nil {
			return nil, err
		}
		return zv.Bytes, nil
	}
}
