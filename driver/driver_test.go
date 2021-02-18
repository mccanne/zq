package driver

import (
	"context"
	"strings"
	"testing"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/assert"
)

type counter struct {
	n int
}

func (c *counter) Write(*zng.Record) error {
	c.n++
	return nil
}

func TestMuxDriver(t *testing.T) {
	input := `
#0:record[_path:string,ts:time]
0:[conn;1425565514.419939;]`

	query, err := compiler.ParseProc("split (=>tail 1 =>tail 1)")
	assert.NoError(t, err)
	program := &ast.Program{Entry: query}

	t.Run("muxed into one writer", func(t *testing.T) {
		zctx := resolver.NewContext()
		reader := tzngio.NewReader(strings.NewReader(input), zctx)
		assert.NoError(t, err)
		c := counter{}
		d := NewCLI(&c)
		err = Run(context.Background(), d, program, zctx, reader, Config{})
		assert.NoError(t, err)
		assert.Equal(t, 2, c.n)
	})

	t.Run("muxed into individual writers", func(t *testing.T) {
		zctx := resolver.NewContext()
		reader := tzngio.NewReader(strings.NewReader(input), zctx)
		assert.NoError(t, err)
		cs := []zbuf.Writer{&counter{}, &counter{}}
		d := NewCLI(cs...)
		err = Run(context.Background(), d, program, zctx, reader, Config{})
		assert.NoError(t, err)
		assert.Equal(t, 1, cs[0].(*counter).n)
		assert.Equal(t, 1, cs[1].(*counter).n)
	})
}
