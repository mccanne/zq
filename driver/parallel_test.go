package driver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scannerCloser struct {
	zbuf.Scanner
	io.Closer
}

type onClose struct {
	fn func() error
}

func (c *onClose) Close() error {
	if c.fn == nil {
		return nil
	}
	return c.fn()
}

type testSource struct {
	opener func(*resolver.Context, SourceFilter) (ScannerCloser, error)
}

func (s *testSource) Open(ctx context.Context, zctx *resolver.Context, sf SourceFilter) (ScannerCloser, error) {
	return s.opener(zctx, sf)
}

func (s *testSource) ToRequest(*api.WorkerChunkRequest) error {
	return errors.New("ToRequest called on testSource")
}

var parallelTestInputs []string = []string{
	`
#0:record[v:int32,ts:time]
0:[0;0;]`,
	`
#0:record[v:int32,ts:time]
0:[1;1;]`,
	`
#0:record[v:int32,ts:time]
0:[2;2;]`,
	`
#0:record[v:int32,ts:time]
0:[3;3;]`,
	`
#0:record[v:int32,ts:time]
0:[4;4;]`,
}

type orderedmsrc struct{}

func (m *orderedmsrc) OrderInfo() (field.Static, bool) {
	return field.New("ts"), false
}

func (m *orderedmsrc) SendSources(ctx context.Context, span nano.Span, srcChan chan Source) error {
	// Create SourceOpeners that await a signal before returning, then
	// signal them in reverse of expected order.
	var releaseChs []chan struct{}
	for _ = range parallelTestInputs {
		releaseChs = append(releaseChs, make(chan struct{}))
	}
	for i := range parallelTestInputs {
		i := i
		opener := func(zctx *resolver.Context, sf SourceFilter) (ScannerCloser, error) {
			rdr := tzngio.NewReader(strings.NewReader(parallelTestInputs[i]), zctx)
			sn, err := zbuf.NewScanner(ctx, rdr, sf.Filter, sf.Span)
			if err != nil {
				return nil, err
			}
			select {
			case <-releaseChs[i]:
			}
			return &scannerCloser{
				Scanner: sn,
				Closer:  &onClose{},
			}, nil
		}
		srcChan <- &testSource{opener: opener}
	}
	for i := len(parallelTestInputs) - 1; i >= 0; i-- {
		close(releaseChs[i])
	}
	return nil
}

func (m *orderedmsrc) SourceFromRequest(context.Context, *api.WorkerChunkRequest) (Source, error) {
	return nil, errors.New("SourceFromRequest called on orderedmsrc")
}

func trim(s string) string {
	return strings.TrimSpace(s) + "\n"
}

func TestParallelOrder(t *testing.T) {
	t.Parallel()

	// Use `v!=3` to trigger & verify empty rank handling in orderedWaiter.
	query, err := compiler.ParseProc("v!=3")
	require.NoError(t, err)
	program := &ast.Program{Entry: query}

	var buf bytes.Buffer
	d := NewCLI(tzngio.NewWriter(zio.NopCloser(&buf)))
	zctx := resolver.NewContext()
	err = MultiRun(context.Background(), d, program, zctx, &orderedmsrc{}, MultiConfig{
		Parallelism: len(parallelTestInputs),
	})
	require.NoError(t, err)

	exp := `
#0:record[v:int32,ts:time]
0:[0;0;]
0:[1;1;]
0:[2;2;]
0:[4;4;]
`
	assert.Equal(t, trim(exp), buf.String())
}

// A noEndScanner never returns proc.EOS from its Pull().
type noEndScanner struct {
	input string
	zctx  *resolver.Context
}

func (rp *noEndScanner) Pull() (zbuf.Batch, error) {
	r := tzngio.NewReader(strings.NewReader(rp.input), rp.zctx)
	return zbuf.NewPuller(r, 1).Pull()
}

func (rp *noEndScanner) Stats() *zbuf.ScannerStats {
	return &zbuf.ScannerStats{}
}

type scannerCloseMS struct {
	closed chan struct{}
	input  string
}

func (m *scannerCloseMS) OrderInfo() (field.Static, bool) {
	return nil, false
}

func (m *scannerCloseMS) SendSources(ctx context.Context, span nano.Span, srcChan chan Source) error {
	opener := func(zctx *resolver.Context, _ SourceFilter) (ScannerCloser, error) {
		return &scannerCloser{
			// Use a noEndScanner so that a parallel head never tries to
			// close the ScannerCloser in its Pull. That way, if the Close fires,
			// we know it must have happened due to the query context cancellation.
			Scanner: &noEndScanner{input: m.input, zctx: zctx},
			Closer: &onClose{fn: func() error {
				close(m.closed)
				return nil
			}},
		}, nil
	}
	srcChan <- &testSource{opener: opener}
	return nil
}

func (m *scannerCloseMS) SourceFromRequest(context.Context, *api.WorkerChunkRequest) (Source, error) {
	return nil, errors.New("SourceFromRequest called on scannerCloseMS")
}

// TestScannerClose verifies that any open ScannerCloser's will be closed soon
// after the MultiRun call finishes.
func TestScannerClose(t *testing.T) {
	query, err := compiler.ParseProc("* | head 1")
	require.NoError(t, err)
	program := &ast.Program{Entry: query}

	var buf bytes.Buffer
	d := NewCLI(tzngio.NewWriter(zio.NopCloser(&buf)))
	zctx := resolver.NewContext()
	ms := &scannerCloseMS{
		input: `
#0:record[v:int32,ts:time]
0:[1;1;]
`,
		closed: make(chan struct{}),
	}
	err = MultiRun(context.Background(), d, program, zctx, ms, MultiConfig{})
	require.NoError(t, err)
	require.Equal(t, trim(ms.input), trim(buf.String()))

	tm := time.NewTimer(5 * time.Second)
	select {
	case <-ms.closed:
	case <-tm.C:
		t.Fatal("time out waiting for close")
	}
}
