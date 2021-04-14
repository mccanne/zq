package zson

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zng"
)

func (p *Parser) ParseValue() (zed.Value, error) {
	v, err := p.matchValue()
	if err == io.EOF {
		err = nil
	}
	if v == nil && err == nil {
		if err := p.lexer.check(1); (err != nil && err != io.EOF) || len(p.lexer.cursor) > 0 {
			return nil, errors.New("zson syntax error")
		}
	}
	return v, err
}

func noEOF(err error) error {
	if err == io.EOF {
		err = nil
	}
	return err
}

func (p *Parser) matchValue() (zed.Value, error) {
	if val, err := p.matchRecord(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	if val, err := p.matchArray(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	if val, err := p.matchSetOrMap(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	if val, err := p.matchTypeValue(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	// Primitive comes last as the other matchers short-circuit more
	// efficiently on sentinel characters.
	if val, err := p.matchPrimitive(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	// But enum really goes last becase we don't want it to pick up
	// true, false, or null.
	if val, err := p.matchEnum(); val != nil || err != nil {
		return p.decorate(val, err)
	}
	return nil, nil
}

func anyAsValue(any zed.Any) *zed.ImpliedValue {
	return &zed.ImpliedValue{
		Kind: "ImpliedValue",
		Of:   any,
	}
}

func (p *Parser) decorate(any zed.Any, err error) (zed.Value, error) {
	if err != nil {
		return nil, err
	}
	// First see if there's a short-form typedef decorator.
	// If there isn't, matchShortForm() returns the AnyValue wrapped
	// in an zed.ImpliedValue (as zed.Value)  Otherwise, it returns an
	//  zed.DefVal (as zed.Vaue).
	val, ok, err := p.matchDecorator(any, nil)
	if err != nil {
		return nil, err
	}
	if !ok {
		// No decorator.  Just return the input value.
		return anyAsValue(any), nil
	}
	// Now see if there are additional decorators to apply as casts and
	// return value chain, wrapped if at all, as an zed.Value.
	for {
		outer, ok, err := p.matchDecorator(nil, val)
		if err != nil {
			return nil, err
		}
		if !ok {
			return val, nil
		}
		val = outer
	}
}

// We pass both any and val in here to avoid having to backtrack.
// If we had proper backtracking, this would look a little more sensible.
func (p *Parser) matchDecorator(any zed.Any, val zed.Value) (zed.Value, bool, error) {
	l := p.lexer
	ok, err := l.match('(')
	if err != nil || !ok {
		return nil, false, noEOF(err)
	}
	val, err = p.parseDecorator(any, val)
	if err != nil {
		return nil, false, err
	}
	ok, err = l.match(')')
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, p.error("mismatched parentheses while parsing type decorator")
	}
	return val, true, nil
}

func (p *Parser) parseDecorator(any zed.Any, val zed.Value) (zed.Value, error) {
	l := p.lexer
	// We can have either:
	//   Case 1: =<name>
	//   Case 2: <type>
	// For case 2, there can be embedded typedefs created from the
	// descent into parseType.
	ok, err := l.match('=')
	if err != nil {
		return nil, err
	}
	if ok {
		name, err := l.scanTypeName()
		if name == "" || err != nil {
			return nil, p.error("bad short-form type definition")
		}
		return &zed.DefValue{
			Kind:     "DefValue",
			Of:       any,
			TypeName: name,
		}, nil
	}
	typ, err := p.parseType()
	if err != nil {
		return nil, err
	}
	if any != nil {
		return &zed.CastValue{
			Kind: "CastValue",
			Of:   anyAsValue(any),
			Type: typ,
		}, nil
	}
	return &zed.CastValue{
		Kind: "CastValue",
		Of:   val,
		Type: typ,
	}, nil
}

// A bug in Go's time.ParseDuration() function causes an error for
// duration value math.MinInt64.  We work around this by explicitly
// checking for the string that represents this duration (which is
// correctly returned by time.Duration.String()).
const minDuration = "-2562047h47m16.854775808s"

func (p *Parser) matchPrimitive() (*zed.Primitive, error) {
	if val, err := p.matchStringPrimitive(); val != nil || err != nil {
		return val, noEOF(err)
	}
	if val, err := p.matchBacktickString(); val != nil || err != nil {
		return val, noEOF(err)
	}
	l := p.lexer
	if err := l.skipSpace(); err != nil {
		return nil, noEOF(err)
	}
	s, err := l.peekPrimitive()
	if err != nil {
		return nil, noEOF(err)
	}
	if s == "" {
		return nil, nil
	}
	// Try to parse the string different ways.  This is not intended
	// to be performant.  ZNG provides performance for the ZSON data model.
	var typ string
	if s == "true" || s == "false" {
		typ = "bool"
	} else if s == "null" {
		typ = "null"
	} else if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		typ = "int64"
	} else if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		typ = "uint64"
	} else if _, err := strconv.ParseFloat(s, 64); err == nil {
		typ = "float64"
	} else if _, err := time.Parse(time.RFC3339Nano, s); err == nil {
		typ = "time"
	} else if _, err := nano.ParseDuration(s); err == nil {
		typ = "duration"
	} else if _, _, err := net.ParseCIDR(s); err == nil {
		typ = "net"
	} else if ip := net.ParseIP(s); ip != nil {
		typ = "ip"
	} else if len(s) >= 2 && s[0:2] == "0x" {
		if len(s) == 2 {
			typ = "bytes"
		} else if _, err := hex.DecodeString(s[2:]); err == nil {
			typ = "bytes"
		} else {
			return nil, err
		}
	} else {
		// no match
		return nil, nil
	}
	l.skip(len(s))
	return &zed.Primitive{
		Kind: "Primitive",
		Type: typ,
		Text: s,
	}, nil
}

func (p *Parser) matchStringPrimitive() (*zed.Primitive, error) {
	s, ok, err := p.matchString()
	if err != nil || !ok {
		return nil, noEOF(err)
	}
	return &zed.Primitive{
		Kind: "Primitive",
		Type: "string",
		Text: s,
	}, nil
}

func (p *Parser) matchString() (string, bool, error) {
	l := p.lexer
	ok, err := l.match('"')
	if err != nil || !ok {
		return "", false, noEOF(err)
	}
	s, err := l.scanString()
	if err != nil {
		return "", false, p.error("parsing string literal")
	}
	ok, err = l.match('"')
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, p.error("mismatched string quotes")
	}
	return s, true, nil
}

var arrow = []byte("=>")

func (p *Parser) matchBacktickString() (*zed.Primitive, error) {
	l := p.lexer
	keepIndentation := false
	ok, err := l.matchBytes(arrow)
	if err != nil {
		return nil, noEOF(err)
	}
	if ok {
		keepIndentation = true
	}
	ok, err = l.match('`')
	if err != nil || !ok {
		if err == nil && keepIndentation {
			err = errors.New("no backtick found following '=>'")
		}
		return nil, err
	}
	s, err := l.scanBacktickString(keepIndentation)
	if err != nil {
		return nil, p.error("parsing backtick string literal")
	}
	ok, err = l.match('`')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched string backticks")
	}
	return &zed.Primitive{
		Kind: "Primitive",
		Type: "string",
		Text: s,
	}, nil
}

func (p *Parser) matchRecord() (*zed.Record, error) {
	l := p.lexer
	if ok, err := l.match('{'); !ok || err != nil {
		return nil, noEOF(err)
	}
	fields, err := p.matchFields()
	if err != nil {
		return nil, err
	}
	ok, err := l.match('}')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched braces while parsing record type")
	}
	return &zed.Record{
		Kind:   "Record",
		Fields: fields,
	}, nil
}

func (p *Parser) matchFields() ([]zed.Field, error) {
	l := p.lexer
	var fields []zed.Field
	for {
		field, err := p.matchField()
		if err != nil {
			return nil, err
		}
		if field == nil {
			break
		}
		fields = append(fields, *field)
		ok, err := l.match(',')
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
	}
	return fields, nil
}

func (p *Parser) matchField() (*zed.Field, error) {
	l := p.lexer
	name, ok, err := p.matchSymbol()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	ok, err = l.match(':')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.errorf("no type name found for field %q", name)
	}
	val, err := p.ParseValue()
	if err != nil {
		return nil, err
	}
	return &zed.Field{
		Name:  name,
		Value: val,
	}, nil
}

func (p *Parser) matchSymbol() (string, bool, error) {
	s, ok, err := p.matchString()
	if err != nil {
		return "", false, noEOF(err)
	}
	if ok {
		return s, true, nil
	}
	s, err = p.matchIdentifier()
	if err != nil || s == "" {
		return "", false, err
	}
	return s, true, nil
}

func (p *Parser) matchArray() (*zed.Array, error) {
	l := p.lexer
	if ok, err := l.match('['); !ok || err != nil {
		return nil, noEOF(err)
	}
	vals, err := p.matchValueList()
	if err != nil {
		return nil, err
	}
	ok, err := l.match(']')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched brackets while parsing array type")
	}
	return &zed.Array{
		Kind:     "Array",
		Elements: vals,
	}, nil
}

func (p *Parser) matchValueList() ([]zed.Value, error) {
	l := p.lexer
	var vals []zed.Value
	for {
		val, err := p.matchValue()
		if err != nil {
			return nil, err
		}
		if val == nil {
			break
		}
		vals = append(vals, val)
		ok, err := l.match(',')
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
	}
	return vals, nil
}

func (p *Parser) matchSetOrMap() (zed.Any, error) {
	l := p.lexer
	if ok, err := l.match('|'); !ok || err != nil {
		return nil, noEOF(err)
	}
	isSet, err := l.matchTight('[')
	if err != nil {
		return nil, err
	}
	var val zed.Any
	var which string
	if isSet {
		which = "set"
		vals, err := p.matchValueList()
		if err != nil {
			return nil, err
		}
		ok, err := l.match(']')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("mismatched set value brackets")
		}
		val = &zed.Set{
			Kind:     "Set",
			Elements: vals,
		}
	} else {
		ok, err := l.matchTight('{')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("no '|[' or '|{' type bracket at '|' character")
		}
		which = "map"
		entries, err := p.matchMapEntries()
		if err != nil {
			return nil, err
		}
		ok, err = l.match('}')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("mismatched map value brackets")
		}
		val = &zed.Map{
			Kind:    "Map",
			Entries: entries,
		}
	}
	ok, err := l.matchTight('|')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.errorf("mismatched closing bracket while parsing %s value", which)
	}
	return val, nil

}

func (p *Parser) matchMapEntries() ([]zed.Entry, error) {
	l := p.lexer
	var entries []zed.Entry
	for {
		ok, err := l.match('{')
		if err != nil {
			return nil, err
		}
		if !ok {
			return entries, nil
		}
		entry, err := p.parseEntry()
		if err != nil {
			return nil, err
		}
		ok, err = l.match('}')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("mismatched braces while parsing map value entries")
		}
		entries = append(entries, *entry)
		ok, err = p.lexer.match(',')
		if err != nil {
			return nil, err
		}
		if !ok {
			return entries, nil
		}
	}
}

func (p *Parser) parseEntry() (*zed.Entry, error) {
	key, err := p.matchValue()
	if err != nil {
		return nil, err
	}
	if key == nil {
		// no match
		return nil, errors.New("map key not found after '{' while parsing entries")
	}
	ok, err := p.lexer.match(',')
	if err != nil {

		return nil, err
	}
	if !ok {
		return nil, p.error("no comma found after key vaoue while parsing map entry")
	}
	val, err := p.ParseValue()
	if err != nil {
		return nil, err
	}
	return &zed.Entry{
		Key:   key,
		Value: val,
	}, nil
}

func (p *Parser) matchEnum() (*zed.Enum, error) {
	// We only detect identifier-style enum values even though they can
	// also be strings but we don't know that until the semantic check.
	name, err := p.matchIdentifier()
	if err != nil || name == "" {
		return nil, noEOF(err)
	}
	return &zed.Enum{
		Kind: "Enum",
		Name: name,
	}, nil
}

func (p *Parser) matchTypeValue() (*zed.TypeValue, error) {
	l := p.lexer
	if ok, err := l.match('('); !ok || err != nil {
		return nil, noEOF(err)
	}
	typ, err := p.parseType()
	if err != nil {
		return nil, err
	}
	ok, err := l.match(')')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched parentheses while parsing type value")
	}
	return &zed.TypeValue{
		Kind:  "TypeValue",
		Value: typ,
	}, nil
}

func ParsePrimitive(typeText, valText string) (zng.Value, error) {
	typ := zng.LookupPrimitive(typeText)
	if typ == nil {
		return zng.Value{}, fmt.Errorf("no such type: %s", typeText)
	}
	var b Builder
	if err := b.BuildPrimitive(&Primitive{Type: typ, Text: valText}); err != nil {
		return zng.Value{}, err
	}
	it := b.Bytes().Iter()
	bytes, _, _ := it.Next()
	return zng.Value{typ, bytes}, nil
}
