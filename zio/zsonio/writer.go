package zsonio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zson"
)

type Writer struct {
	writer      io.WriteCloser
	zctx        *resolver.Context
	mapper      *resolver.Mapper
	typedefs    typemap
	tab         int
	newline     string
	whitespace  string
	typeTab     int
	typeNewline string
	nid         int
	types       *zson.TypeTable
}

type WriterOpts struct {
	Pretty int
}

func NewWriter(w io.WriteCloser, opts WriterOpts) *Writer {
	newline := ""
	if opts.Pretty > 0 {
		newline = "\n"
	}
	zctx := resolver.NewContext()
	return &Writer{
		zctx:       zctx,
		writer:     w,
		typedefs:   make(typemap),
		tab:        opts.Pretty,
		newline:    newline,
		whitespace: strings.Repeat(" ", 80),
		mapper:     resolver.NewMapper(zctx),
	}
}

func (w *Writer) Close() error {
	return w.writer.Close()
}

func (w *Writer) Write(rec *zng.Record) error {
	typ, err := w.mapper.Translate(rec.Type)
	if err != nil {
		return err
	}
	if err := w.writeValueAndDecorate(typ, rec.Raw); err != nil {
		return err
	}
	return w.write("\n")
}

func (w *Writer) writeValueAndDecorate(typ zng.Type, bytes zcode.Bytes) error {
	known := w.typedefs.exists(typ)
	if err := w.writeValue(0, typ, bytes, known, false); err != nil {
		return err
	}
	return w.decorate(typ, false)
}

func (w *Writer) writeValue(indent int, typ zng.Type, bytes zcode.Bytes, parentKnown, decorate bool) error {
	known := parentKnown || w.typedefs.exists(typ)
	if bytes == nil {
		if err := w.write("null"); err != nil {
			return err
		}
		return w.decorate(typ, parentKnown)
	}
	var err error
	switch t := typ.(type) {
	default:
		err = w.write(typ.ZSONOf(bytes))
	case *zng.TypeAlias:
		if err := w.writeValue(indent, t.Type, bytes, known, false); err != nil {
			return err
		}
	case *zng.TypeRecord:
		err = w.writeRecord(indent, t, bytes, known)
	case *zng.TypeArray:
		err = w.writeVector(indent, "[", "]", t.Type, zng.Value{t, bytes}, known)
	case *zng.TypeSet:
		err = w.writeVector(indent, "|[", "]|", t.Type, zng.Value{t, bytes}, known)
	case *zng.TypeUnion:
		err = w.writeUnion(indent, t, bytes)
	case *zng.TypeMap:
		err = w.writeMap(indent, t, bytes, known)
	case *zng.TypeEnum:
		err = w.write(t.ZSONOf(bytes))
	case *zng.TypeOfType:
		err = w.writef("(%s)", string(bytes))
	}
	if err == nil && decorate {
		err = w.decorate(typ, parentKnown)
	}
	return err
}

func (w *Writer) nextInternalType() string {
	name := strconv.Itoa(w.nid)
	w.nid++
	return name
}

func (w *Writer) decorate(typ zng.Type, known bool) error {
	if known || zson.Implied(typ) {
		return nil
	}
	if name, ok := w.typedefs[typ]; ok {
		return w.writef(" (%s)", name)
	}
	if zson.SelfDescribing(typ) {
		var name string
		if typ, ok := typ.(*zng.TypeAlias); ok {
			name = typ.Name
		} else {
			name = w.nextInternalType()
		}
		w.typedefs[typ] = name
		return w.writef(" (=%s)", name)
	}
	return w.writef(" (%s)", w.lookupType(typ))
}

func (w *Writer) writeRecord(indent int, typ *zng.TypeRecord, bytes zcode.Bytes, known bool) error {
	if err := w.write("{"); err != nil {
		return err
	}
	if len(typ.Columns) == 0 {
		return w.write("}")
	}
	indent += w.tab
	sep := w.newline
	it := bytes.Iter()
	for _, field := range typ.Columns {
		if it.Done() {
			return &zng.RecordTypeError{Name: string(field.Name), Type: field.Type.String(), Err: zng.ErrMissingField}
		}
		bytes, _, err := it.Next()
		if err != nil {
			return err
		}
		if err := w.write(sep); err != nil {
			return err
		}
		if err := w.indent(indent, zng.QuotedName(field.Name)); err != nil {
			return err
		}
		if err := w.write(": "); err != nil {
			return err
		}
		if err := w.writeValue(indent, field.Type, bytes, known, true); err != nil {
			return err
		}
		sep = "," + w.newline
	}
	if err := w.write(w.newline); err != nil {
		return err
	}
	return w.indent(indent-w.tab, "}")
}

func (w *Writer) writeVector(indent int, open, close string, inner zng.Type, zv zng.Value, known bool) error {
	if err := w.write(open); err != nil {
		return err
	}
	len, err := zv.ContainerLength()
	if err != nil {
		return err
	}
	if len == 0 {
		return w.write(close)
	}
	indent += w.tab
	sep := w.newline
	it := zv.Iter()
	for !it.Done() {
		bytes, _, err := it.Next()
		if err != nil {
			return err
		}
		if err := w.write(sep); err != nil {
			return err
		}
		if err := w.indent(indent, ""); err != nil {
			return err
		}
		if err := w.writeValue(indent, inner, bytes, known, true); err != nil {
			return err
		}
		sep = "," + w.newline
	}
	if err := w.write(w.newline); err != nil {
		return err
	}
	return w.indent(indent-w.tab, close)
}

func (w *Writer) writeUnion(indent int, union *zng.TypeUnion, bytes zcode.Bytes) error {
	typ, _, bytes, err := union.SplitZng(bytes)
	if err != nil {
		return err
	}
	// XXX For now, we always decorate a union value so that
	// we can determine the selector from the value's explicit type.
	// We can later optimize this so we only print the decorator if its
	// ambigous with another type (e.g., int8 and int16 vs a union of int8 and string).
	// Let's do this after we have the parser working and capable of this
	// disambiguation.  See issue #1764.
	// In other words, just because we known the union's type doesn't mean
	// we know the type of a particular value of that union.
	known := false
	return w.writeValue(indent, typ, bytes, known, true)
}

func (w *Writer) writeMap(indent int, typ *zng.TypeMap, bytes zcode.Bytes, known bool) error {
	if err := w.write("|{"); err != nil {
		return err
	}
	if bytes == nil {
		return w.write("|}")
	}
	indent += w.tab
	sep := w.newline
	for it := bytes.Iter(); !it.Done(); {
		keyBytes, _, err := it.Next()
		if err != nil {
			return err
		}
		if it.Done() {
			return errors.New("truncated map value")
		}
		valBytes, _, err := it.Next()
		if err != nil {
			return err
		}
		if err := w.write(sep); err != nil {
			return err
		}
		if err := w.indent(indent, "{"); err != nil {
			return err
		}
		if err := w.writeValue(indent+w.tab, typ.KeyType, keyBytes, known, true); err != nil {
			return err
		}
		if err := w.write(","); err != nil {
			return err
		}
		if err := w.writeValue(indent+w.tab, typ.ValType, valBytes, known, true); err != nil {
			return err
		}
		if err := w.write("}"); err != nil {
			return err
		}
		sep = "," + w.newline
	}
	if err := w.write(w.newline); err != nil {
		return err
	}
	return w.indent(indent-w.tab, "}|")
}

func (w *Writer) indent(tab int, s string) error {
	n := len(w.whitespace)
	if n < tab {
		n = 2 * tab
		w.whitespace = strings.Repeat(" ", n)
	}
	if err := w.write(w.whitespace[0:tab]); err != nil {
		return err
	}
	return w.write(s)
}

func (w *Writer) write(s string) error {
	_, err := w.writer.Write([]byte(s))
	return err
}

func (w *Writer) writef(s string, args ...interface{}) error {
	_, err := fmt.Fprintf(w.writer, s, args...)
	return err
}

// lookupType returns the type string for the given type embedding any
// needed typedefs for aliases that have not been previously defined.
// These typedefs use the embedded syntax (name=(type-string)).
// Typedefs handled by decorators are handled in decorate().
func (w *Writer) lookupType(typ zng.Type) string {
	if name, ok := w.typedefs[typ]; ok {
		return name
	}
	if alias, ok := typ.(*zng.TypeAlias); ok {
		// We don't check here for typedefs that illegally change
		// the type of a type name as we build this output from
		// the internal type system which should not let this happen.
		name := alias.Name
		w.typedefs[typ] = name
		body := w.lookupType(alias.Type)
		return fmt.Sprintf("%s=(%s)", name, body)
	}
	if typ.ID() < zng.IdTypeDef {
		name := typ.String()
		w.typedefs[typ] = name
		return name
	}
	name := w.nextInternalType()
	body := w.formatType(typ)
	w.typedefs[typ] = name
	return fmt.Sprintf("%s=(%s)", name, body)
}

func (w *Writer) formatType(typ zng.Type) string {
	if name, ok := w.typedefs[typ]; ok {
		return name
	}
	switch typ := typ.(type) {
	case *zng.TypeAlias:
		// Aliases are handled differently above to determine the
		// plain form vs embedded typedef.
		panic("alias shouldn't be formatted")
	case *zng.TypeRecord:
		return w.formatRecord(typ)
	case *zng.TypeArray:
		return fmt.Sprintf("[%s]", w.lookupType(typ.Type))
	case *zng.TypeSet:
		return fmt.Sprintf("|[%s]|", w.lookupType(typ.Type))
	case *zng.TypeMap:
		return fmt.Sprintf("|{%s,%s}|", w.lookupType(typ.KeyType), w.lookupType(typ.ValType))
	case *zng.TypeUnion:
		return w.formatUnion(typ)
	case *zng.TypeEnum:
		return w.formatEnum(typ)
	case *zng.TypeOfType:
		return typ.ZSON()
	}
	panic("unknown case in formatType(): " + typ.String())
}

func (w *Writer) formatRecord(typ *zng.TypeRecord) string {
	var s strings.Builder
	var sep string
	s.WriteString("{")
	for _, col := range typ.Columns {
		s.WriteString(sep)
		s.WriteString(zng.QuotedName(col.Name))
		s.WriteString(":")
		s.WriteString(w.lookupType(col.Type))
		sep = ","
	}
	s.WriteString("}")
	return s.String()
}

func (w *Writer) formatUnion(typ *zng.TypeUnion) string {
	var s strings.Builder
	sep := ""
	s.WriteString("(")
	for _, typ := range typ.Types {
		s.WriteString(sep)
		s.WriteString(w.lookupType(typ))
		sep = ","
	}
	s.WriteString(")")
	return s.String()
}

// XXX This needs a refactor.  The type formatter needs to be be able to format
// a value for enum but the Writer type presume values are "written" not "formatter".
// This hack works with a bytes.Buffer works around this.  Also, when we are writing
// enum values inside of a type, we don't want the pretty printing so the refactor
// will allow you to specify indent on a per-format basis.  See issue #1763.

type nopCloser struct {
	bytes.Buffer
}

func (*nopCloser) Close() error {
	return nil
}

func (w *Writer) formatEnum(typ *zng.TypeEnum) string {
	save := w.writer
	b := &nopCloser{}
	w.writer = b
	sep := ""
	w.write("<")
	inner := typ.Type
	for k, elem := range typ.Elements {
		w.write(sep)
		w.writef("%s:", zng.QuotedName(elem.Name))
		known := k != 0
		if err := w.writeValue(0, inner, elem.Value, known, true); err != nil {
			//XXX See comment above about refactor.
			return err.Error()
		}
		sep = ","
	}
	w.write(">")
	w.writer = save
	return b.String()
}
