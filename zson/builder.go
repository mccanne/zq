package zson

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
)

func Build(b *zcode.Builder, val Value) (zed.Value, error) {
	b.Reset()
	if err := buildValue(b, val); err != nil {
		return zed.Value{}, err
	}
	it := b.Bytes().Iter()
	bytes, _, err := it.Next()
	if err != nil {
		return zed.Value{}, err
	}
	return zed.Value{val.TypeOf(), bytes}, nil
}

func buildValue(b *zcode.Builder, val Value) error {
	switch val := val.(type) {
	case *Primitive:
		return BuildPrimitive(b, *val)
	case *Record:
		return buildRecord(b, val)
	case *Array:
		return buildArray(b, val)
	case *Set:
		return buildSet(b, val)
	case *Union:
		return buildUnion(b, val)
	case *Map:
		return buildMap(b, val)
	case *Enum:
		return buildEnum(b, val)
	case *TypeValue:
		return buildTypeValue(b, val)
	case *Null:
		b.AppendNull()
		return nil
	}
	return fmt.Errorf("unknown ast type: %T", val)
}

func BuildPrimitive(b *zcode.Builder, val Primitive) error {
	switch zed.AliasOf(val.Type).(type) {
	case *zed.TypeOfUint8, *zed.TypeOfUint16, *zed.TypeOfUint32, *zed.TypeOfUint64:
		v, err := strconv.ParseUint(val.Text, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid unsigned integer: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeUint(v))
		return nil
	case *zed.TypeOfInt8, *zed.TypeOfInt16, *zed.TypeOfInt32, *zed.TypeOfInt64:
		v, err := strconv.ParseInt(val.Text, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeInt(v))
		return nil
	case *zed.TypeOfDuration:
		d, err := nano.ParseDuration(val.Text)
		if err != nil {
			return fmt.Errorf("invalid duration: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeDuration(d))
		return nil
	case *zed.TypeOfTime:
		t, err := time.Parse(time.RFC3339Nano, val.Text)
		if err != nil {
			return fmt.Errorf("invalid ISO time: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeTime(nano.TimeToTs(t)))
		return nil
	case *zed.TypeOfFloat64:
		v, err := strconv.ParseFloat(val.Text, 64)
		if err != nil {
			return fmt.Errorf("invalid floating point: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeFloat64(v))
		return nil
	case *zed.TypeOfBool:
		var v bool
		if val.Text == "true" {
			v = true
		} else if val.Text != "false" {
			return fmt.Errorf("invalid bool: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeBool(v))
		return nil
	case *zed.TypeOfBytes:
		s := val.Text
		if len(s) < 2 || s[0:2] != "0x" {
			return fmt.Errorf("invalid bytes: %s", s)
		}
		var bytes []byte
		if len(s) == 2 {
			// '0x' is an empty byte string (not null byte string)
			bytes = make([]byte, 0, 0)
		} else {
			var err error
			bytes, err = hex.DecodeString(s[2:])
			if err != nil {
				return fmt.Errorf("invalid bytes: %s (%w)", s, err)
			}
		}
		b.AppendPrimitive(zcode.Bytes(bytes))
		return nil
	case *zed.TypeOfString, *zed.TypeOfError:
		body := zed.EncodeString(val.Text)
		if !utf8.Valid(body) {
			return fmt.Errorf("invalid utf8 string: %q", val.Text)
		}
		b.AppendPrimitive(body)
		return nil
	case *zed.TypeOfBstring:
		b.AppendPrimitive(unescapeHex([]byte(val.Text)))
		return nil
	case *zed.TypeOfIP:
		ip := net.ParseIP(val.Text)
		if ip == nil {
			return fmt.Errorf("invalid IP: %s", val.Text)
		}
		b.AppendPrimitive(zed.EncodeIP(ip))
		return nil
	case *zed.TypeOfNet:
		_, net, err := net.ParseCIDR(val.Text)
		if err != nil {
			return fmt.Errorf("invalid network: %s (%w)", val.Text, err)
		}
		b.AppendPrimitive(zed.EncodeNet(net))
		return nil
	case *zed.TypeOfNull:
		if val.Text != "" {
			return fmt.Errorf("invalid text body of null value: %q", val.Text)
		}
		b.AppendPrimitive(nil)
		return nil
	case *zed.TypeOfType:
		return fmt.Errorf("type values should not be encoded as primitives: %q", val.Text)
	}
	return fmt.Errorf("unknown primitive: %T", val.Type)
}

func unescapeHex(in []byte) []byte {
	if bytes.IndexByte(in, '\\') < 0 {
		return in
	}
	b := make([]byte, 0, len(in))
	i := 0
	for i < len(in) {
		c := in[i]
		if c == '\\' && len(in[i:]) >= 4 && in[i+1] == 'x' {
			v1 := zed.Unhex(in[i+2])
			v2 := zed.Unhex(in[i+3])
			// This is undefined behavior for non hex \x chars.
			c = v1<<4 | v2
			i += 4
		} else {
			i++
		}
		b = append(b, c)
	}
	return b
}

func buildRecord(b *zcode.Builder, val *Record) error {
	b.BeginContainer()
	for _, v := range val.Fields {
		if err := buildValue(b, v); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func buildArray(b *zcode.Builder, array *Array) error {
	b.BeginContainer()
	for _, v := range array.Elements {
		if err := buildValue(b, v); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func buildSet(b *zcode.Builder, set *Set) error {
	b.BeginContainer()
	for _, v := range set.Elements {
		if err := buildValue(b, v); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func buildMap(b *zcode.Builder, m *Map) error {
	b.BeginContainer()
	for _, entry := range m.Entries {
		if err := buildValue(b, entry.Key); err != nil {
			return err
		}
		if err := buildValue(b, entry.Value); err != nil {
			return err
		}
	}
	b.EndContainer()
	return nil
}

func buildUnion(b *zcode.Builder, union *Union) error {
	if selector := union.Selector; selector >= 0 {
		b.BeginContainer()
		b.AppendPrimitive(zed.EncodeInt(int64(union.Selector)))
		if err := buildValue(b, union.Value); err != nil {
			return err
		}
		b.EndContainer()
	} else {
		b.AppendNull()
	}
	return nil
}

func buildEnum(b *zcode.Builder, enum *Enum) error {
	typ, ok := enum.Type.(*zed.TypeEnum)
	if !ok {
		// This shouldn't happen.
		return errors.New("enum value is not of type enum")
	}
	selector := typ.Lookup(enum.Name)
	b.AppendPrimitive(zed.EncodeUint(uint64(selector)))
	return nil
}

func buildTypeValue(b *zcode.Builder, tv *TypeValue) error {
	b.AppendPrimitive(zed.EncodeTypeValue(tv.Value))
	return nil
}
