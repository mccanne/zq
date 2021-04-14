package expr

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"regexp"
	"regexp/syntax"

	//XXX this shouldn't be reaching into the AST but we'll leave it for
	// now until we factor-in the flow-based package
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/pkg/byteconv"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

//XXX TBD:
// - change these comparisons to work in the zcode.Bytes domain
// - add timer, interval comparisons when we add time, interval literals to the language
// - add count comparisons when we add count literals to the language
// - add set/array/record comparisons when we add container literals to the language

// Predicate is a function that takes a Value and returns a boolean result
// based on the typed value.
type Boolean func(zng.Value) bool

var compareBool = map[string]func(bool, bool) bool{
	"=":  func(a, b bool) bool { return a == b },
	"!=": func(a, b bool) bool { return a != b },
	">":  func(a, b bool) bool { return a && !b },
	">=": func(a, b bool) bool { return a || !b },
	"<":  func(a, b bool) bool { return !a && b },
	"<=": func(a, b bool) bool { return !a || b },
}

// CompareBool returns a Predicate that compares zng.Values to a boolean literal
// that must be a boolean or coercible to an integer.  In the later case, the integer
// is converted to a boolean.
func CompareBool(op string, pattern bool) (Boolean, error) {
	compare, ok := compareBool[op]
	if !ok {
		return nil, fmt.Errorf("unknown bool comparator: %s", op)
	}
	return func(v zng.Value) bool {
		if v.Type.ID() != zng.IDBool {
			return false
		}
		b, err := zng.DecodeBool(v.Bytes)
		if err != nil {
			return false
		}
		return compare(b, pattern)
	}, nil
}

var compareInt = map[string]func(int64, int64) bool{
	"=":  func(a, b int64) bool { return a == b },
	"!=": func(a, b int64) bool { return a != b },
	">":  func(a, b int64) bool { return a > b },
	">=": func(a, b int64) bool { return a >= b },
	"<":  func(a, b int64) bool { return a < b },
	"<=": func(a, b int64) bool { return a <= b }}

var compareFloat = map[string]func(float64, float64) bool{
	"=":  func(a, b float64) bool { return a == b },
	"!=": func(a, b float64) bool { return a != b },
	">":  func(a, b float64) bool { return a > b },
	">=": func(a, b float64) bool { return a >= b },
	"<":  func(a, b float64) bool { return a < b },
	"<=": func(a, b float64) bool { return a <= b }}

// Return a predicate for comparing this value to one more typed
// byte slices by calling the predicate function with a Value.
// Operand is one of "=", "!=", "<", "<=", ">", ">=".
func CompareInt64(op string, pattern int64) (Boolean, error) {
	CompareInt, ok1 := compareInt[op]
	CompareFloat, ok2 := compareFloat[op]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("unknown int comparator: %s", op)
	}
	// many different Z data types can be compared with integers
	return func(val zng.Value) bool {
		zv := val.Bytes
		switch val.Type.ID() {
		case zng.IDInt8, zng.IDInt16, zng.IDInt32, zng.IDInt64:
			v, err := zng.DecodeInt(zv)
			if err == nil {
				return CompareInt(v, pattern)
			}
		case zng.IDUint8, zng.IDUint16, zng.IDUint32, zng.IDUint64:
			v, err := zng.DecodeUint(zv)
			if err == nil && v <= math.MaxInt64 {
				return CompareInt(int64(v), pattern)
			}
		case zng.IDFloat64:
			v, err := zng.DecodeFloat64(zv)
			if err == nil {
				return CompareFloat(v, float64(pattern))
			}
		case zng.IDTime:
			ts, err := zng.DecodeTime(zv)
			if err == nil {
				return CompareInt(int64(ts), pattern*1e9)
			}
		case zng.IDDuration:
			v, err := zng.DecodeInt(zv)
			if err == nil {
				return CompareInt(int64(v), pattern*1e9)
			}
		}
		return false
	}, nil
}

func CompareTime(op string, pattern int64) (Boolean, error) {
	CompareInt, ok1 := compareInt[op]
	CompareFloat, ok2 := compareFloat[op]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("unknown int comparator: %s", op)
	}
	// many different Z data types can be compared with integers
	return func(val zng.Value) bool {
		zv := val.Bytes
		switch val.Type.ID() {
		case zng.IDInt8, zng.IDInt16, zng.IDInt32, zng.IDInt64:
			v, err := zng.DecodeInt(zv)
			if err == nil {
				return CompareInt(v, pattern)
			}
		case zng.IDUint8, zng.IDUint16, zng.IDUint32, zng.IDUint64:
			v, err := zng.DecodeUint(zv)
			if err == nil && v <= math.MaxInt64 {
				return CompareInt(int64(v), pattern)
			}
		case zng.IDFloat64:
			v, err := zng.DecodeFloat64(zv)
			if err == nil {
				return CompareFloat(v, float64(pattern))
			}
		case zng.IDTime:
			ts, err := zng.DecodeTime(zv)
			if err == nil {
				return CompareInt(int64(ts), pattern)
			}
		case zng.IDDuration:
			v, err := zng.DecodeInt(zv)
			if err == nil {
				return CompareInt(int64(v), pattern)
			}
		}
		return false
	}, nil
}

//XXX should just do equality and we should compare in the encoded domain
// and not make copies and have separate cases for len 4 and len 16
var compareAddr = map[string]func(net.IP, net.IP) bool{
	"=":  func(a, b net.IP) bool { return a.Equal(b) },
	"!=": func(a, b net.IP) bool { return !a.Equal(b) },
	">":  func(a, b net.IP) bool { return bytes.Compare(a, b) > 0 },
	">=": func(a, b net.IP) bool { return bytes.Compare(a, b) >= 0 },
	"<":  func(a, b net.IP) bool { return bytes.Compare(a, b) < 0 },
	"<=": func(a, b net.IP) bool { return bytes.Compare(a, b) <= 0 },
}

// Comparison returns a Predicate that compares typed byte slices that must
// be TypeAddr with the value's address using a comparison based on op.
// Only equality operands are allowed.
func CompareIP(op string, pattern net.IP) (Boolean, error) {
	compare, ok := compareAddr[op]
	if !ok {
		return nil, fmt.Errorf("unknown addr comparator: %s", op)
	}
	return func(v zng.Value) bool {
		if v.Type.ID() != zng.IDIP {
			return false
		}
		ip, err := zng.DecodeIP(v.Bytes)
		if err != nil {
			return false
		}
		return compare(ip, pattern)
	}, nil
}

// CompareFloat64 returns a Predicate that compares typed byte slices that must
// be coercible to an double with the value's double value using a comparison
// based on op.  Int, count, port, and double types can
// all be converted to the integer value.  XXX there are some overflow issues here.
func CompareFloat64(op string, pattern float64) (Boolean, error) {
	compare, ok := compareFloat[op]
	if !ok {
		return nil, fmt.Errorf("unknown double comparator: %s", op)
	}
	return func(val zng.Value) bool {
		zv := val.Bytes
		switch val.Type.ID() {
		// We allow comparison of float constant with integer-y
		// fields and just use typeDouble to parse since it will do
		// the right thing for integers.  XXX do we want to allow
		// integers that cause float64 overflow?  user can always
		// use an integer constant instead of a float constant to
		// compare with the integer-y field.
		case zng.IDFloat64:
			v, err := zng.DecodeFloat64(zv)
			if err == nil {
				return compare(v, pattern)
			}
		case zng.IDInt8, zng.IDInt16, zng.IDInt32, zng.IDInt64:
			v, err := zng.DecodeInt(zv)
			if err == nil {
				return compare(float64(v), pattern)
			}
		case zng.IDUint8, zng.IDUint16, zng.IDUint32, zng.IDUint64:
			v, err := zng.DecodeUint(zv)
			if err == nil {
				return compare(float64(v), pattern)
			}
		case zng.IDTime:
			ts, err := zng.DecodeTime(zv)
			if err == nil {
				return compare(float64(ts)/1e9, pattern)
			}
		case zng.IDDuration:
			v, err := zng.DecodeDuration(zv)
			if err == nil {
				return compare(float64(v)/1e9, pattern)
			}
		}
		return false
	}, nil
}

var compareString = map[string]func(string, string) bool{
	"=":  func(a, b string) bool { return a == b },
	"!=": func(a, b string) bool { return a != b },
	">":  func(a, b string) bool { return a > b },
	">=": func(a, b string) bool { return a >= b },
	"<":  func(a, b string) bool { return a < b },
	"<=": func(a, b string) bool { return a <= b },
}

func CompareBstring(op string, pattern []byte) (Boolean, error) {
	compare, ok := compareString[op]
	if !ok {
		return nil, fmt.Errorf("unknown string comparator: %s", op)
	}
	s := string(pattern)
	return func(v zng.Value) bool {
		switch v.Type.ID() {
		case zng.IDBstring, zng.IDString:
			return compare(byteconv.UnsafeString(v.Bytes), s)
		}
		return false
	}, nil
}

func CompileRegexp(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(string(zng.UnescapeBstring([]byte(pattern))))
	if err != nil {
		if syntaxErr, ok := err.(*syntax.Error); ok {
			syntaxErr.Expr = pattern
		}
		return nil, err
	}
	return re, err
}

// NewRegexpBoolean returns a Booelan that compares values that must
// be a stringy the given regexp.
func NewRegexpBoolean(re *regexp.Regexp) Boolean {
	return func(v zng.Value) bool {
		if zng.IsStringy(v.Type.ID()) {
			return re.Match(v.Bytes)
		}
		return false
	}
}

func CompareUnset(op string) (Boolean, error) {
	switch op {
	case "=":
		return func(v zng.Value) bool {
			return v.IsUnset()
		}, nil
	case "!=":
		return func(v zng.Value) bool {
			return !v.IsUnset()
		}, nil
	default:
		return nil, fmt.Errorf("unknown unset comparator: %s", op)
	}
}

// a better way to do this would be to compare IP's and mask's but
// go doesn't provide an easy way to compare masks so we do this
// hacky thing and compare strings
var compareSubnet = map[string]func(*net.IPNet, *net.IPNet) bool{
	"=":  func(a, b *net.IPNet) bool { return bytes.Equal(a.IP, b.IP) },
	"!=": func(a, b *net.IPNet) bool { return bytes.Equal(a.IP, b.IP) },
	"<":  func(a, b *net.IPNet) bool { return bytes.Compare(a.IP, b.IP) < 0 },
	"<=": func(a, b *net.IPNet) bool { return bytes.Compare(a.IP, b.IP) <= 0 },
	">":  func(a, b *net.IPNet) bool { return bytes.Compare(a.IP, b.IP) > 0 },
	">=": func(a, b *net.IPNet) bool { return bytes.Compare(a.IP, b.IP) >= 0 },
}

var matchSubnet = map[string]func(net.IP, *net.IPNet) bool{
	"=": func(a net.IP, b *net.IPNet) bool {
		ok := b.IP.Equal(a.Mask(b.Mask))
		return ok
	},
	"!=": func(a net.IP, b *net.IPNet) bool {
		return !b.IP.Equal(a.Mask(b.Mask))
	},
	"<": func(a net.IP, b *net.IPNet) bool {
		net := a.Mask(b.Mask)
		return bytes.Compare(net, b.IP) < 0
	},
	"<=": func(a net.IP, b *net.IPNet) bool {
		net := a.Mask(b.Mask)
		return bytes.Compare(net, b.IP) <= 0
	},
	">": func(a net.IP, b *net.IPNet) bool {
		net := a.Mask(b.Mask)
		return bytes.Compare(net, b.IP) > 0
	},
	">=": func(a net.IP, b *net.IPNet) bool {
		net := a.Mask(b.Mask)
		return bytes.Compare(net, b.IP) >= 0
	},
}

// Comparison returns a Predicate that compares typed byte slices that must
// be an addr or a subnet to the value's subnet value using a comparison
// based on op.  Onluy equalty and inequality are permitted.  If the typed
// byte slice is a subnet, then the comparison is based on strict equality.
// If the typed byte slice is an addr, then the comparison is performed by
// doing a CIDR match on the address with the subnet.
func CompareSubnet(op string, pattern *net.IPNet) (Boolean, error) {
	compare, ok1 := compareSubnet[op]
	match, ok2 := matchSubnet[op]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("unknown subnet comparator: %s", op)
	}
	return func(v zng.Value) bool {
		val := v.Bytes
		switch v.Type.ID() {
		case zng.IDIP:
			ip, err := zng.DecodeIP(val)
			if err == nil {
				return match(ip, pattern)
			}
		case zng.IDNet:
			subnet, err := zng.DecodeNet(val)
			if err == nil {
				return compare(subnet, pattern)
			}
		}
		return false
	}, nil
}

// Given a predicate for comparing individual elements, produce a new
// predicate that implements the "in" comparison.  The new predicate looks
// at the type of the value being compared, if it is a set or array,
// the original predicate is applied to each element.  The new precicate
// returns true iff the predicate matched an element from the collection.
func Contains(compare Boolean) Boolean {
	return func(v zng.Value) bool {
		var el zng.Value
		el.Type = zng.InnerType(v.Type)
		if el.Type == nil {
			return false
		}
		for it := v.Iter(); !it.Done(); {
			var err error
			el.Bytes, _, err = it.Next()
			if err != nil {
				return false
			}
			if compare(el) {
				return true
			}
		}
		return false
	}
}

// Comparison returns a Predicate for comparing this value to other values.
// The op argument is one of "=", "!=", "<", "<=", ">", ">=".
// See the comments of the various type implementations
// of this method as some types limit the operand to equality and
// the various types handle coercion in different ways.
func Comparison(op string, primitive zed.Primitive) (Boolean, error) {
	// String literals inside zql are parsed as zng bstrings
	// (since bstrings can represent a wider range of values,
	// specifically arrays of bytes that do not correspond to
	// UTF-8 encoded strings).
	if primitive.Type == "string" {
		primitive = zed.Primitive{Kind: "Primitive", Type: "bstring", Text: primitive.Text}
	}
	zv, err := zson.ParsePrimitive(primitive.Type, primitive.Text)
	if err != nil {
		return nil, err
	}
	switch zv.Type.(type) {
	case *zng.TypeOfNull:
		return CompareUnset(op)
	case *zng.TypeOfIP:
		v, err := zng.DecodeIP(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareIP(op, v)
	case *zng.TypeOfNet:
		v, err := zng.DecodeNet(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareSubnet(op, v)
	case *zng.TypeOfBool:
		v, err := zng.DecodeBool(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareBool(op, v)
	case *zng.TypeOfFloat64:
		v, err := zng.DecodeFloat64(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareFloat64(op, v)
	case *zng.TypeOfString, *zng.TypeOfBstring, *zng.TypeOfType, *zng.TypeOfError:
		return CompareBstring(op, zv.Bytes)
	case *zng.TypeOfInt64:
		v, err := zng.DecodeInt(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareInt64(op, v)
	case *zng.TypeOfTime, *zng.TypeOfDuration:
		v, err := zng.DecodeInt(zv.Bytes)
		if err != nil {
			return nil, err
		}
		return CompareTime(op, v)
	default:
		return nil, fmt.Errorf("literal comparison of type %q unsupported", zv.Type.ZSON())
	}
}
