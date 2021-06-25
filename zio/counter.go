package zio

import (
	"sync/atomic"

	"github.com/brimdata/zed"
)

// Counter wraps a zbuf.Reader and provides a method to return the number
// of records read from the stream.
type Counter struct {
	Reader
	counter *int64
}

// NewCounter provides a wrapper for Reader that tracks the number of
// records read in the variable pointed to by p.  Atomic operations
// are carrried out on the count so the caller should use package atomic
// to read the referenced count while there is potential concurrency.
func NewCounter(reader Reader, p *int64) *Counter {
	return &Counter{Reader: reader, counter: p}
}

func (c *Counter) Read() (*zed.Record, error) {
	rec, err := c.Reader.Read()
	if rec != nil {
		atomic.AddInt64(c.counter, 1)
	}
	return rec, err
}
