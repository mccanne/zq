package zngnative

import (
	"errors"
	"fmt"
	"net"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zng"
)

type Value struct {
	Type  zng.Type
	Value interface{}
}

func ToNativeValue(zv zng.Value) (Value, error) {
	switch zv.Type.ID() {
	case zng.IdBool:
		b, err := zng.DecodeBool(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zng.TypeBool, b}, nil

	case zng.IdByte:
		b, err := zng.DecodeByte(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zng.TypeByte, uint64(b)}, nil

	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		v, err := zng.DecodeInt(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, v}, nil

	case zng.IdUint16, zng.IdUint32, zng.IdUint64:
		v, err := zng.DecodeUint(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, v}, nil

	case zng.IdFloat64:
		v, err := zng.DecodeFloat64(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, v}, nil

	case zng.IdString:
		s, err := zng.DecodeString(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, s}, nil

	case zng.IdBytes:
		b, err := zng.DecodeBytes(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, b}, nil

	case zng.IdBstring:
		s, err := zng.DecodeBstring(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, s}, nil

	case zng.IdIP:
		a, err := zng.DecodeIP(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, a}, nil

	case zng.IdPort:
		p, err := zng.DecodePort(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, uint64(p)}, nil

	case zng.IdNet:
		n, err := zng.DecodeNet(zv.Bytes)
		if err != nil {
			return Value{}, err
		}
		return Value{zv.Type, n}, nil

	case zng.IdTime:
		t, err := zng.DecodeTime(zv.Bytes)
		if err != nil {
			return Value{}, nil
		}
		return Value{zv.Type, int64(t)}, nil

	case zng.IdDuration:
		d, err := zng.DecodeDuration(zv.Bytes)
		if err != nil {
			return Value{}, nil
		}
		return Value{zv.Type, d}, nil
	}

	// Keep arrays, sets, and records in their zval encoded form.
	// The purpose of Value is to avoid encoding temporary
	// values but since we can't construct these types in expressions,
	// this just lets us lazily decode them.
	switch zv.Type.(type) {
	case *zng.TypeArray, *zng.TypeSet, *zng.TypeRecord:
		return Value{zv.Type, zv.Bytes}, nil
	}

	return Value{}, fmt.Errorf("unknown type %s", zv.Type)
}

func (v *Value) ToZngValue() (zng.Value, error) {
	switch v.Type.ID() {
	case zng.IdBool:
		b := v.Value.(bool)
		return zng.Value{zng.TypeBool, zng.EncodeBool(b)}, nil

	case zng.IdByte:
		b := v.Value.(uint64)
		return zng.Value{zng.TypeByte, zng.EncodeByte(byte(b))}, nil

	case zng.IdInt16:
		i := v.Value.(int64)
		return zng.Value{zng.TypeInt16, zng.EncodeInt(i)}, nil

	case zng.IdInt32:
		i := v.Value.(int64)
		return zng.Value{zng.TypeInt32, zng.EncodeInt(i)}, nil

	case zng.IdInt64:
		i := v.Value.(int64)
		return zng.Value{zng.TypeInt64, zng.EncodeInt(i)}, nil

	case zng.IdUint16:
		i := v.Value.(uint64)
		return zng.Value{zng.TypeUint16, zng.EncodeUint(i)}, nil

	case zng.IdUint32:
		i := v.Value.(uint64)
		return zng.Value{zng.TypeUint32, zng.EncodeUint(i)}, nil

	case zng.IdUint64:
		i := v.Value.(uint64)
		return zng.Value{zng.TypeUint64, zng.EncodeUint(i)}, nil

	case zng.IdFloat64:
		f := v.Value.(float64)
		return zng.Value{zng.TypeFloat64, zng.EncodeFloat64(f)}, nil

	case zng.IdString:
		s := v.Value.(string)
		return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil

	case zng.IdBytes:
		b := v.Value.([]byte)
		return zng.Value{zng.TypeBytes, zng.EncodeBytes(b)}, nil

	case zng.IdBstring:
		s := v.Value.(string)
		return zng.Value{zng.TypeBstring, zng.EncodeBstring(s)}, nil

	case zng.IdIP:
		i := v.Value.(net.IP)
		return zng.Value{zng.TypeIP, zng.EncodeIP(i)}, nil

	case zng.IdPort:
		p := v.Value.(uint64)
		return zng.Value{zng.TypePort, zng.EncodePort(uint32(p))}, nil

	case zng.IdNet:
		n := v.Value.(*net.IPNet)
		return zng.Value{zng.TypeNet, zng.EncodeNet(n)}, nil

	case zng.IdTime:
		t := nano.Ts(v.Value.(int64))
		return zng.Value{zng.TypeTime, zng.EncodeTime(t)}, nil

	case zng.IdDuration:
		d := v.Value.(int64)
		return zng.Value{zng.TypeDuration, zng.EncodeDuration(d)}, nil
	}

	// Arrays, sets, and records are just zval encoded.
	switch v.Type.(type) {
	case *zng.TypeArray, *zng.TypeSet, *zng.TypeRecord:
		return zng.Value{v.Type, v.Value.(zcode.Bytes)}, nil
	}

	return zng.Value{}, errors.New("unknown type")
}
