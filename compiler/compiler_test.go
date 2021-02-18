package compiler_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/test"
	"github.com/brimsec/zq/proc"
	"github.com/brimsec/zq/proc/proctest"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compile(pctx *proc.Context, inputs []proc.Interface, z string) (*compiler.Program, error) {
	query, err := compiler.ParseProgram(z)
	if err != nil {
		return nil, err
	}
	program := compiler.NewProgram(query, pctx)
	err = program.Compile(nil, inputs)
	if err != nil {
		return nil, err
	}
	return program, err
}

func TestCompileParents(t *testing.T) {
	input := `
#0:record[ts:time]
0:[1;]
`
	exp := `
#0:record[ts:time]
0:[1;]
0:[1;]
`
	zctx := resolver.NewContext()
	pctx := &proc.Context{Context: context.Background(), TypeContext: zctx}
	var sources []proc.Interface
	for i := 0; i < 2; i++ {
		r := tzngio.NewReader(bytes.NewReader([]byte(input)), zctx)
		sources = append(sources, &proctest.RecordPuller{R: r})
	}
	t.Run("read two sources", func(t *testing.T) {
		program, err := compile(pctx, sources, "split (=>filter * =>filter *) | filter *")
		require.NoError(t, err)
		var sb strings.Builder
		err = zbuf.CopyPuller(tzngio.NewWriter(zio.NopCloser(&sb)), program.Outputs()[0])
		require.NoError(t, err)
		assert.Equal(t, test.Trim(exp), sb.String())
	})

	t.Run("too few parents", func(t *testing.T) {
		query, err := compiler.ParseProgram("split (=>filter * =>filter * =>filter *) | filter *")
		require.NoError(t, err)

		query.Entry.(*ast.SequentialProc).Procs = query.Entry.(*ast.SequentialProc).Procs[1:]

		program := compiler.NewProgram(query, pctx)
		err = program.Compile(nil, sources)
		require.Error(t, err)
	})

	t.Run("too many parents", func(t *testing.T) {
		query, err := compiler.ParseProgram("* | split(=>filter * =>filter *) | filter *")
		require.NoError(t, err)
		program := compiler.NewProgram(query, pctx)
		err = program.Compile(nil, sources)
		require.Error(t, err)
	})
}

// TestCompileMergeDone exercises the bug reported in issue #1635.
// When we refactor the AST to make merge/combine explicit, this test will
// need to be updated and can be moved to ztest using the "merge" zql operator
// that will be available then.
func TestCompileMergeDone(t *testing.T) {
	input := `
#0:record[k:int32]
0:[1;]
0:[2;]
0:[3;]
0:[4;]
`
	zctx := resolver.NewContext()
	pctx := &proc.Context{Context: context.Background(), TypeContext: zctx}
	r := tzngio.NewReader(bytes.NewReader([]byte(input)), zctx)
	src := &proctest.RecordPuller{R: r}
	query, err := compiler.ParseProgram("split(=>filter * =>head 1) | head 3")
	require.NoError(t, err)

	seq, ok := query.Entry.(*ast.SequentialProc)
	require.Equal(t, ok, true)
	p, ok := seq.Procs[0].(*ast.ParallelProc)
	require.Equal(t, ok, true)

	// Force the parallel proc to create a merge proc instead of combine.
	p.MergeOrderField = field.New("k")
	program := compiler.NewProgram(query, pctx)
	err = program.Compile(nil, []proc.Interface{src})
	require.NoError(t, err)

	var sb strings.Builder
	err = zbuf.CopyPuller(tzngio.NewWriter(zio.NopCloser(&sb)), program.Outputs()[0])
	require.NoError(t, err)
}
