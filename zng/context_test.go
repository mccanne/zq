package zng_test

import (
	"testing"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicates(t *testing.T) {
	ctx := zson.NewContext()
	setType := ctx.LookupTypeSet(zng.TypeInt32)
	typ1, err := ctx.LookupTypeRecord([]zng.Column{
		zng.NewColumn("a", zng.TypeString),
		zng.NewColumn("b", setType),
	})
	require.NoError(t, err)
	typ2, err := zson.ParseType(ctx, "{a:string,b:|[int32]|}")
	require.NoError(t, err)
	assert.EqualValues(t, typ1.ID(), typ2.ID())
	assert.EqualValues(t, setType.ID(), typ2.(*zng.TypeRecord).Columns[1].Type.ID())
	typ3, err := ctx.LookupByValue(zng.EncodeTypeValue(setType))
	require.NoError(t, err)
	assert.Equal(t, setType.ID(), typ3.ID())
}

func TestTranslateAlias(t *testing.T) {
	c1 := zson.NewContext()
	c2 := zson.NewContext()
	set1, err := zson.ParseType(c1, "|[int64]|")
	require.NoError(t, err)
	set2, err := zson.ParseType(c2, "|[int64]|")
	require.NoError(t, err)
	alias1, err := c1.LookupTypeAlias("foo", set1)
	require.NoError(t, err)
	alias2, err := c2.LookupTypeAlias("foo", set2)
	require.NoError(t, err)
	alias3, err := c2.TranslateType(alias1)
	require.NoError(t, err)
	assert.Equal(t, alias2, alias3)
}

func TestCopyMutateColumns(t *testing.T) {
	c := zson.NewContext()
	cols := []zng.Column{{"foo", zng.TypeString}, {"bar", zng.TypeInt64}}
	typ, err := c.LookupTypeRecord(cols)
	require.NoError(t, err)
	cols[0].Type = nil
	require.NotNil(t, typ.Columns[0].Type)
}
