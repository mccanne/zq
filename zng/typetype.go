package zng

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/brimdata/zed/zcode"
)

type TypeOfType struct{}

func (t *TypeOfType) ID() int {
	return IDType
}

func (t *TypeOfType) String() string {
	return "type"
}

func (t *TypeOfType) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Format(zv), nil
}

func (t *TypeOfType) Format(zv zcode.Bytes) string {
	return fmt.Sprintf("(%s)", FormatTypeValue(zv))
}

func EncodeTypeValue(t Type) zcode.Bytes {
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
		//container := IsContainerType(t.Type)
		for _, elem := range t.Elements {
			name := []byte(elem.Name)
			b = zcode.AppendUvarint(b, uint64(len(name)))
			b = append(b, name...)
			//XXX conform with format in issue #XXX.
			//b = zcode.AppendAs(b, container, elem.Value)
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

func FormatTypeValue(tv zcode.Bytes) string {
	var b strings.Builder
	formatTypeValue(tv, &b)
	return b.String()
}

func truncErr(b *strings.Builder) {
	b.WriteString("<ERR truncated type value>")
}

func formatTypeValue(tv zcode.Bytes, b *strings.Builder) zcode.Bytes {
	if len(tv) == 0 {
		truncErr(b)
		return nil
	}
	id := tv[0]
	tv = tv[1:]
	switch id {
	case IDTypeDef:
		name, tv := decodeNameWithCheck(tv, b)
		if tv == nil {
			return nil
		}
		b.WriteString(name)
		b.WriteString("=(")
		tv = formatTypeValue(tv, b)
		b.WriteByte(')')
		return tv
	case IDTypeName:
		name, tv := decodeNameWithCheck(tv, b)
		if tv == nil {
			return nil
		}
		b.WriteString(name)
		return tv
	case IDRecord:
		b.WriteByte('{')
		n, tv := decodeInt(tv, b)
		if tv == nil {
			truncErr(b)
			return nil
		}
		for k := 0; k < n; k++ {
			if k > 0 {
				b.WriteByte(',')
			}
			var name string
			name, tv = decodeNameWithCheck(tv, b)
			b.WriteString(name)
			b.WriteString(":")
			tv = formatTypeValue(tv, b)
			if tv == nil {
				return nil
			}
		}
		b.WriteByte('}')
	case IDArray:
		b.WriteByte('[')
		tv = formatTypeValue(tv, b)
		b.WriteByte(']')
	case IDSet:
		b.WriteString("|[")
		tv = formatTypeValue(tv, b)
		b.WriteString("]|")
	case IDMap:
		b.WriteString("|{")
		tv = formatTypeValue(tv, b)
		b.WriteByte(',')
		tv = formatTypeValue(tv, b)
		b.WriteString("}|")
	case IDUnion:
		b.WriteByte('(')
		n, tv := decodeInt(tv, b)
		if tv == nil {
			truncErr(b)
			return nil
		}
		for k := 0; k < n; k++ {
			if k > 0 {
				b.WriteByte(',')
			}
			tv = formatTypeValue(tv, b)
		}
		b.WriteByte(')')
	case IDEnum:
		//XXX this is not quite compatible with the current enum format,
		// but we're going to simplify it so, here...
		b.WriteByte('<')
		n, tv := decodeInt(tv, b)
		if tv == nil {
			truncErr(b)
			return nil
		}
		for k := 0; k < n; k++ {
			if k > 0 {
				b.WriteByte(',')
			}
			var symbol string
			symbol, tv = decodeNameWithCheck(tv, b)
			if tv == nil {
				return nil
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

func decodeNameAndCheck(tv zcode.Bytes, b *strings.Builder) (string, zcode.Bytes) {
	name, tv = decodeName(tv)
	if tv == nil {
		truncErr(b)
	}
	return name, tv
}

func decodeName(tv zcode.Bytes) (string, zcode.Bytes) {
	namelen, tv := decodeInt(tv, b)
	if tv == nil {
		return "", nil
	}
	if int(namelen) > len(tv) {
		return "", nil
	}
	return string(tv[:namelen]), tv[namelen:]
}

func decodeInt(tv zcode.Bytes) (int, zcode.Bytes) {
	if len(tv) < 0 {
		return 0, nil
	}
	namelen, n := binary.Uvarint(tv)
	if n <= 0 {
		return 0, nil
	}
	return int(namelen), tv[n:]
}
