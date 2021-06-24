package zed_test

import (
	"bytes"
	"testing"

	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSet(t *testing.T) {
	t.Run("duplicate-element", func(t *testing.T) {
		b := zcode.NewBuilder()
		b.BeginContainer()
		b.AppendPrimitive([]byte("dup"))
		b.AppendPrimitive([]byte("dup"))
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		set := zcode.AppendPrimitive(nil, []byte("dup"))
		expected := zcode.AppendContainer(nil, set)
		require.Exactly(t, expected, b.Bytes())
	})
	t.Run("unsorted-elements", func(t *testing.T) {
		b := zcode.NewBuilder()
		b.BeginContainer()
		b.AppendPrimitive([]byte("z"))
		b.AppendPrimitive([]byte("a"))
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		set := zcode.AppendPrimitive(nil, []byte("a"))
		set = zcode.AppendPrimitive(set, []byte("z"))
		expected := zcode.AppendContainer(nil, set)
		require.Exactly(t, expected, b.Bytes())
	})
	t.Run("unsorted-and-duplicate-elements", func(t *testing.T) {
		b := zcode.NewBuilder()
		big := bytes.Repeat([]byte("x"), 256)
		small := []byte("small")
		b.AppendPrimitive(big)
		b.BeginContainer()
		// Append duplicate elements in reverse of set-normal order.
		for i := 0; i < 3; i++ {
			b.AppendContainer(big)
			b.AppendPrimitive(big)
			b.AppendContainer(small)
			b.AppendPrimitive(small)
			b.AppendContainer(nil)
			b.AppendPrimitive(nil)
		}
		b.TransformContainer(zng.NormalizeSet)
		b.EndContainer()
		set := zcode.AppendPrimitive(nil, nil)
		set = zcode.AppendContainer(set, small)
		set = zcode.AppendPrimitive(set, small)
		set = zcode.AppendContainer(set, big)
		set = zcode.AppendPrimitive(set, big)
		expected := zcode.AppendPrimitive(nil, big)
		expected = zcode.AppendContainer(expected, set)
		require.Exactly(t, expected, b.Bytes())
	})
}
