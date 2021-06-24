package zed

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

func NewTypeValue(t Type) Value {
	return Value{TypeType, EncodeTypeValue(t)}
}

func EncodeTypeValue(t Type) zcode.Bytes {
	var typedefs map[string]Type
	return appendTypeValue(nil, t, &typedefs)
}

func appendTypeValue(b zcode.Bytes, t Type, typedefs *map[string]Type) zcode.Bytes {
	switch t := t.(type) {
	case *TypeAlias:
		if *typedefs == nil {
			*typedefs = make(map[string]Type)
		}
		id := byte(IDTypeDef)
		if previous := (*typedefs)[t.Name]; previous == t.Type {
			id = IDTypeName
		} else {
			(*typedefs)[t.Name] = t.Type
		}
		b = append(b, id)
		b = zcode.AppendUvarint(b, uint64(len(t.Name)))
		b = append(b, zcode.Bytes(t.Name)...)
		if id == IDTypeName {
			return b
		}
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeRecord:
		b = append(b, IDTypeRecord)
		b = zcode.AppendUvarint(b, uint64(len(t.Columns)))
		for _, col := range t.Columns {
			b = zcode.AppendUvarint(b, uint64(len(col.Name)))
			b = append(b, col.Name...)
			b = appendTypeValue(b, col.Type, typedefs)
		}
		return b
	case *TypeUnion:
		b = append(b, IDTypeUnion)
		b = zcode.AppendUvarint(b, uint64(len(t.Types)))
		for _, t := range t.Types {
			b = appendTypeValue(b, t, typedefs)
		}
		return b
	case *TypeSet:
		b = append(b, IDTypeSet)
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeArray:
		b = append(b, IDTypeArray)
		return appendTypeValue(b, t.Type, typedefs)
	case *TypeEnum:
		b = append(b, IDTypeEnum)
		b = zcode.AppendUvarint(b, uint64(len(t.Symbols)))
		for _, s := range t.Symbols {
			b = zcode.AppendUvarint(b, uint64(len(s)))
			b = append(b, s...)
		}
		return b
	case *TypeMap:
		b = append(b, IDTypeMap)
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
		name, tv := decodeNameAndCheck(tv, b)
		if tv == nil {
			return nil
		}
		b.WriteString(name)
		b.WriteString("=(")
		tv = formatTypeValue(tv, b)
		b.WriteByte(')')
		return tv
	case IDTypeName:
		name, tv := decodeNameAndCheck(tv, b)
		if tv == nil {
			return nil
		}
		b.WriteString(name)
		return tv
	case IDTypeRecord:
		b.WriteByte('{')
		var n int
		n, tv = decodeInt(tv)
		if tv == nil {
			truncErr(b)
			return nil
		}
		for k := 0; k < n; k++ {
			if k > 0 {
				b.WriteByte(',')
			}
			var name string
			name, tv = decodeNameAndCheck(tv, b)
			b.WriteString(name)
			b.WriteString(":")
			tv = formatTypeValue(tv, b)
			if tv == nil {
				return nil
			}
		}
		b.WriteByte('}')
	case IDTypeArray:
		b.WriteByte('[')
		tv = formatTypeValue(tv, b)
		b.WriteByte(']')
	case IDTypeSet:
		b.WriteString("|[")
		tv = formatTypeValue(tv, b)
		b.WriteString("]|")
	case IDTypeMap:
		b.WriteString("|{")
		tv = formatTypeValue(tv, b)
		b.WriteByte(',')
		tv = formatTypeValue(tv, b)
		b.WriteString("}|")
	case IDTypeUnion:
		b.WriteByte('(')
		var n int
		n, tv = decodeInt(tv)
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
	case IDTypeEnum:
		b.WriteByte('<')
		var n int
		n, tv = decodeInt(tv)
		if tv == nil {
			truncErr(b)
			return nil
		}
		for k := 0; k < n; k++ {
			if k > 0 {
				b.WriteByte(',')
			}
			var symbol string
			symbol, tv = decodeNameAndCheck(tv, b)
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
	var name string
	name, tv = decodeName(tv)
	if tv == nil {
		truncErr(b)
	}
	return name, tv
}

func decodeName(tv zcode.Bytes) (string, zcode.Bytes) {
	namelen, tv := decodeInt(tv)
	if tv == nil || int(namelen) > len(tv) {
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
