package zfmt

import (
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/zng"
)

type canonZed struct {
	formatter
}

//XXX this needs to change when we use the zson values from the ast
func (c *canonZed) literal(e zed.Primitive) {
	switch e.Type {
	case "string", "bstring", "error":
		c.write("\"%s\"", e.Text)
	case "regexp":
		c.write("/%s/", e.Text)
	default:
		//XXX need decorators for non-implied
		c.write("%s", e.Text)

	}
}

func (c *canonZed) fieldpath(path []string) {
	if len(path) == 0 {
		c.write(".")
		return
	}
	for k, s := range path {
		if zng.IsIdentifier(s) {
			if k != 0 {
				c.write(".")
			}
			c.write(s)
		} else {
			if k == 0 {
				c.write(".")
			}
			c.write("[%q]", s)
		}
	}
}

func (c *canonZed) typ(t zed.Type) {
	switch t := t.(type) {
	case *zed.TypePrimitive:
		c.write(t.Name)
	case *zed.TypeRecord:
		c.write("{")
		c.typeFields(t.Fields)
		c.write("}")
	case *zed.TypeArray:
		c.write("[")
		c.typ(t.Type)
		c.write("]")
	case *zed.TypeSet:
		c.write("|[")
		c.typ(t.Type)
		c.write("]|")
	case *zed.TypeUnion:
		c.write("(")
		c.types(t.Types)
		c.write(")")
	case *zed.TypeEnum:
		//XXX need to figure out Z syntax for enum literal which may
		// be different than zson, requiring some ast adjustments.
		c.write("TBD:ENUM")
	case *zed.TypeMap:
		c.write("|{")
		c.typ(t.KeyType)
		c.write(",")
		c.typ(t.ValType)
		c.write("}|")
	case *zed.TypeNull:
		c.write("null")
	case *zed.TypeDef:
		c.write("%s=(", t.Name)
		c.typ(t.Type)
		c.write(")")
	case *zed.TypeName:
		c.write(t.Name)
	}
}

func (c *canonZed) typeFields(fields []zed.TypeField) {
	for k, f := range fields {
		if k != 0 {
			c.write(",")
		}
		c.write("%s:", zng.QuotedName(f.Name))
		c.typ(f.Type)
	}
}

func (c *canonZed) types(types []zed.Type) {
	for k, t := range types {
		if k != 0 {
			c.write(",")
		}
		c.typ(t)
	}
}
