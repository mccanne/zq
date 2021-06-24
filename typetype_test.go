package zed_test

import (
	"testing"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/require"
)

func TestTypeValue(t *testing.T) {
	const s = "{A:{B:int64},C:int32}"
	zctx := zson.NewContext()
	typ, err := zson.ParseType(zson.NewContext(), s)
	require.NoError(t, err)
	tv := zctx.LookupTypeValue(typ)
	require.Exactly(t, s, zng.FormatTypeValue(tv.Bytes))
}

func TestTypeValueCrossContext(t *testing.T) {
	const s = "{A:{B:int64},C:int32}"
	typ, err := zson.ParseType(zson.NewContext(), s)
	require.NoError(t, err)
	tv := zson.NewContext().LookupTypeValue(typ)
	require.Exactly(t, s, zng.FormatTypeValue(tv.Bytes))
}
