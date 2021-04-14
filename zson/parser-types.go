package zson

import (
	"unicode"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/zng"
)

func (p *Parser) parseType() (zed.Type, error) {
	typ, err := p.matchType()
	if typ == nil && err == nil {
		err = p.error("couldn't parse type")
	}
	return typ, err
}

func (p *Parser) matchType() (zed.Type, error) {
	if typ, err := p.matchTypeName(); typ != nil || err != nil {
		return typ, err
	}
	if typ, err := p.matchTypeRecord(); typ != nil || err != nil {
		return typ, err
	}
	if typ, err := p.matchTypeArray(); typ != nil || err != nil {
		return typ, err
	}
	if typ, err := p.matchTypeSetOrMap(); typ != nil || err != nil {
		return typ, err
	}
	if typ, err := p.matchTypeUnion(); typ != nil || err != nil {
		return typ, err
	}
	if typ, err := p.matchTypeEnum(); typ != nil || err != nil {
		return typ, err
	}
	// no match
	return nil, nil
}

func (p *Parser) matchIdentifier() (string, error) {
	l := p.lexer
	if err := l.skipSpace(); err != nil {
		return "", err
	}
	r, _, err := l.peekRune()
	if err != nil || !zng.IDChar(r) {
		return "", err
	}
	return l.scanIdentifier()
}

func (p *Parser) matchTypeName() (zed.Type, error) {
	l := p.lexer
	if err := l.skipSpace(); err != nil {
		return nil, err
	}
	r, _, err := l.peekRune()
	if err != nil {
		return nil, err
	}
	if !(zng.IDChar(r) || unicode.IsDigit(r)) {
		return nil, nil
	}
	name, err := l.scanTypeName()
	if err != nil {
		return nil, err
	}
	if t := zng.LookupPrimitive(name); t != nil {
		return &zed.TypePrimitive{"TypePrimitive", name}, nil
	}
	// Wherever we have a type name, we can have a type def defining the
	// type name.
	if ok, err := l.match('='); !ok || err != nil {
		return &zed.TypeName{"TypeName", name}, nil
	}
	tv, err := p.matchTypeValue()
	if err != nil {
		return nil, err
	}
	if tv == nil {
		return nil, p.errorf("bad type sytax in typedef '%s=...'", name)
	}
	return &zed.TypeDef{
		Kind: "TypeDef",
		Name: name,
		Type: tv.Value,
	}, nil
}

func (p *Parser) matchTypeRecord() (*zed.TypeRecord, error) {
	l := p.lexer
	if ok, err := l.match('{'); !ok || err != nil {
		return nil, err
	}
	var fields []zed.TypeField
	for {
		field, err := p.matchTypeField()
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
	ok, err := l.match('}')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched braces while parsing record type")
	}
	return &zed.TypeRecord{
		Kind:   "TypeRecord",
		Fields: fields,
	}, nil
}

func (p *Parser) matchTypeField() (*zed.TypeField, error) {
	l := p.lexer
	symbol, ok, err := p.matchSymbol()
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
		return nil, p.errorf("no type name found for field %q", symbol)
	}
	typ, err := p.parseType()
	if err != nil {
		return nil, err
	}
	return &zed.TypeField{
		Name: symbol,
		Type: typ,
	}, nil
}

func (p *Parser) matchTypeArray() (*zed.TypeArray, error) {
	l := p.lexer
	if ok, err := l.match('['); !ok || err != nil {
		return nil, err
	}
	typ, err := p.parseType()
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
	return &zed.TypeArray{
		Kind: "TypeArray",
		Type: typ,
	}, nil
}

func (p *Parser) matchTypeSetOrMap() (zed.Type, error) {
	l := p.lexer
	if ok, err := l.match('|'); !ok || err != nil {
		return nil, err
	}
	isSet, err := l.matchTight('[')
	if err != nil {
		return nil, err
	}
	var typ zed.Type
	var which string
	if isSet {
		which = "set"
		inner, err := p.parseType()
		if err != nil {
			return nil, err
		}
		ok, err := l.match(']')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("mismatched set-brackets while parsing set type")
		}
		typ = &zed.TypeSet{
			Kind: "TypeSet",
			Type: inner,
		}
	} else {
		ok, err := l.matchTight('{')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("no '|[' or '|{' type token at '|' character")
		}
		which = "map"
		typ, err = p.parseTypeMap()
		if err != nil {
			return nil, err
		}
		ok, err = l.match('}')
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, p.error("mismatched set-brackets while parsing map type")
		}
	}
	ok, err := l.matchTight('|')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.errorf("mismatched closing bracket while parsing type %q", which)
	}
	return typ, nil

}

func (p *Parser) parseTypeMap() (*zed.TypeMap, error) {
	keyType, err := p.parseType()
	if err != nil {
		return nil, err
	}
	ok, err := p.lexer.match(',')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("value type missing while parsing map type")
	}
	valType, err := p.parseType()
	if err != nil {
		return nil, err
	}
	return &zed.TypeMap{
		Kind:    "TypeMap",
		KeyType: keyType,
		ValType: valType,
	}, nil
}

func (p *Parser) matchTypeUnion() (*zed.TypeUnion, error) {
	l := p.lexer
	if ok, err := l.match('('); !ok || err != nil {
		return nil, err
	}
	var types []zed.Type
	for {
		typ, err := p.matchType()
		if err != nil {
			return nil, err
		}
		if typ == nil {
			break
		}
		types = append(types, typ)
		ok, err := l.match(',')
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
	}
	if len(types) < 2 {
		return nil, p.error("type list not found parsing union type at '('")
	}
	ok, err := l.match(')')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched parentheses while parsing union type")
	}
	return &zed.TypeUnion{
		Kind:  "TypeUnion",
		Types: types,
	}, nil
}

func (p *Parser) matchTypeEnum() (*zed.TypeEnum, error) {
	l := p.lexer
	if ok, err := l.match('<'); !ok || err != nil {
		return nil, err
	}
	fields, err := p.matchEnumFields()
	if err != nil {
		return nil, err
	}
	ok, err := l.match('>')
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.error("mismatched brackets while parsing enum type")
	}
	return &zed.TypeEnum{
		Kind:     "TypeEnum",
		Elements: fields,
	}, nil
}

func (p *Parser) matchEnumFields() ([]zed.Field, error) {
	l := p.lexer
	var fields []zed.Field
	for {
		field, err := p.matchEnumField()
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

func (p *Parser) matchEnumField() (*zed.Field, error) {
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
	var val zed.Value
	if ok {
		v, err := p.ParseValue()
		if err != nil {
			return nil, err
		}
		val = v
	}
	return &zed.Field{
		Name:  name,
		Value: val,
	}, nil
}
