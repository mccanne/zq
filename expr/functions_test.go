package expr_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/brimsec/zq/expr/function"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zng"
)

func zaddr(addr string) zng.Value {
	parsed := net.ParseIP(addr)
	return zng.Value{zng.TypeIP, zng.EncodeIP(parsed)}
}

func TestBadFunction(t *testing.T) {
	testError(t, "notafunction()", "", function.ErrNoSuchFunction, "calling nonexistent function")
}

func TestAbs(t *testing.T) {
	record := `
#0:record[u:uint64]
0:[50;]`

	testSuccessful(t, "abs(-5)", record, zint64(5))
	testSuccessful(t, "abs(5)", record, zint64(5))
	testSuccessful(t, "abs(-3.2)", record, zfloat64(3.2))
	testSuccessful(t, "abs(3.2)", record, zfloat64(3.2))
	testSuccessful(t, "abs(u)", record, zuint64(50))

	testError(t, "abs()", record, function.ErrTooFewArgs, "abs with no args")
	testError(t, "abs(1, 2)", record, function.ErrTooManyArgs, "abs with too many args")
	testError(t, `abs("hello")`, record, function.ErrBadArgument, "abs with non-number")
}

func TestSqrt(t *testing.T) {
	record := `
#0:record[f:float64,i:int32]
0:[6.25;9;]`

	testSuccessful(t, "sqrt(4.0)", record, zfloat64(2.0))
	testSuccessful(t, "sqrt(f)", record, zfloat64(2.5))
	testSuccessful(t, "sqrt(i)", record, zfloat64(3.0))

	testError(t, "sqrt()", record, function.ErrTooFewArgs, "sqrt with no args")
	testError(t, "sqrt(1, 2)", record, function.ErrTooManyArgs, "sqrt with too many args")
	testError(t, "sqrt(-1)", record, function.ErrBadArgument, "sqrt of negative")
}

func TestMinMax(t *testing.T) {
	record := `
#0:record[i:uint64,f:float64]
0:[1;2;]`

	// Simple cases
	testSuccessful(t, "min(1)", record, zint64(1))
	testSuccessful(t, "max(1)", record, zint64(1))
	testSuccessful(t, "min(1, 2, 3)", record, zint64(1))
	testSuccessful(t, "max(1, 2, 3)", record, zint64(3))
	testSuccessful(t, "min(3, 2, 1)", record, zint64(1))
	testSuccessful(t, "max(3, 2, 1)", record, zint64(3))

	// Fails with no arguments
	testError(t, "min()", record, function.ErrTooFewArgs, "min with no args")
	testError(t, "max()", record, function.ErrTooFewArgs, "max with no args")

	// Mixed types work
	testSuccessful(t, "min(i, 2, 3)", record, zuint64(1))
	testSuccessful(t, "min(2, 3, i)", record, zint64(1))
	testSuccessful(t, "max(i, 2, 3)", record, zuint64(3))
	testSuccessful(t, "max(2, 3, i)", record, zint64(3))
	testSuccessful(t, "min(1, -2.0)", record, zint64(-2))
	testSuccessful(t, "min(-2.0, 1)", record, zfloat64(-2))
	testSuccessful(t, "max(-1, 2.0)", record, zint64(2))
	testSuccessful(t, "max(2.0, -1)", record, zfloat64(2))

	// Fails on invalid types
	testError(t, `min("hello", 2)`, record, function.ErrBadArgument, "min() on string")
	testError(t, `max("hello", 2)`, record, function.ErrBadArgument, "max() on string")
	testError(t, `min(1.2.3.4, 2)`, record, function.ErrBadArgument, "min() on ip")
	testError(t, `max(1.2.3.4, 2)`, record, function.ErrBadArgument, "max() on ip")

}

func TestCeilFloorRound(t *testing.T) {
	testSuccessful(t, "ceil(1.5)", "", zfloat64(2))
	testSuccessful(t, "floor(1.5)", "", zfloat64(1))
	testSuccessful(t, "round(1.5)", "", zfloat64(2))

	testSuccessful(t, "ceil(5)", "", zint64(5))
	testSuccessful(t, "floor(5)", "", zint64(5))
	testSuccessful(t, "round(5)", "", zint64(5))

	testError(t, "ceil()", "", function.ErrTooFewArgs, "ceil() with no args")
	testError(t, "ceil(1, 2)", "", function.ErrTooManyArgs, "ceil() with too many args")
	testError(t, "floor()", "", function.ErrTooFewArgs, "floor() with no args")
	testError(t, "floor(1, 2)", "", function.ErrTooManyArgs, "floor() with too many args")
	testError(t, "round()", "", function.ErrTooFewArgs, "round() with no args")
	testError(t, "round(1, 2)", "", function.ErrTooManyArgs, "round() with too many args")
}

func TestLogPow(t *testing.T) {
	// Math.log() computes natural logarithm.  Rather than writing
	// out long floating point numbers in the parameters or results,
	// use more complex expressions that evaluate to simpler values.
	testSuccessful(t, "log(32) / log(2)", "", zfloat64(5))
	testSuccessful(t, "log(32.0) / log(2.0)", "", zfloat64(5))

	testSuccessful(t, "pow(10, 2)", "", zfloat64(100))
	testSuccessful(t, "pow(4.0, 1.5)", "", zfloat64(8))

	testError(t, "log()", "", function.ErrTooFewArgs, "log() with no args")
	testError(t, "log(2, 3)", "", function.ErrTooManyArgs, "log() with too many args")
	testError(t, "log(0)", "", function.ErrBadArgument, "log() of 0")
	testError(t, "log(-1)", "", function.ErrBadArgument, "log() of negative number")

	testError(t, "pow()", "", function.ErrTooFewArgs, "pow() with no args")
	testError(t, "pow(2, 3, r)", "", function.ErrTooManyArgs, "pow() with too many args")
	testError(t, "pow(-1, 0.5)", "", function.ErrBadArgument, "pow() with invalid arguments")
}

func TestMod(t *testing.T) {
	record := `
#0:record[u:uint64]
0:[5;]`

	testSuccessful(t, "mod(5, 3)", record, zint64(2))
	testSuccessful(t, "mod(u, 3)", record, zuint64(2))
	testSuccessful(t, "mod(8, 5)", record, zint64(3))
	testSuccessful(t, "mod(8, u)", record, zint64(3))

	testError(t, "mod()", record, function.ErrTooFewArgs, "mod() with no args")
	testError(t, "mod(1, 2, 3)", record, function.ErrTooManyArgs, "mod() with too many args")
	testError(t, "mod(3.2, 2)", record, function.ErrBadArgument, "mod() with float64 arg")
}

func TestOtherStrFuncs(t *testing.T) {
	testSuccessful(t, `replace("bann", "n", "na")`, "", zstring("banana"))
	testError(t, `replace("foo", "bar")`, "", function.ErrTooFewArgs, "replace() with too few args")
	testError(t, `replace("foo", "bar", "baz", "blort")`, "", function.ErrTooManyArgs, "replace() with too many args")
	testError(t, `replace("foo", "o", 5)`, "", function.ErrBadArgument, "replace() with non-string arg")

	testSuccessful(t, `to_lower("BOO")`, "", zstring("boo"))
	testError(t, `to_lower()`, "", function.ErrTooFewArgs, "toLower() with no args")
	testError(t, `to_lower("BOO", "HOO")`, "", function.ErrTooManyArgs, "toLower() with too many args")

	testSuccessful(t, `to_upper("boo")`, "", zstring("BOO"))
	testError(t, `to_upper()`, "", function.ErrTooFewArgs, "toUpper() with no args")
	testError(t, `to_upper("boo", "hoo")`, "", function.ErrTooManyArgs, "toUpper() with too many args")

	testSuccessful(t, `trim("  hi  there   ")`, "", zstring("hi  there"))
	testError(t, `trim()`, "", function.ErrTooFewArgs, "trim() with no args")
	testError(t, `trim("  hi  ", "  there  ")`, "", function.ErrTooManyArgs, "trim() with too many args")
}

func TestLen(t *testing.T) {
	record := `
#0:record[s:set[int32],a:array[int32]]
0:[[1;2;3;][4;5;6;]]`

	testSuccessful(t, "len(s)", record, zint64(3))
	testSuccessful(t, "len(a)", record, zint64(3))

	testError(t, "len()", record, function.ErrTooFewArgs, "len() with no args")
	testError(t, `len("foo", "bar")`, record, function.ErrTooManyArgs, "len() with too many args")
	testError(t, "len(5)", record, function.ErrBadArgument, "len() with non-container arg")

	record = `
#0:record[s:string,bs:bstring,bs2:bstring]
0:[🍺;\xf0\x9f\x8d\xba;\xba\x8d\x9f\xf0;]`

	testSuccessful(t, `len("foo")`, record, zint64(3))
	testSuccessful(t, `len(s)`, record, zint64(4))
	testSuccessful(t, `len(bs)`, record, zint64(4))
	testSuccessful(t, `len(bs2)`, record, zint64(4))

	testSuccessful(t, `rune_len("foo")`, record, zint64(3))
	testSuccessful(t, `rune_len(s)`, record, zint64(1))
	testSuccessful(t, `rune_len(bs)`, record, zint64(1))
	testSuccessful(t, `rune_len(bs2)`, record, zint64(4))
}

func TestTime(t *testing.T) {
	// These represent the same time (Tue, 26 May 2020 15:27:47.967 in GMT)
	iso := "2020-05-26T15:27:47.967Z"
	msec := 1590506867_967
	nsec := msec * 1_000_000
	zval := zng.Value{zng.TypeTime, zng.EncodeTime(nano.Ts(nsec))}

	exp := fmt.Sprintf(`iso("%s")`, iso)
	testSuccessful(t, exp, "", zval)
	exp = fmt.Sprintf("msec(%d):time", msec)
	testSuccessful(t, exp, "", zval)
	exp = fmt.Sprintf("msec(%d.0):time", msec)
	testSuccessful(t, exp, "", zval)
	exp = fmt.Sprintf("usec(%d):time", msec*1000)
	testSuccessful(t, exp, "", zval)
	exp = fmt.Sprintf("usec(%d.0):time", msec*1000)
	testSuccessful(t, exp, "", zval)
	exp = fmt.Sprintf("%d:time", nsec)
	testSuccessful(t, exp, "", zval)
	testSuccessful(t, "trunc(1590506867.967, 1)", "", zng.Value{zng.TypeTime, zng.EncodeTime(nano.Ts(1590506867 * 1_000_000_000))})

	testError(t, "iso()", "", function.ErrTooFewArgs, "Time.fromISO() with no args")
	testError(t, `iso("abc", "def")`, "", function.ErrTooManyArgs, "Time.fromISO() with too many args")
	testError(t, "iso(1234)", "", function.ErrBadArgument, "Time.fromISO() with wrong argument type")

	testError(t, "msec()", "", function.ErrTooFewArgs, "Time.fromMilliseconds() with no args")
	testError(t, "msec(123, 456)", "", function.ErrTooManyArgs, "Time.fromMilliseconds() with too many args")

	testError(t, "usec()", "", function.ErrTooFewArgs, "Time.fromMicroseconds() with no args")
	testError(t, "usec(123, 456)", "", function.ErrTooManyArgs, "Time.fromMicroseconds() with too many args")
}
