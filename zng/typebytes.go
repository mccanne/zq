package zng

import (
	"strings"

	"github.com/brimdata/zed/zcode"
)

func FormatType(typ Type) string {
	var b strings.Builder
	formatType(typ, &b, nil)
	return b.String()
}

func formatType(typ Type, b *strings.Builder, typedefs map[string]Type) {
	switch t := typ.(type) {
	case *TypeAlias:
		name := t.Name
		b.WriteString(name)
		if typedefs == nil {
			typedefs = make(map[string]Type)
		}
		if previous, ok := typedefs[t.Name]; !ok || previous != t {
			typedefs[t.Name] = t
			b.WriteString("=(")
			formatType(t.Type, b, typedefs)
			b.WriteByte(')')
		}
	case *TypeRecord:
		b.WriteByte('{')
		for k, col := range t.Columns {
			if k > 0 {
				b.WriteByte(',')
			}
			b.WriteString(QuotedName(col.Name))
			b.WriteString(":")
			formatType(col.Type, b, typedefs)
		}
		b.WriteByte('}')
	case *TypeArray:
		b.WriteByte('[')
		formatType(t.Type, b, typedefs)
		b.WriteByte(']')
	case *TypeSet:
		b.WriteString("|[")
		formatType(t.Type, b, typedefs)
		b.WriteString("]|")
	case *TypeMap:
		b.WriteString("|{")
		formatType(t.KeyType, b, typedefs)
		b.WriteByte(',')
		formatType(t.ValType, b, typedefs)
		b.WriteString("}|")
	case *TypeUnion:
		b.WriteByte('(')
		for k, typ := range t.Types {
			if k > 0 {
				b.WriteByte(',')
			}
			formatType(typ, b, typedefs)
		}
		b.WriteByte(')')
	case *TypeEnum:
		b.WriteByte('<')
		for k, elem := range t.Elements {
			if k > 0 {
				b.WriteByte(',')
			}
			b.WriteString(QuotedName(elem.Name))
		}
		b.WriteByte('>')
	default:
		b.WriteString(typ.String())
	}
}

func EncodeType(t Type) zcode.Bytes {
	return appendTypeValue(nil, t, nil)
}

func appendTypeValue(b zcode.Bytes, t Type, typedefs map[string]Type) zcode.Bytes {
	switch t := t.(type) {
	case *TypeAlias:
		if typedefs == nil {
			typedefs = make(map[string]Type)
		}
		id := byte(IDTypeDef)
		if previous := typedefs[t.Name]; previous == t {
			id = IDTypeName
		} else {
			typedefs[t.Name] = t
		}
		b = append(b, id)
		b = zcode.AppendUvarint(b, uint64(len(t.Name)))
		b = append(b, zcode.Bytes(t.Name)...)
		if id == IDTypeName {
			return b
		}
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeRecord:
		b = append(b, IDRecord)
		b = zcode.AppendUvarint(b, uint64(len(t.Columns)))
		for _, col := range t.Columns {
			name := []byte(col.Name)
			b = zcode.AppendUvarint(b, uint64(len(name)))
			b = append(b, name...)
			b = appendTypeValue(b, col.Type, typedefs)
		}
		return b
	case *TypeUnion:
		b = append(b, IDUnion)
		b = zcode.AppendUvarint(b, uint64(len(t.Types)))
		for _, t := range t.Types {
			b = appendTypeValue(b, t, typedefs)
		}
		return b
	case *TypeSet:
		b = append(b, IDSet)
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeArray:
		b = append(b, IDArray)
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeEnum:
		b = append(b, IDEnum)
		b = appendTypeValue(b, t.Type, typedefs)
		b = zcode.AppendUvarint(b, uint64(len(t.Elements)))
		container := IsContainerType(t.Type)
		for _, elem := range t.Elements {
			name := []byte(elem.Name)
			b = zcode.AppendUvarint(b, uint64(len(name)))
			b = append(b, name...)
			b = zcode.AppendAs(b, container, elem.Value)
		}
		return b
	case *TypeMap:
		b = append(b, IDMap)
		b = appendTypeValue(b, t.KeyType, typedefs)
		return appendTypeValue(b, t.ValType, typedefs)
	default:
		// Primitive type
		return append(b, byte(t.ID()))
	}
}
