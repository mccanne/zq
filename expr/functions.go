package expr

import (
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zng"
)

type Args struct {
	vals []zng.Value
	scratch
}

func NewArgs(n int, keep bool) *Args {
	return &Args{
		vals:    make([]zng.Value, n),
		scratch: scratch{keep: keep},
	}
}

type Function func(*Args) (zng.Value, error)

var ErrTooFewArgs = errors.New("too few arguments")
var ErrTooManyArgs = errors.New("too many arguments")
var ErrBadArgument = errors.New("bad argument")

var allFns = map[string]struct {
	minArgs int
	maxArgs int
	impl    Function
}{
	"len": {1, 1, lenFn},

	"Math.abs":   {1, 1, mathAbs},
	"Math.ceil":  {1, 1, mathCeil},
	"Math.floor": {1, 1, mathFloor},
	"Math.log":   {1, 1, mathLog},
	"Math.max":   {1, -1, mathMax},
	"Math.min":   {1, -1, mathMin},
	"Math.mod":   {2, 2, mathMod},
	"Math.round": {1, 1, mathRound},
	"Math.pow":   {2, 2, mathPow},
	"Math.sqrt":  {1, 1, mathSqrt},

	"String.byteLen":     {1, 1, stringByteLen},
	"String.formatFloat": {1, 1, stringFormatFloat},
	"String.formatInt":   {1, 1, stringFormatInt},
	"String.formatIp":    {1, 1, stringFormatIp},
	"String.parseFloat":  {1, 1, stringParseFloat},
	"String.parseInt":    {1, 1, stringParseInt},
	"String.parseIp":     {1, 1, stringParseIp},
	"String.replace":     {3, 3, stringReplace},
	"String.runeLen":     {1, 1, stringRuneLen},
	"String.toLower":     {1, 1, stringToLower},
	"String.toUpper":     {1, 1, stringToUpper},
	"String.trim":        {1, 1, stringTrim},

	"Time.fromISO":          {1, 1, timeFromISO},
	"Time.fromMilliseconds": {1, 1, timeFromMsec},
	"Time.fromMicroseconds": {1, 1, timeFromUsec},
	"Time.fromNanoseconds":  {1, 1, timeFromNsec},
	"Time.trunc":            {2, 2, timeTrunc},

	"typeof": {1, 1, typeOf},
	"iserr":  {1, 1, isErr},
}

//XXX this should be renamed so as not to clash with the conventional
// id "err" used for the local variable
func err(fn string, err error) (zng.Value, error) {
	return zng.Value{}, fmt.Errorf("%s: %w", fn, err)
}

func lenFn(args *Args) (zng.Value, error) {
	switch zng.AliasedType(args.vals[0].Type).(type) {
	case *zng.TypeArray, *zng.TypeSet:
		v := args.vals[0]
		len, err := v.ContainerLength()
		if err != nil {
			return zng.Value{}, err
		}
		return zng.Value{zng.TypeInt64, args.Int(int64(len))}, nil
	default:
		return err("len", ErrBadArgument)
	}
}

func mathAbs(args *Args) (zng.Value, error) {
	v := args.vals[0]
	id := v.Type.ID()
	if zng.IsFloat(id) {
		f, _ := zng.DecodeFloat64(v.Bytes)
		f = math.Abs(f)
		return zng.Value{zng.TypeFloat64, args.Float64(f)}, nil
	}
	if !zng.IsInteger(id) {
		return err("Math.abs", ErrBadArgument)
	}
	if !zng.IsSigned(id) {
		return v, nil
	}
	x, _ := zng.DecodeInt(v.Bytes)
	if x < 0 {
		x = -x
	}
	return zng.Value{v.Type, args.Int(x)}, nil
}

func mathCeil(args *Args) (zng.Value, error) {
	v := args.vals[0]
	id := v.Type.ID()
	if zng.IsFloat(id) {
		f, _ := zng.DecodeFloat64(v.Bytes)
		f = math.Ceil(f)
		return zng.Value{zng.TypeFloat64, args.Float64(f)}, nil
	}
	if zng.IsInteger(id) {
		return v, nil
	}
	return err("Math.Ceil", ErrBadArgument)
}

func mathFloor(args *Args) (zng.Value, error) {
	v := args.vals[0]
	id := v.Type.ID()
	if zng.IsFloat(id) {
		f, _ := zng.DecodeFloat64(v.Bytes)
		f = math.Floor(f)
		return zng.Value{zng.TypeFloat64, args.Float64(f)}, nil
	}
	if zng.IsInteger(id) {
		return v, nil
	}
	return err("Math.Floor", ErrBadArgument)
}

func mathLog(args *Args) (zng.Value, error) {
	x, ok := CoerceToFloat(args.vals[0])
	// XXX should have better error messages
	if !ok {
		return err("Math.log", ErrBadArgument)
	}
	if x <= 0 {
		return err("Math.log", ErrBadArgument)
	}
	return zng.Value{zng.TypeFloat64, args.Float64(math.Log(x))}, nil
}

type reducer struct {
	f64 func(float64, float64) float64
	i64 func(int64, int64) int64
	u64 func(uint64, uint64) uint64
}

var min = &reducer{
	f64: func(a, b float64) float64 {
		if a < b {
			return a
		}
		return b
	},
	i64: func(a, b int64) int64 {
		if a < b {
			return a
		}
		return b
	},
	u64: func(a, b uint64) uint64 {
		if a < b {
			return a
		}
		return b
	},
}

var max = &reducer{
	f64: func(a, b float64) float64 {
		if a > b {
			return a
		}
		return b
	},
	i64: func(a, b int64) int64 {
		if a > b {
			return a
		}
		return b
	},
	u64: func(a, b uint64) uint64 {
		if a > b {
			return a
		}
		return b
	},
}

func mathMax(args *Args) (zng.Value, error) {
	return reduce(args, max)
}

func mathMin(args *Args) (zng.Value, error) {
	return reduce(args, min)
}

func reduce(args *Args, fn *reducer) (zng.Value, error) {
	zv := args.vals[0]
	typ := zv.Type
	id := typ.ID()
	if zng.IsFloat(id) {
		result, _ := zng.DecodeFloat64(zv.Bytes)
		for _, zv := range args.vals[1:] {
			v, ok := CoerceToFloat(zv)
			if !ok {
				return zng.Value{}, ErrBadArgument
			}
			result = fn.f64(result, v)
		}
		return zng.Value{typ, args.Float64(result)}, nil
	}
	if !zng.IsNumber(id) {
		// XXX better message
		return zng.Value{}, ErrBadArgument
	}
	if zng.IsSigned(id) {
		result, _ := zng.DecodeInt(zv.Bytes)
		for _, zv := range args.vals[1:] {
			v, ok := CoerceToInt(zv)
			if !ok {
				// XXX better message
				return zng.Value{}, ErrBadArgument
			}
			result = fn.i64(result, v)
		}
		return zng.Value{typ, args.Int(result)}, nil
	}
	result, _ := zng.DecodeUint(zv.Bytes)
	for _, zv := range args.vals[1:] {
		v, ok := CoerceToUint(zv)
		if !ok {
			// XXX better message
			return zng.Value{}, ErrBadArgument
		}
		result = fn.u64(result, v)
	}
	return zng.Value{typ, args.Uint(result)}, nil
}

//XXX currently integer mod, but this could also do fmod
// also why doesn't zql have x%y instead of Math.mod(x,y)?
func mathMod(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	id := zv.Type.ID()
	if zng.IsFloat(id) {
		return err("Math.mod", ErrBadArgument)
	}
	y, ok := CoerceToUint(args.vals[1])
	if !ok {
		return err("Math.mod", ErrBadArgument)
	}
	if !zng.IsNumber(id) {
		return err("Math.mod", ErrBadArgument)
	}
	if zng.IsSigned(id) {
		x, _ := zng.DecodeInt(zv.Bytes)
		return zng.Value{zv.Type, args.Int(x % int64(y))}, nil
	}
	x, _ := zng.DecodeUint(zv.Bytes)
	return zng.Value{zv.Type, args.Uint(x % y)}, nil
}

func mathRound(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	id := zv.Type.ID()
	if zng.IsFloat(id) {
		f, _ := zng.DecodeFloat64(zv.Bytes)
		return zng.Value{zv.Type, args.Float64(math.Round(f))}, nil

	}
	if !zng.IsNumber(id) {
		return err("Math.round", ErrBadArgument)
	}
	return zv, nil
}

func mathPow(args *Args) (zng.Value, error) {
	x, ok := CoerceToFloat(args.vals[0])
	if !ok {
		return err("Math.pow", ErrBadArgument)
	}
	y, ok := CoerceToFloat(args.vals[1])
	if !ok {
		return err("Math.pow", ErrBadArgument)
	}
	r := math.Pow(x, y)
	if math.IsNaN(r) {
		return err("Math.pow", ErrBadArgument)
	}
	return zng.Value{zng.TypeFloat64, args.Float64(r)}, nil
}

func mathSqrt(args *Args) (zng.Value, error) {
	x, ok := CoerceToFloat(args.vals[0])
	if !ok {
		return err("Math.sqrt", ErrBadArgument)
	}
	x = math.Sqrt(x)
	if math.IsNaN(x) {
		// For now we can't represent non-numeric values in a float64,
		// we will revisit this but it has implications for file
		// formats, zql, etc.
		return err("Math.sqrt", ErrBadArgument)
	}
	return zng.Value{zng.TypeFloat64, args.Float64(x)}, nil
}

// XXX we should just have a len function that applies to different types
// and a way to get unicode char len, charlen()?
func stringByteLen(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !zng.IsStringy(zv.Type.ID()) {
		return err("Strings.byteLen", ErrBadArgument)
	}
	v := len(string(zv.Bytes))
	return zng.Value{zng.TypeInt64, args.Int(int64(v))}, nil
}

func stringFormatFloat(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if zv.Type.ID() != zng.IdFloat64 {
		return err("string.floatToString", ErrBadArgument)
	}
	f, _ := zng.DecodeFloat64(zv.Bytes)
	s := strconv.FormatFloat(f, 'g', -1, 64)
	return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil
}

func stringFormatInt(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	id := zv.Type.ID()
	var s string
	if !zng.IsInteger(id) {
		return err("string.intToString", ErrBadArgument)
	}
	if zng.IsSigned(id) {
		v, _ := zng.DecodeInt(zv.Bytes)
		// XXX GC
		s = strconv.FormatInt(v, 10)
	} else {
		v, _ := zng.DecodeUint(zv.Bytes)
		// XXX GC
		s = strconv.FormatUint(v, 10)
	}
	return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil
}

func stringFormatIp(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if zv.Type.ID() != zng.IdIP {
		return err("string.ipToString", ErrBadArgument)
	}
	ip, _ := zng.DecodeIP(zv.Bytes)
	return zng.Value{zng.TypeString, zng.EncodeString(ip.String())}, nil
}

func stringParseInt(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !zng.IsStringy(zv.Type.ID()) {
		return err("String.parseInt", ErrBadArgument)
	}
	v, e := zng.DecodeString(zv.Bytes)
	if e != nil {
		return zng.Value{}, e
	}
	i, perr := strconv.ParseInt(v, 10, 64)
	if perr != nil {
		// Get rid of the strconv wrapping gunk to get the
		// actual error message
		e := perr.(*strconv.NumError)
		return err("String.parseInt", e.Err)
	}
	return zng.Value{zng.TypeInt64, args.Int(i)}, nil
}

func stringParseFloat(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !zng.IsStringy(zv.Type.ID()) {
		return err("String.parseFloat", ErrBadArgument)
	}
	v, perr := zng.DecodeString(zv.Bytes)
	if perr != nil {
		return zng.Value{}, perr
	}
	f, perr := strconv.ParseFloat(v, 64)
	if perr != nil {
		// Get rid of the strconv wrapping gunk to get the
		// actual error message
		e := perr.(*strconv.NumError)
		return err("String.parseFloat", e.Err)
	}
	return zng.Value{zng.TypeFloat64, args.Float64(f)}, nil
}

func stringParseIp(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !zng.IsStringy(zv.Type.ID()) {
		return err("String.parseIp", ErrBadArgument)
	}
	v, perr := zng.DecodeString(zv.Bytes)
	if perr != nil {
		return zng.Value{}, perr
	}
	// XXX GC
	a := net.ParseIP(v)
	if a == nil {
		return err("String.parseIp", ErrBadArgument)
	}
	// XXX GC
	return zng.Value{zng.TypeIP, zng.EncodeIP(a)}, nil
}

func isStringy(v zng.Value) bool {
	return zng.IsStringy(v.Type.ID())
}

func stringReplace(args *Args) (zng.Value, error) {
	zvs := args.vals[0]
	zvold := args.vals[1]
	zvnew := args.vals[2]
	if !isStringy(zvs) || !isStringy(zvold) || !isStringy(zvnew) {
		return err("String.replace", ErrBadArgument)
	}
	s, err := zng.DecodeString(zvs.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	old, err := zng.DecodeString(zvold.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	new, err := zng.DecodeString(zvnew.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	result := strings.ReplaceAll(s, old, new)
	return zng.Value{zng.TypeString, zng.EncodeString(result)}, nil
}

func stringRuneLen(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !isStringy(zv) {
		return err("Strings.byteLen", ErrBadArgument)
	}
	s, err := zng.DecodeString(zv.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	v := utf8.RuneCountInString(s)
	return zng.Value{zng.TypeInt64, args.Int(int64(v))}, nil
}

func stringToLower(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !isStringy(zv) {
		return err("String.toLower", ErrBadArgument)
	}
	s, err := zng.DecodeString(zv.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	// XXX GC
	s = strings.ToLower(s)
	return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil
}

func stringToUpper(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !isStringy(zv) {
		return err("String.toUpper", ErrBadArgument)
	}
	s, err := zng.DecodeString(zv.Bytes)
	if err != nil {
		return zng.Value{}, err
	}
	// XXX GC
	s = strings.ToUpper(s)
	return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil
}

func stringTrim(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !isStringy(zv) {
		return err("String.trim", ErrBadArgument)
	}
	// XXX GC
	s := strings.TrimSpace(string(zv.Bytes))
	return zng.Value{zng.TypeString, zng.EncodeString(s)}, nil
}

func timeFromISO(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	if !isStringy(zv) {
		return err("Time.fromISO", ErrBadArgument)
	}
	ts, e := time.Parse(time.RFC3339Nano, string(zv.Bytes))
	if e != nil {
		return err("Time.fromISO", ErrBadArgument)
	}
	return zng.Value{zng.TypeTime, args.Time(nano.Ts(ts.UnixNano()))}, nil
}

func timeFromMsec(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	ms, ok := CoerceToInt(zv)
	if !ok {
		return err("Time.fromMilliseconds", ErrBadArgument)
	}
	return zng.Value{zng.TypeTime, args.Time(nano.Ts(ms * 1_000_000))}, nil
}

func timeFromUsec(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	us, ok := CoerceToInt(zv)
	if !ok {
		return err("Time.fromMicroseconds", ErrBadArgument)
	}
	return zng.Value{zng.TypeTime, args.Time(nano.Ts(us * 1000))}, nil
}

func timeFromNsec(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	ns, ok := CoerceToInt(zv)
	if !ok {
		return err("Time.fromNanoseconds", ErrBadArgument)
	}
	return zng.Value{zng.TypeTime, args.Time(nano.Ts(ns))}, nil
}

func timeTrunc(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	ts, ok := CoerceToTime(zv)
	if !ok {
		return err("Time.trunc", ErrBadArgument)
	}
	dur, ok := CoerceToInt(args.vals[1])
	if !ok {
		return err("Time.trunc", ErrBadArgument)
	}
	dur *= 1_000_000_000
	return zng.Value{zng.TypeTime, args.Time(nano.Ts(ts.Trunc(dur)))}, nil
}

func typeOf(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	return zng.Value{zng.TypeType, zng.EncodeType(zv.Type.String())}, nil
}

func isErr(args *Args) (zng.Value, error) {
	zv := args.vals[0]
	result := zng.True
	if zv.Type != zng.TypeError {
		result = zng.False
	}
	return result, nil
}
