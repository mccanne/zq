package zed_test

import (
	"strings"
	"testing"
	"time"

	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordTypeCheck(t *testing.T) {
	r := zng.NewRecord(
		zng.NewTypeRecord(0, []zng.Column{
			zng.NewColumn("f", zng.NewTypeSet(0, zng.TypeString)),
		}),
		nil)
	t.Run("set/error/duplicate-element", func(t *testing.T) {
		var b zcode.Builder
		b.BeginContainer()
		b.AppendPrimitive([]byte("dup"))
		b.AppendPrimitive([]byte("dup"))
		// Don't normalize.
		b.EndContainer()
		r.Bytes = b.Bytes()
		assert.EqualError(t, r.TypeCheck(), "<set element> (|[string]|): duplicate element")
	})
	t.Run("set/error/unsorted-elements", func(t *testing.T) {
		var b zcode.Builder
		b.BeginContainer()
		b.AppendPrimitive([]byte("a"))
		b.AppendPrimitive([]byte("z"))
		b.AppendPrimitive([]byte("b"))
		// Don't normalize.
		b.EndContainer()
		r.Bytes = b.Bytes()
		assert.EqualError(t, r.TypeCheck(), "<set element> (|[string]|): elements not sorted")
	})
	t.Run("set/primitive-elements", func(t *testing.T) {
		var b zcode.Builder
		b.BeginContainer()
		b.AppendPrimitive([]byte("dup"))
		b.AppendPrimitive([]byte("dup"))
		b.AppendPrimitive([]byte("z"))
		b.AppendPrimitive([]byte("a"))
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		r.Bytes = b.Bytes()
		assert.NoError(t, r.TypeCheck())
	})
	t.Run("set/complex-elements", func(t *testing.T) {
		var b zcode.Builder
		b.BeginContainer()
		for _, s := range []string{"dup", "dup", "z", "a"} {
			b.BeginContainer()
			b.AppendPrimitive([]byte(s))
			b.EndContainer()
		}
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		r := zng.NewRecord(
			zng.NewTypeRecord(0, []zng.Column{
				zng.NewColumn("f", zng.NewTypeSet(0, zng.NewTypeRecord(0, []zng.Column{
					zng.NewColumn("g", zng.TypeString),
				}))),
			}),
			b.Bytes())
		assert.NoError(t, r.TypeCheck())
	})

}

func TestRecordAccessAlias(t *testing.T) {
	const input = `{foo:"hello" (=zfile),bar:true (=zbool)} (=0)`
	reader := zson.NewReader(strings.NewReader(input), zson.NewContext())
	rec, err := reader.Read()
	require.NoError(t, err)
	s, err := rec.AccessString("foo")
	require.NoError(t, err)
	assert.Equal(t, s, "hello")
	b, err := rec.AccessBool("bar")
	require.NoError(t, err)
	assert.Equal(t, b, true)
}

func TestRecordTs(t *testing.T) {
	cases := []struct {
		input    string
		expected nano.Ts
	}{
		{"{ts:1970-01-01T00:00:01Z}", nano.Ts(time.Second)},
		{"{notts:1970-01-01T00:00:01Z}", nano.MinTs}, // No ts field.
		{"{ts:null (time)}", nano.MinTs},             // Null ts field.
		{"{ts:1}", nano.MinTs},                       // Type of ts field is not TypeOfTime.
	}
	for _, c := range cases {
		zr := zson.NewReader(strings.NewReader(c.input), zson.NewContext())
		rec, err := zr.Read()
		assert.NoError(t, err)
		require.NotNil(t, rec)
		assert.Exactly(t, c.expected, rec.Ts(), "input: %q", c.input)
	}
}
