package zed_test

import (
	"strings"
	"testing"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"

	"github.com/stretchr/testify/require"
)

func TestRecordIter(t *testing.T) {
	// Test a few edge cases: record with another record as the first
	// field, record with another record as the last field, non-record
	// container types inside records...
	const input = `{r1:{r2:{s:"hello"},a:[1,2,3],r3:{i:1.2.3.4}}}`
	rec, err := zson.NewReader(strings.NewReader(input), zson.NewContext()).Read()
	require.NoError(t, err)

	it := rec.FieldIter()
	require.False(t, it.Done(), "iterator is not exhausted")

	name, val, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, "r1.r2.s", strings.Join(name, "."), "got correct field name")
	require.Equal(t, zng.TypeString, val.Type, "got correct type for first field")
	require.Equal(t, "hello", string(val.Bytes), "got correct value for first field")
	require.False(t, it.Done(), "iterator is not exhausted")

	name, val, err = it.Next()
	require.NoError(t, err)
	require.Equal(t, "r1.a", strings.Join(name, "."), "got correct field name")
	l, err := val.ContainerLength()
	require.NoError(t, err)
	require.Equal(t, 3, l, "got array of length 3")
	require.False(t, it.Done(), "iterator is not exhausted")

	name, val, err = it.Next()
	require.NoError(t, err)
	require.Equal(t, "r1.r3.i", strings.Join(name, "."), "got correct field name")
	require.Equal(t, zng.TypeIP, val.Type, "got correct type for last field")
	require.True(t, it.Done(), "iterator is exhausted after last field")
}
