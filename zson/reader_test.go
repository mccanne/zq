package zson_test

import (
	"io"
	"testing"
	"time"

	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadOneLineNoEOF(t *testing.T) {
	const expected = `{msg:"record1"}`
	type result struct {
		err error
		rec *zng.Record
	}
	done := make(chan result)
	go func() {
		var reader slowStream = make(chan []byte, 1)
		// The test needs two records because with a single record the parser
		// will stall waiting to see if the record has a decorator.
		reader <- []byte(expected + "\n" + expected)
		r := zson.NewReader(reader, zson.NewContext())
		rec, err := r.Read()
		done <- result{rec: rec, err: err}
	}()
	select {
	// Because this test CAN deadlock fail after 5 seconds.
	case <-time.After(time.Second * 5):
		t.Fatal("testing did not complete in 5 seconds")
	case res := <-done:
		require.NoError(t, res.err)
		rec := res.rec
		assert.Equal(t, expected, rec.Type.Format(rec.Bytes))
	}
}

type slowStream chan []byte

func (s slowStream) Read(b []byte) (n int, err error) {
	in, ok := <-s
	if !ok {
		err = io.EOF
	}
	return copy(b, in), err
}
