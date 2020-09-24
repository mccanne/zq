package zngnative

import (
	"math"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zng"
)

// CoerceToFloat64 attempts to convert a value to a float64. The
// resulting coerced value is written to out, and true is returned. If
// the value cannot be coerced, then false is returned.
func CoerceToFloat64(in zng.Value) (float64, bool) {
	native, err := ToNativeValue(in)
	if err != nil {
		return 0, false
	}
	return CoerceNativeToFloat64(native)
}

func CoerceNativeToFloat64(in Value) (float64, bool) {
	switch in.Type.ID() {
	case zng.IdFloat64:
		return in.Value.(float64), true
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		return float64(in.Value.(int64)), true
	case zng.IdByte, zng.IdUint16, zng.IdUint32, zng.IdUint64:
		return float64(in.Value.(uint64)), true
	case zng.IdTime, zng.IdDuration:
		return float64(in.Value.(int64)) / 1e9, true
	default:
		return 0, false
	}
}

// CoerceToInt attempts to convert a value to a signed integer.
// This always succeeds for signed integer types and succeeds for
// unsigned integers so long as they are not greater than MaxInt64.
// A float64 may be coerced only if the value is equivalent.
// Time and Intervals are converted as their nanosecond values.
func CoerceToInt(in zng.Value) (int64, bool) {
	native, err := ToNativeValue(in)
	if err != nil {
		return 0, false
	}
	return CoerceNativeToInt(native)
}

func CoerceNativeToInt(in Value) (int64, bool) {
	switch in.Type.ID() {
	case zng.IdFloat64:
		f := in.Value.(float64)
		i := int64(f)
		if float64(i) != f {
			return 0, false
		}
		return i, true
	case zng.IdTime, zng.IdDuration:
		return int64(in.Value.(int64) / 1e9), true
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		return in.Value.(int64), true
	case zng.IdByte, zng.IdUint16, zng.IdUint32, zng.IdUint64:
		u := in.Value.(uint64)
		// Ensure this fits in a signed int64 type.
		if u > math.MaxInt64 {
			return 0, false
		}
		return int64(u), true
	default:
		// can't be cast to integer
		return 0, false
	}
}

// CoerceToUint attempts to convert a value to an unsigned integer.
// This always succeeds for unsigned integer types and succeeds for
// signed integers so long as they are not negative.
// A float64 may be coerced only if the value is equivalent.
// Time and Intervals are converted as their nanosecond values.
func CoerceToUint(in zng.Value) (uint64, bool) {
	native, err := ToNativeValue(in)
	if err != nil {
		return 0, false
	}
	return CoerceNativeToUint(native)
}

func CoerceNativeToUint(in Value) (uint64, bool) {
	switch in.Type.ID() {
	case zng.IdFloat64:
		f := in.Value.(float64)
		i := uint64(f)
		if float64(i) != f {
			return 0, false
		}
		return i, true
	case zng.IdTime, zng.IdDuration:
		return uint64(in.Value.(uint64) / 1e9), true
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		si := in.Value.(int64)
		if si < 0 {
			return 0, false
		}
		return uint64(si), true
	case zng.IdByte, zng.IdUint16, zng.IdUint32, zng.IdUint64:
		return in.Value.(uint64), true
	default:
		// can't be cast to unsigned integer
		return 0, false
	}
}

// CoerceToDuration attempts to convert a value to a duration.  Int
// and Double are converted as seconds. The resulting coerced value is
// written to out, and true is returned. If the value cannot be
// coerced, then false is returned.
func CoerceToDuration(in zng.Value) (int64, bool) {
	var out int64
	var err error
	switch in.Type.ID() {
	default:
		return 0, false
	case zng.IdDuration:
		out, err = zng.DecodeDuration(in.Bytes)
	case zng.IdUint16, zng.IdUint32, zng.IdUint64:
		var v uint64
		v, err = zng.DecodeUint(in.Bytes)
		// check for overflow
		if v > math.MaxInt64 {
			return 0, false
		}
		out = 1_000_000_000 * int64(v)
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		out, err = zng.DecodeInt(in.Bytes)
		out *= 1_000_000_000
	case zng.IdFloat64:
		var v float64
		v, err = zng.DecodeFloat64(in.Bytes)
		v *= 1e9
		out = int64(v)
	}
	if err != nil {
		return 0, false
	}
	return out, true
}

func CoerceToPort(in zng.Value) (uint32, bool) {
	var out uint32
	var err error
	body := in.Bytes
	switch in.Type.ID() {
	default:
		return 0, false
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		var v int64
		v, err = zng.DecodeInt(body)
		// check for overflow
		if v < 0 || v > math.MaxUint16 {
			return 0, false
		}
		out = uint32(v)
	case zng.IdUint16, zng.IdUint32, zng.IdUint64:
		var v uint64
		v, err = zng.DecodeUint(body)
		// check for overflow
		if v > math.MaxUint16 {
			return 0, false
		}
		out = uint32(v)
	case zng.IdPort:
		out, err = zng.DecodePort(body)
	case zng.IdFloat64:
		var v float64
		v, err = zng.DecodeFloat64(body)
		out = uint32(v)
		if float64(out) != v {
			return 0, false
		}
	}
	if err != nil {
		return 0, false
	}
	return out, true
}

// CoerceToTime attempts to convert a value to a time. Int and Double
// are converted as seconds. The resulting coerced value is written to
// out, and true is returned. If the value cannot be coerced, then
// false is returned.
func CoerceToTime(in zng.Value) (nano.Ts, bool) {
	native, err := ToNativeValue(in)
	if err != nil {
		return 0, false
	}
	return CoerceNativeToTime(native)
}

func CoerceNativeToTime(in Value) (nano.Ts, bool) {
	var err error
	var ts nano.Ts
	switch in.Type.ID() {
	default:
		return 0, false
	case zng.IdTime:
		ts = nano.Ts(in.Value.(int64))
	case zng.IdInt16, zng.IdInt32, zng.IdInt64:
		ts = nano.Ts(in.Value.(int64)) * 1_000_000_000
	case zng.IdUint16, zng.IdUint32, zng.IdUint64:
		v := in.Value.(uint64)
		// check for overflow
		if v > math.MaxInt64 {
			return 0, false
		}
		ts = nano.Ts(v)
	case zng.IdFloat64:
		ts = nano.Ts(in.Value.(float64) * 1e9)
	}
	if err != nil {
		return 0, false
	}
	return ts, true
}

func CoerceToString(in zng.Value) (string, bool) {
	switch in.Type.ID() {
	default:
		return "", false
	case zng.IdString, zng.IdBstring, zng.IdEnum, zng.IdBytes:
		return string(in.Bytes), true
	}
}
