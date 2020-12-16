package resolver_test

import (
	"bytes"
	"errors"
	"math"
	"net"
	"strings"
	"testing"

	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trim(s string) string {
	return strings.TrimSpace(s) + "\n"
}

func rectzng(t *testing.T, rec *zng.Record) string {
	var b strings.Builder
	w := tzngio.NewWriter(zio.NopCloser(&b))
	err := w.Write(rec)
	require.NoError(t, err)
	return b.String()
}

func tzngToRec(t *testing.T, zctx *resolver.Context, tzng string) *zng.Record {
	r := tzngio.NewReader(strings.NewReader(tzng), zctx)
	rec, err := r.Read()
	require.NoError(t, err)
	return rec
}

func boomerang(t *testing.T, in interface{}, out interface{}) {
	rec, err := resolver.MarshalRecord(resolver.NewContext(), in)
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := zngio.NewWriter(zio.NopCloser(&buf), zngio.WriterOpts{})
	err = zw.Write(rec)
	require.NoError(t, err)
	zctx := resolver.NewContext()
	zr := zngio.NewReader(&buf, zctx)
	rec, err = zr.Read()
	require.NoError(t, err)
	err = resolver.UnmarshalRecord(zctx, rec, out)
	require.NoError(t, err)
}

func TestMarshal(t *testing.T) {
	type S2 struct {
		Field2 string `zng:"f2"`
		Field3 int
	}
	type S1 struct {
		Field1  string
		Sub1    S2
		PField1 *bool
	}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, S1{
		Field1: "value1",
		Sub1: S2{
			Field2: "value2",
			Field3: -1,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[Field1:string,Sub1:record[f2:string,Field3:int64],PField1:bool]
0:[value1;[value2;-1;]-;]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))
}

type Thing struct {
	A string `zng:"a"`
	B int
}

type Things struct {
	Things []Thing
}

func TestMarshalSlice(t *testing.T) {
	s := []Thing{{"hello", 123}, {"world", 0}}
	r := Things{s}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, r)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[Things:array[record[a:string,B:int64]]]
0:[[[hello;123;][world;0;]]]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	empty := []Thing{}
	r2 := Things{empty}
	rec2, err := resolver.MarshalRecord(zctx, r2)
	require.NoError(t, err)
	require.NotNil(t, rec2)

	exp2 := `
#0:record[Things:array[record[a:string,B:int64]]]
0:[[]]
`
	assert.Equal(t, trim(exp2), rectzng(t, rec2))
}

func TestMarshalNilSlice(t *testing.T) {
	type TestNilSlice struct {
		Name  string
		Slice []string
	}
	t1 := TestNilSlice{Name: "test"}
	var t2 TestNilSlice
	boomerang(t, t1, &t2)
	assert.Equal(t, t1, t2)
}

func TestMarshalEmptySlice(t *testing.T) {
	type TestNilSlice struct {
		Name  string
		Slice []string
	}
	t1 := TestNilSlice{Name: "test", Slice: []string{}}
	var t2 TestNilSlice
	boomerang(t, t1, &t2)
	assert.Equal(t, t1, t2)
}

func TestMarshalTime(t *testing.T) {
	type TestTime struct {
		Ts nano.Ts
	}
	t1 := TestTime{Ts: nano.Now()}
	var t2 TestTime
	boomerang(t, t1, &t2)
	assert.Equal(t, t1, t2)
}

type TestIP struct {
	Addr net.IP
}

func TestIPType(t *testing.T) {
	addr := net.ParseIP("192.168.1.1").To4()
	require.NotNil(t, addr)
	s := TestIP{Addr: addr}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, s)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[Addr:ip]
0:[192.168.1.1;]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	var tip TestIP
	err = resolver.UnmarshalRecord(zctx, rec, &tip)
	require.NoError(t, err)
	require.Equal(t, s, tip)
}

func TestUnmarshalRecord(t *testing.T) {
	type T3 struct {
		T3f1 int32
		T3f2 float32
	}
	type T2 struct {
		T2f1 T3
		T2f2 string
	}
	type T1 struct {
		T1f1 *T2 `zng:"top"`
	}
	v1 := T1{
		T1f1: &T2{T2f1: T3{T3f1: 1, T3f2: 1.0}, T2f2: "t2f2-string1"},
	}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, v1)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[top:record[T2f1:record[T3f1:int32,T3f2:float64],T2f2:string]]
0:[[[1;1;]t2f2-string1;]]
`
	require.Equal(t, trim(exp), rectzng(t, rec))

	zctx = resolver.NewContext()
	rec = tzngToRec(t, zctx, exp)

	var v2 T1
	err = resolver.UnmarshalRecord(zctx, rec, &v2)
	require.NoError(t, err)
	require.Equal(t, v1, v2)

	type T4 struct {
		T4f1 *T2 `zng:"top"`
	}
	var v3 *T4
	err = resolver.UnmarshalRecord(zctx, rec, &v3)
	require.NoError(t, err)
	require.NotNil(t, v3)
	require.NotNil(t, v3.T4f1)
	require.Equal(t, *v1.T1f1, *v3.T4f1)
}

func TestUnmarshalSlice(t *testing.T) {
	type T1 struct {
		T1f1 []bool
	}
	v1 := T1{
		T1f1: []bool{true, false, true},
	}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, v1)
	require.NoError(t, err)
	require.NotNil(t, rec)

	var v2 T1
	err = resolver.UnmarshalRecord(zctx, rec, &v2)
	require.NoError(t, err)
	require.Equal(t, v1, v2)

	type T2 struct {
		Field1 []*int
	}
	intp := func(x int) *int { return &x }
	v3 := T2{
		Field1: []*int{intp(1), intp(2)},
	}
	zctx = resolver.NewContext()
	rec, err = resolver.MarshalRecord(zctx, v3)
	require.NoError(t, err)
	require.NotNil(t, rec)

	var v4 T2
	err = resolver.UnmarshalRecord(zctx, rec, &v4)
	require.NoError(t, err)
	require.Equal(t, v1, v2)
}

type testMarshaler string

func (m testMarshaler) MarshalZNG(zctx *resolver.Context, b *zcode.Builder) (zng.Type, error) {
	return resolver.Marshal(zctx, b, "marshal-"+string(m))
}

func (m *testMarshaler) UnmarshalZNG(zctx *resolver.Context, zt zng.Type, zb zcode.Bytes) error {
	var s string
	if err := resolver.Unmarshal(zctx, zt, zb, &s); err != nil {
		return err
	}
	ss := strings.Split(s, "-")
	if len(ss) != 2 && ss[0] != "marshal" {
		return errors.New("bad value")
	}
	*m = testMarshaler(ss[1])
	return nil
}

func TestMarshalInterface(t *testing.T) {
	type rectype struct {
		M1 *testMarshaler
		M2 testMarshaler
	}
	m1 := testMarshaler("m1")
	r1 := rectype{M1: &m1, M2: testMarshaler("m2")}
	rec, err := resolver.MarshalRecord(resolver.NewContext(), r1)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[M1:string,M2:string]
0:[marshal-m1;marshal-m2;]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	var r2 rectype
	err = resolver.UnmarshalRecord(resolver.NewContext(), rec, &r2)
	require.NoError(t, err)
	assert.Equal(t, "m1", string(*r1.M1))
	assert.Equal(t, "m2", string(r1.M2))
}

func TestMarshalArray(t *testing.T) {
	type rectype struct {
		A1 [2]int8
		A2 *[2]string
		A3 [][2]byte
	}
	a2 := &[2]string{"foo", "bar"}
	r1 := rectype{A1: [2]int8{1, 2}, A2: a2} // A3 left as nil
	rec, err := resolver.MarshalRecord(resolver.NewContext(), r1)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[A1:array[int8],A2:array[string],A3:array[array[uint8]]]
0:[[1;2;][foo;bar;]-;]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	var r2 rectype
	err = resolver.UnmarshalRecord(resolver.NewContext(), rec, &r2)
	require.NoError(t, err)
	assert.Equal(t, r1.A1, r2.A1)
	assert.Equal(t, *r2.A2, *r2.A2)
	assert.Len(t, r2.A3, 0)
}

func TestIntsAndUints(t *testing.T) {
	type rectype struct {
		I    int
		I8   int8
		I16  int16
		I32  int32
		I64  int64
		U    uint
		UI8  uint8
		UI16 uint16
		UI32 uint32
		UI64 uint64
	}
	r1 := rectype{
		I:    math.MinInt64,
		I8:   math.MinInt8,
		I16:  math.MinInt16,
		I32:  math.MinInt32,
		I64:  math.MinInt64,
		U:    math.MaxUint64,
		UI8:  math.MaxUint8,
		UI16: math.MaxUint16,
		UI32: math.MaxUint32,
		UI64: math.MaxUint64,
	}
	rec, err := resolver.MarshalRecord(resolver.NewContext(), r1)
	require.NoError(t, err)
	require.NotNil(t, rec)

	exp := `
#0:record[I:int64,I8:int8,I16:int16,I32:int32,I64:int64,U:uint64,UI8:uint8,UI16:uint16,UI32:uint32,UI64:uint64]
0:[-9223372036854775808;-128;-32768;-2147483648;-9223372036854775808;18446744073709551615;255;65535;4294967295;18446744073709551615;]
`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	var r2 rectype
	err = resolver.UnmarshalRecord(resolver.NewContext(), rec, &r2)
	require.NoError(t, err)
	assert.Equal(t, r1, r2)
}

func TestCustomRecord(t *testing.T) {
	vals := []interface{}{
		Thing{"hello", 123},
		99,
	}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalCustomRecord(zctx, []string{"foo", "bar"}, vals)
	require.NoError(t, err)
	exp := `
#0:record[foo:record[a:string,B:int64],bar:int64]
0:[[hello;123;]99;]`
	assert.Equal(t, trim(exp), rectzng(t, rec))

	vals = []interface{}{
		Thing{"hello", 123},
		nil,
	}
	exp = `
#0:record[foo:record[a:string,B:int64],bar:null]
0:[[hello;123;]-;]`
	rec, err = resolver.MarshalCustomRecord(zctx, []string{"foo", "bar"}, vals)
	require.NoError(t, err)
	assert.Equal(t, trim(exp), rectzng(t, rec))
}

type ThingTwo struct {
	C string `zng:"c"`
}

type ThingaMaBob interface {
	Who() string
}

func (t *Thing) Who() string    { return t.A }
func (t *ThingTwo) Who() string { return t.C }

func Make(which int) ThingaMaBob {
	if which == 1 {
		return &Thing{A: "It's a thing one"}
	}
	if which == 2 {
		return &ThingTwo{"It'sa  thing two"}
	}
	return nil
}

type Rolls []int

func TestInterfaceMarshal(t *testing.T) {
	t1 := Make(2)
	zctx := resolver.NewContext()
	zv, err := resolver.MarshalValue(zctx, t1)
	require.NoError(t, err)
	assert.Equal(t, "resolver_test.ThingTwo=({c:string})", zv.Type.ZSON())

	rolls := Rolls{1, 2, 3}
	zv, err = resolver.MarshalValue(zctx, rolls)
	require.NoError(t, err)
	assert.Equal(t, "resolver_test.Rolls=([int64])", zv.Type.ZSON())

	plain := []int32{1, 2, 3}
	zv, err = resolver.MarshalValue(zctx, plain)
	require.NoError(t, err)
	assert.Equal(t, "[int32]", zv.Type.ZSON())
}
