package stringsearch

import (
	"testing"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/require"
)

func TestFieldNameIter(t *testing.T) {
	const typeString = "{r1:{r2:{s:string,r3:{t:time}},a:[int64],r4:{i:ip}},empty:{}}"
	typ, err := zson.ParseType(zson.NewContext(), typeString)
	require.NoError(t, err)
	var f FieldNameIter
	f.Init(typ.(*zng.TypeRecord))
	require.False(t, f.Done())
	require.Exactly(t, "r1.r2.s", string(f.Next()))
	require.False(t, f.Done())
	require.Exactly(t, "r1.r2.r3.t", string(f.Next()))
	require.False(t, f.Done())
	require.Exactly(t, "r1.a", string(f.Next()))
	require.False(t, f.Done())
	require.Exactly(t, "r1.r4.i", string(f.Next()))
	require.False(t, f.Done())
	require.Exactly(t, "empty", string(f.Next()))
	require.True(t, f.Done())
}

func TestFieldNameIterEmptyTopLevelRecord(t *testing.T) {
	typ, err := zson.ParseType(zson.NewContext(), "{}")
	require.NoError(t, err)
	var f FieldNameIter
	f.Init(typ.(*zng.TypeRecord))
	require.True(t, f.Done())
}
