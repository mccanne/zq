package expr_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zson"
	"github.com/brimsec/zq/ztest"
	"github.com/stretchr/testify/require"
)

func testSuccessful(t *testing.T, e string, record string, expect zng.Value) {
	if record == "" {
		record = "{}"
	}
	zctx := resolver.NewContext()
	typ, _ := zctx.LookupTypeRecord([]zng.Column{zng.Column{"result", expect.Type}})
	bytes := zcode.AppendPrimitive(nil, expect.Bytes)
	rec := zng.NewRecord(typ, bytes)
	formatter := zson.NewFormatter(0)
	val, err := formatter.Format(zng.Value{rec.Type, rec.Raw})
	require.NoError(t, err)
	zt := &ztest.ZTest{
		ZQL:    fmt.Sprintf("cut result = %s", e),
		Input:  []string{record},
		Output: val + "\n",
	}
	t.Run(e, func(t *testing.T) {
		t.Parallel()
		err := zt.RunInternal("")
		if err != nil {
			t.Fatal(err)
		}
	})
}

func testError(t *testing.T, e string, record string, expectErr error, description string) {
	if record == "" {
		record = "{}"
	}
	zt := &ztest.ZTest{
		ZQL:     fmt.Sprintf("cut result = %s", e),
		Input:   []string{record},
		Output:  "",
		ErrorRE: fmt.Sprintf(".*%s.*", expectErr),
	}
	t.Run(e, func(t *testing.T) {
		t.Parallel()
		err := zt.RunInternal("")
		if err != nil {
			t.Fatal(err)
		}
	})
}

func zbool(b bool) zng.Value {
	return zng.Value{zng.TypeBool, zng.EncodeBool(b)}
}

func zint32(v int32) zng.Value {
	return zng.Value{zng.TypeInt32, zng.EncodeInt(int64(v))}
}

func zint64(v int64) zng.Value {
	return zng.Value{zng.TypeInt64, zng.EncodeInt(v)}
}

func zuint64(v uint64) zng.Value {
	return zng.Value{zng.TypeUint64, zng.EncodeUint(v)}
}

func zfloat64(f float64) zng.Value {
	return zng.Value{zng.TypeFloat64, zng.EncodeFloat64(f)}
}

func zstring(s string) zng.Value {
	return zng.Value{zng.TypeString, zng.EncodeString(s)}
}

func zip(t *testing.T, s string) zng.Value {
	ip := net.ParseIP(s)
	require.NotNil(t, ip, "converted ip")
	return zng.Value{zng.TypeIP, zng.EncodeIP(ip)}
}

func TestPrimitives(t *testing.T) {
	record := `
#0:record[x:int32,f:float64,s:string]
0:[10;2.5;hello;]`

	// Test simple literals
	testSuccessful(t, "50", record, zint64(50))
	testSuccessful(t, "3.14", record, zfloat64(3.14))
	testSuccessful(t, `"boo"`, record, zstring("boo"))

	// Test good field references
	testSuccessful(t, "x", record, zint32(10))
	testSuccessful(t, "f", record, zfloat64(2.5))
	testSuccessful(t, "s", record, zstring("hello"))

	// Test bad field reference
	//	testError(t, "doesnexist", record, zng.ErrMissing, "referencing nonexistent field")
}

func TestLogical(t *testing.T) {
	record := `
#0:record[t:bool,f:bool]
0:[T;F;]`

	testSuccessful(t, "t AND t", record, zbool(true))
	testSuccessful(t, "t AND f", record, zbool(false))
	testSuccessful(t, "f AND t", record, zbool(false))
	testSuccessful(t, "f AND f", record, zbool(false))

	testSuccessful(t, "t OR t", record, zbool(true))
	testSuccessful(t, "t OR f", record, zbool(true))
	testSuccessful(t, "f OR t", record, zbool(true))
	testSuccessful(t, "f OR f", record, zbool(false))

	testSuccessful(t, "!t", record, zbool(false))
	testSuccessful(t, "!f", record, zbool(true))
	testSuccessful(t, "!!f", record, zbool(false))
}

func TestCompareNumbers(t *testing.T) {
	var numericTypes = []string{"uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "float64"}
	var intFields = []string{"u8", "i16", "u16", "i32", "u32", "i64", "u64"}

	for _, typ := range numericTypes {
		// Make a test point with this type in a field called x plus
		// one field of each other integer type
		record := fmt.Sprintf(`
#0:record[x:%s,u8:uint8,i16:int16,u16:uint16,i32:int32,u32:uint32,i64:int64,u64:uint16]
0:[1;0;0;0;0;0;0;0;]`, typ)

		// Test the 6 comparison operators against a constant
		testSuccessful(t, "x = 1", record, zbool(true))
		testSuccessful(t, "x = 0", record, zbool(false))
		testSuccessful(t, "x != 0", record, zbool(true))
		testSuccessful(t, "x != 1", record, zbool(false))
		testSuccessful(t, "x < 2", record, zbool(true))
		testSuccessful(t, "x < 1", record, zbool(false))
		testSuccessful(t, "x <= 2", record, zbool(true))
		testSuccessful(t, "x <= 1", record, zbool(true))
		testSuccessful(t, "x <= 0", record, zbool(false))
		testSuccessful(t, "x > 0", record, zbool(true))
		testSuccessful(t, "x > 1", record, zbool(false))
		testSuccessful(t, "x >= 0", record, zbool(true))
		testSuccessful(t, "x >= 1", record, zbool(true))
		testSuccessful(t, "x >= 2", record, zbool(false))

		// Test the full matrix of comparisons between all
		// the integer types
		for _, other := range intFields {
			exp := fmt.Sprintf("x = %s", other)
			testSuccessful(t, exp, record, zbool(false))

			exp = fmt.Sprintf("x != %s", other)
			testSuccessful(t, exp, record, zbool(true))

			exp = fmt.Sprintf("x < %s", other)
			testSuccessful(t, exp, record, zbool(false))

			exp = fmt.Sprintf("x <= %s", other)
			testSuccessful(t, exp, record, zbool(false))

			exp = fmt.Sprintf("x > %s", other)
			testSuccessful(t, exp, record, zbool(true))

			exp = fmt.Sprintf("x >= %s", other)
			testSuccessful(t, exp, record, zbool(true))
		}

		// For integer types, test this type against other
		// number-ish types: port, time, duration
		if typ != "float64" {
			record := fmt.Sprintf(`
#port=uint16
#0:record[x:%s,p:port,t:time,d:duration]
0:[1;80;1583794452;1000;]`, typ)

			// port
			testSuccessful(t, "x = p", record, zbool(false))
			testSuccessful(t, "p = x", record, zbool(false))
			testSuccessful(t, "x != p", record, zbool(true))
			testSuccessful(t, "p != x", record, zbool(true))
			testSuccessful(t, "x < p", record, zbool(true))
			testSuccessful(t, "p < x", record, zbool(false))
			testSuccessful(t, "x <= p", record, zbool(true))
			testSuccessful(t, "p <= x", record, zbool(false))
			testSuccessful(t, "x > p", record, zbool(false))
			testSuccessful(t, "p > x", record, zbool(true))
			testSuccessful(t, "x >= p", record, zbool(false))
			testSuccessful(t, "p >= x", record, zbool(true))

			// time
			testSuccessful(t, "x = t", record, zbool(false))
			testSuccessful(t, "t = x", record, zbool(false))
			testSuccessful(t, "x != t", record, zbool(true))
			testSuccessful(t, "t != x", record, zbool(true))
			testSuccessful(t, "x < t", record, zbool(true))
			testSuccessful(t, "t < x", record, zbool(false))
			testSuccessful(t, "x <= t", record, zbool(true))
			testSuccessful(t, "t <= x", record, zbool(false))
			testSuccessful(t, "x > t", record, zbool(false))
			testSuccessful(t, "t > x", record, zbool(true))
			testSuccessful(t, "x >= t", record, zbool(false))
			testSuccessful(t, "t >= x", record, zbool(true))

			// duration
			testSuccessful(t, "x = d", record, zbool(false))
			testSuccessful(t, "d = x", record, zbool(false))
			testSuccessful(t, "x != d", record, zbool(true))
			testSuccessful(t, "d != x", record, zbool(true))
			testSuccessful(t, "x < d", record, zbool(true))
			testSuccessful(t, "d < x", record, zbool(false))
			testSuccessful(t, "x <= d", record, zbool(true))
			testSuccessful(t, "d <= x", record, zbool(false))
			testSuccessful(t, "x > d", record, zbool(false))
			testSuccessful(t, "d > x", record, zbool(true))
			testSuccessful(t, "x >= d", record, zbool(false))
			testSuccessful(t, "d >= x", record, zbool(true))
		}

		// Test this type against non-numeric types
		record = fmt.Sprintf(`
#0:record[x:%s,s:string,bs:bstring,i:ip,n:net]
0:[1;hello;world;10.1.1.1;10.1.0.0/16;]`, typ)

		testError(t, "x = s", record, expr.ErrIncompatibleTypes, "comparing integer and string")
		testError(t, "x != s", record, expr.ErrIncompatibleTypes, "comparing integer and string")
		testError(t, "x < s", record, expr.ErrIncompatibleTypes, "comparing integer and string")
		testError(t, "x <= s", record, expr.ErrIncompatibleTypes, "comparing integer and string")
		testError(t, "x > s", record, expr.ErrIncompatibleTypes, "comparing integer and string")
		testError(t, "x >= s", record, expr.ErrIncompatibleTypes, "comparing integer and string")

		testError(t, "x = bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")
		testError(t, "x != bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")
		testError(t, "x < bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")
		testError(t, "x <= bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")
		testError(t, "x > bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")
		testError(t, "x >= bs", record, expr.ErrIncompatibleTypes, "comparing integer and bstring")

		testError(t, "x = i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")
		testError(t, "x != i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")
		testError(t, "x < i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")
		testError(t, "x <= i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")
		testError(t, "x > i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")
		testError(t, "x >= i", record, expr.ErrIncompatibleTypes, "comparing integer and ip")

		testError(t, "x = n", record, expr.ErrIncompatibleTypes, "comparing integer and net")
		testError(t, "x != n", record, expr.ErrIncompatibleTypes, "comparing integer and net")
		testError(t, "x < n", record, expr.ErrIncompatibleTypes, "comparing integer and net")
		testError(t, "x <= n", record, expr.ErrIncompatibleTypes, "comparing integer and net")
		testError(t, "x > n", record, expr.ErrIncompatibleTypes, "comparing integer and net")
		testError(t, "x >= n", record, expr.ErrIncompatibleTypes, "comparing integer and string")
	}

	// Test comparison between signed and unsigned and also
	// floats that cast to different integers.
	rec2 := `
#0:record[i:int64,u:uint64,f:float64]
0:[-1;18446744073709551615;-1.0;]`
	testSuccessful(t, "i = u", rec2, zbool(false))
	testSuccessful(t, "i != u", rec2, zbool(true))
	testSuccessful(t, "i < u", rec2, zbool(true))
	testSuccessful(t, "i <= u", rec2, zbool(true))
	testSuccessful(t, "i > u", rec2, zbool(false))
	testSuccessful(t, "i >= u", rec2, zbool(false))

	testSuccessful(t, "u = i", rec2, zbool(false))
	testSuccessful(t, "u != i", rec2, zbool(true))
	testSuccessful(t, "u < i", rec2, zbool(false))
	testSuccessful(t, "u <= i", rec2, zbool(false))
	testSuccessful(t, "u > i", rec2, zbool(true))
	testSuccessful(t, "u >= i", rec2, zbool(true))

	testSuccessful(t, "f = u", rec2, zbool(false))
	testSuccessful(t, "f != u", rec2, zbool(true))
	testSuccessful(t, "f < u", rec2, zbool(true))
	testSuccessful(t, "f <= u", rec2, zbool(true))
	testSuccessful(t, "f > u", rec2, zbool(false))
	testSuccessful(t, "f >= u", rec2, zbool(false))

	testSuccessful(t, "u = f", rec2, zbool(false))
	testSuccessful(t, "u != f", rec2, zbool(true))
	testSuccessful(t, "u < f", rec2, zbool(false))
	testSuccessful(t, "u <= f", rec2, zbool(false))
	testSuccessful(t, "u > f", rec2, zbool(true))
	testSuccessful(t, "u >= f", rec2, zbool(true))
}

func TestCompareNonNumbers(t *testing.T) {
	record := `
#port=uint16
#0:record[b:bool,s:string,bs:bstring,i:ip,p:port,net:net,t:time,d:duration]
0:[t;hello;world;10.1.1.1;443;10.1.0.0/16;1583794452;1000;]`

	// bool
	testSuccessful(t, "b = true", record, zbool(true))
	testSuccessful(t, "b = false", record, zbool(false))
	testSuccessful(t, "b != true", record, zbool(false))
	testSuccessful(t, "b != false", record, zbool(true))

	// string
	testSuccessful(t, `s = "hello"`, record, zbool(true))
	testSuccessful(t, `s != "hello"`, record, zbool(false))
	testSuccessful(t, `s = "world"`, record, zbool(false))
	testSuccessful(t, `s != "world"`, record, zbool(true))
	testSuccessful(t, `bs = "world"`, record, zbool(true))
	testSuccessful(t, `bs != "world"`, record, zbool(false))
	testSuccessful(t, `bs = "hello"`, record, zbool(false))
	testSuccessful(t, `bs != "hello"`, record, zbool(true))
	testSuccessful(t, "s = bs", record, zbool(false))
	testSuccessful(t, "s != bs", record, zbool(true))

	// ip
	testSuccessful(t, "i = 10.1.1.1", record, zbool(true))
	testSuccessful(t, "i != 10.1.1.1", record, zbool(false))
	testSuccessful(t, "i = 1.1.1.10", record, zbool(false))
	testSuccessful(t, "i != 1.1.1.10", record, zbool(true))
	testSuccessful(t, "i = i", record, zbool(true))

	// port
	testSuccessful(t, "p = 443", record, zbool(true))
	testSuccessful(t, "p != 443", record, zbool(false))

	// net
	testSuccessful(t, "net = 10.1.0.0/16", record, zbool(true))
	testSuccessful(t, "net != 10.1.0.0/16", record, zbool(false))
	testSuccessful(t, "net = 10.1.0.0/24", record, zbool(false))
	testSuccessful(t, "net != 10.1.0.0/24", record, zbool(true))

	// Test comparisons between incompatible types
	allTypes := []struct {
		field string
		typ   string
	}{
		{"b", "bool"},
		{"s", "string"},
		{"bs", "bstring"},
		{"i", "ip"},
		{"p", "port"},
		{"net", "net"},
	}

	allOperators := []string{"=", "!=", "<", "<=", ">", ">="}

	for _, t1 := range allTypes {
		for _, t2 := range allTypes {
			if t1 == t2 || (t1.typ == "string" && t2.typ == "bstring") || (t1.typ == "bstring" && t2.typ == "string") {
				continue
			}
			for _, op := range allOperators {
				exp := fmt.Sprintf("%s = %s", t1.field, t2.field)
				desc := fmt.Sprintf("compare %s %s %s", t1.typ, op, t2.typ)
				testError(t, exp, record, expr.ErrIncompatibleTypes, desc)
			}
		}
	}

	// relative comparisons on strings
	record = `
#0:record[s:string,bs:bstring]
0:[abc;def;]`

	testSuccessful(t, `s < "brim"`, record, zbool(true))
	testSuccessful(t, `s < "aaa"`, record, zbool(false))
	testSuccessful(t, `s < "abc"`, record, zbool(false))

	testSuccessful(t, `s > "brim"`, record, zbool(false))
	testSuccessful(t, `s > "aaa"`, record, zbool(true))
	testSuccessful(t, `s > "abc"`, record, zbool(false))

	testSuccessful(t, `s <= "brim"`, record, zbool(true))
	testSuccessful(t, `s <= "aaa"`, record, zbool(false))
	testSuccessful(t, `s <= "abc"`, record, zbool(true))

	testSuccessful(t, `s >= "brim"`, record, zbool(false))
	testSuccessful(t, `s >= "aaa"`, record, zbool(true))
	testSuccessful(t, `s >= "abc"`, record, zbool(true))

	testSuccessful(t, `bs < "security"`, record, zbool(true))
	testSuccessful(t, `bs < "aaa"`, record, zbool(false))
	testSuccessful(t, `bs < "def"`, record, zbool(false))

	testSuccessful(t, `bs > "security"`, record, zbool(false))
	testSuccessful(t, `bs > "aaa"`, record, zbool(true))
	testSuccessful(t, `bs > "def"`, record, zbool(false))

	testSuccessful(t, `bs <= "security"`, record, zbool(true))
	testSuccessful(t, `bs <= "aaa"`, record, zbool(false))
	testSuccessful(t, `bs <= "def"`, record, zbool(true))

	testSuccessful(t, `bs >= "security"`, record, zbool(false))
	testSuccessful(t, `bs >= "aaa"`, record, zbool(true))
	testSuccessful(t, `bs >= "def"`, record, zbool(true))
}

func TestPattern(t *testing.T) {
	testSuccessful(t, `"abc" = "abc"`, "", zbool(true))
	testSuccessful(t, `"abc" != "abc"`, "", zbool(false))
	testSuccessful(t, "10.1.1.1 in 10.0.0.0/8", "", zbool(true))
	testSuccessful(t, "10.1.1.1 in 192.168.0.0/16", "", zbool(false))
	testSuccessful(t, "!(10.1.1.1 in 10.0.0.0/8)", "", zbool(false))
	testSuccessful(t, "!(10.1.1.1 in 192.168.0.0/16)", "", zbool(true))
}

func TestIn(t *testing.T) {
	record := `
#0:record[a:array[int32],s:set[int32]]
0:[[1;2;3;][4;5;6;]]`

	testSuccessful(t, "1 in a", record, zbool(true))
	testSuccessful(t, "0 in a", record, zbool(false))

	testSuccessful(t, "1 in s", record, zbool(false))
	testSuccessful(t, "4 in s", record, zbool(true))

	testError(t, `"boo" in a`, record, expr.ErrIncompatibleTypes, "in operator with mismatched type")
	testError(t, `"boo" in s`, record, expr.ErrIncompatibleTypes, "in operator with mismatched type")
	testError(t, "1 in 2", record, expr.ErrNotContainer, "in operator with non-container")
}

func TestArithmetic(t *testing.T) {
	record := `
#0:record[x:int32,f:float64]
0:[10;2.5;]`

	// Test integer arithmetic
	testSuccessful(t, "100 + 23", record, zint64(123))
	testSuccessful(t, "x + 5", record, zint64(15))
	testSuccessful(t, "5 + x", record, zint64(15))
	testSuccessful(t, "x - 5", record, zint64(5))
	testSuccessful(t, "0 - x", record, zint64(-10))
	testSuccessful(t, "x + 5 - 3", record, zint64(12))
	testSuccessful(t, "x*2", record, zint64(20))
	testSuccessful(t, "5*x*2", record, zint64(100))
	testSuccessful(t, "x/3", record, zint64(3))
	testSuccessful(t, "20/x", record, zint64(2))

	// Test precedence of arithmetic operations
	testSuccessful(t, "x + 1 * 10", record, zint64(20))
	testSuccessful(t, "(x + 1) * 10", record, zint64(110))

	// Test arithmetic with floats
	testSuccessful(t, "f + 1.0", record, zfloat64(3.5))
	testSuccessful(t, "1.0 + f", record, zfloat64(3.5))
	testSuccessful(t, "f - 1.0", record, zfloat64(1.5))
	testSuccessful(t, "0.0 - f", record, zfloat64(-2.5))
	testSuccessful(t, "f * 1.5", record, zfloat64(3.75))
	testSuccessful(t, "1.5 * f", record, zfloat64(3.75))
	testSuccessful(t, "f / 1.25", record, zfloat64(2.0))
	testSuccessful(t, "5.0 / f", record, zfloat64(2.0))

	width := func(id int) int {
		switch id {
		case zng.IdInt8, zng.IdUint8:
			return 8
		case zng.IdInt16, zng.IdUint16:
			return 16
		case zng.IdInt32, zng.IdUint32:
			return 32
		case zng.IdInt64, zng.IdUint64:
			return 64
		}
		panic("width")
	}
	signed := func(width int) zng.Type {
		switch width {
		case 8:
			return zng.TypeInt8
		case 16:
			return zng.TypeInt16
		case 32:
			return zng.TypeInt32
		case 64:
			return zng.TypeInt64
		}
		panic("signed")
	}
	unsigned := func(width int) zng.Type {
		switch width {
		case 8:
			return zng.TypeUint8
		case 16:
			return zng.TypeUint16
		case 32:
			return zng.TypeUint32
		case 64:
			return zng.TypeUint64
		}
		panic("signed")
	}
	// Test arithmetic between integer types
	iresult := func(t1, t2 string, v uint64) zng.Value {
		typ1 := zng.LookupPrimitive(t1)
		typ2 := zng.LookupPrimitive(t2)
		id1 := typ1.ID()
		id2 := typ2.ID()
		sign1 := zng.IsSigned(id1)
		sign2 := zng.IsSigned(id2)
		sign := true
		if sign1 == sign2 {
			sign = sign1
		}
		w := width(id1)
		if w2 := width(id2); w2 > w {
			w = w2
		}
		if sign {
			return zng.Value{signed(w), zng.AppendInt(nil, int64(v))}
		}
		return zng.Value{unsigned(w), zng.AppendUint(nil, v)}
	}

	var intTypes = []string{"int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64"}
	for _, t1 := range intTypes {
		for _, t2 := range intTypes {
			record := fmt.Sprintf(`
#0:record[a:%s,b:%s]
0:[4;2;]`, t1, t2)
			testSuccessful(t, "a + b", record, iresult(t1, t2, 6))
			testSuccessful(t, "b + a", record, iresult(t1, t2, 6))
			testSuccessful(t, "a - b", record, iresult(t1, t2, 2))
			testSuccessful(t, "a * b", record, iresult(t1, t2, 8))
			testSuccessful(t, "b * a", record, iresult(t1, t2, 8))
			testSuccessful(t, "a / b", record, iresult(t1, t2, 2))
			testSuccessful(t, "b / a", record, iresult(t1, t2, 0))
		}

		// Test arithmetic mixing float + int
		record = fmt.Sprintf(`
#0:record[x:%s,f:float64]
0:[10;2.5;]`, t1)

		testSuccessful(t, "f + 5", record, zfloat64(7.5))
		testSuccessful(t, "5 + f", record, zfloat64(7.5))
		testSuccessful(t, "f + x", record, zfloat64(12.5))
		testSuccessful(t, "x + f", record, zfloat64(12.5))
		testSuccessful(t, "x - f", record, zfloat64(7.5))
		testSuccessful(t, "f - x", record, zfloat64(-7.5))
		testSuccessful(t, "x*f", record, zfloat64(25.0))
		testSuccessful(t, "f*x", record, zfloat64(25.0))
		testSuccessful(t, "x/f", record, zfloat64(4.0))
		testSuccessful(t, "f/x", record, zfloat64(0.25))
	}
	// Test string concatenation
	testSuccessful(t, `"hello" + " world"`, record, zstring("hello world"))

	// Test string arithmetic other than + fails
	testError(t, `"hello" - " world"`, record, expr.ErrIncompatibleTypes, "subtracting strings")
	testError(t, `"hello" * " world"`, record, expr.ErrIncompatibleTypes, "multiplying strings")
	testError(t, `"hello" / " world"`, record, expr.ErrIncompatibleTypes, "dividing strings")

	// Test that addition fails on an unsupported type
	testError(t, "10.1.1.1 + 1", record, expr.ErrIncompatibleTypes, "adding ip and integer")
	testError(t, "10.1.1.1 + 3.14159", record, expr.ErrIncompatibleTypes, "adding ip and float")
	testError(t, `10.1.1.1 + "foo"`, record, expr.ErrIncompatibleTypes, "adding ip and string")
}

func TestArrayIndex(t *testing.T) {
	record := `
#0:record[x:array[int64],i:uint16]
0:[[1;2;3;]1;]`

	testSuccessful(t, "x[0]", record, zint64(1))
	testSuccessful(t, "x[1]", record, zint64(2))
	testSuccessful(t, "x[2]", record, zint64(3))
	testSuccessful(t, "x[i]", record, zint64(2))
	testSuccessful(t, "i+1", record, zint64(2))
	testSuccessful(t, "x[i+1]", record, zint64(3))

	testError(t, "x[-1]", record, expr.ErrIndexOutOfBounds, "negative array index")
	testError(t, "x[3]", record, expr.ErrIndexOutOfBounds, "array index too large")
}

func TestFieldReference(t *testing.T) {
	record := `
#0:record[rec:record[i:int32,s:string,f:float64]]
0:[[5;boo;6.1;]]`

	testSuccessful(t, "rec.i", record, zint32(5))
	testSuccessful(t, "rec.s", record, zstring("boo"))
	testSuccessful(t, "rec.f", record, zfloat64(6.1))
}

func TestConditional(t *testing.T) {
	record := `
#0:record[x:int64]
0:[1;]`

	testSuccessful(t, `x = 0 ? "zero" : "not zero"`, record, zstring("not zero"))
	testSuccessful(t, `x = 1 ? "one" : "not one"`, record, zstring("one"))
	testError(t, `x ? "x" : "not x"`, record, expr.ErrIncompatibleTypes, "conditional with non-boolean condition")

	// Ensure that the unevaluated clause doesn't generate errors
	// (field y doesn't exist but it shouldn't be evaluated)
	testSuccessful(t, "x = 0 ? y : x", record, zint64(1))
	testSuccessful(t, "x != 0 ? x : y", record, zint64(1))
}

func a(t *testing.T) {
	// Test casts to byte
	testSuccessful(t, "10 :uint8", "", zng.Value{zng.TypeUint8, zng.EncodeUint(10)})
	testError(t, "-1 :uint8", "", expr.ErrBadCast, "out of range cast to uint8")
	testError(t, "300 :uint8", "", expr.ErrBadCast, "out of range cast to uint8")
	testError(t, `"foo" :uint8"`, "", expr.ErrBadCast, "cannot cast incompatible type to uint8")

	// Test casts to int16
	testSuccessful(t, "10 :int16", "", zng.Value{zng.TypeInt16, zng.EncodeInt(10)})
	testError(t, "-33000 :int16", "", expr.ErrBadCast, "out of range cast to int16")
	testError(t, "33000 :int16", "", expr.ErrBadCast, "out of range cast to int16")
	testError(t, `"foo" :int16"`, "", expr.ErrBadCast, "cannot cast incompatible type to int16")

	// Test casts to uint16
	testSuccessful(t, "10 :uint16", "", zng.Value{zng.TypeUint16, zng.EncodeUint(10)})
	testError(t, "-1 :uint16", "", expr.ErrBadCast, "out of range cast to uint16")
	testError(t, "66000 :uint16", "", expr.ErrBadCast, "out of range cast to uint16")
	testError(t, `"foo" :uint16"`, "", expr.ErrBadCast, "cannot cast incompatible type to uint16")

	// Test casts to int32
	testSuccessful(t, "10 :int32", "", zng.Value{zng.TypeInt32, zng.EncodeInt(10)})
	testError(t, "-2200000000 :int32", "", expr.ErrBadCast, "out of range cast to int32")
	testError(t, "2200000000 :int32", "", expr.ErrBadCast, "out of range cast to int32")
	testError(t, `"foo" :int32"`, "", expr.ErrBadCast, "cannot cast incompatible type to int32")

	// Test casts to uint32
	testSuccessful(t, "10 :uint32", "", zng.Value{zng.TypeUint32, zng.EncodeUint(10)})
	testError(t, "-1 :uint32", "", expr.ErrBadCast, "out of range cast to uint32")
	testError(t, "4300000000 :uint8", "", expr.ErrBadCast, "out of range cast to uint32")
	testError(t, `"foo" :uint32"`, "", expr.ErrBadCast, "cannot cast incompatible type to uint32")

	// Test casts to uint64
	testSuccessful(t, "10 :uint64", "", zuint64(10))
	testError(t, "-1 :uint64", "", expr.ErrBadCast, "out of range cast to uint64")
	testError(t, `"foo" :uint64"`, "", expr.ErrBadCast, "cannot cast incompatible type to uint64")

	// Test casts to float64
	testSuccessful(t, "10 :float64", "", zfloat64(10))
	testError(t, `"foo" :float64"`, "", expr.ErrBadCast, "cannot cast incompatible type to float64")

	// Test casts to ip
	testSuccessful(t, `"1.2.3.4" :ip`, "", zip(t, "1.2.3.4"))
	testError(t, "1234 :ip", "", expr.ErrBadCast, "cast of invalid ip address fails")
	testError(t, `"not an address" :ip`, "", expr.ErrBadCast, "cast of invalid ip address fails")

	// Test casts to time
	ts := zng.Value{zng.TypeTime, zng.EncodeTime(nano.Ts(1589126400_000_000_000))}
	testSuccessful(t, "1589126400.0 :time", "", ts)
	testSuccessful(t, "1589126400 :time", "", ts)
	testError(t, `"1234" :time`, "", expr.ErrBadCast, "cannot cast string to time")
}

func TestCasts(t *testing.T) {
	testSuccessful(t, "1.2:string", "", zstring("1.2"))
	testSuccessful(t, "5:string", "", zstring("5"))
	testSuccessful(t, "1.2.3.4:string", "", zstring("1.2.3.4"))
	testSuccessful(t, `"1":int64`, "", zint64(1))
	testSuccessful(t, `"-1":int64`, "", zint64(-1))
	testSuccessful(t, `"5.5":float64`, "", zfloat64(5.5))
	testSuccessful(t, `"1.2.3.4":ip`, "", zaddr("1.2.3.4"))

	testError(t, "1:ip", "", expr.ErrBadCast, "ip cast non-ip arg")
	testError(t, `"abc":int64`, "", expr.ErrBadCast, "int64 cast with non-parseable string")
	testError(t, `"abc":float64`, "", expr.ErrBadCast, "float64 cast with non-parseable string")
	testError(t, `"abc":ip`, "", expr.ErrBadCast, "ip cast with non-parseable string")
}
