package zio

import (
	"context"
	"sync"

	"github.com/brimdata/zed"
)

// A Combiner is a Reader that returns records by reading from multiple Readers.
type Combiner struct {
	cancel  context.CancelFunc
	ctx     context.Context
	done    []bool
	once    sync.Once
	readers []Reader
	results chan combinerResult
}

func NewCombiner(ctx context.Context, readers []Reader) *Combiner {
	ctx, cancel := context.WithCancel(ctx)
	return &Combiner{
		cancel:  cancel,
		ctx:     ctx,
		done:    make([]bool, len(readers)),
		readers: readers,
		results: make(chan combinerResult),
	}
}

type combinerResult struct {
	err error
	idx int
	rec *zed.Record
}

func (c *Combiner) run() {
	for i := range c.readers {
		idx := i
		go func() {
			for {
				rec, err := c.readers[idx].Read()
				select {
				case c.results <- combinerResult{err, idx, rec}:
					if rec == nil || err != nil {
						return
					}
				case <-c.ctx.Done():
					return
				}
			}
		}()
	}
}

func (c *Combiner) finished() bool {
	for i := range c.done {
		if !c.done[i] {
			return false
		}
	}
	return true
}

func (c *Combiner) Read() (*zed.Record, error) {
	c.once.Do(c.run)
	for {
		select {
		case r := <-c.results:
			if r.err != nil {
				c.cancel()
				return nil, r.err
			}
			if r.rec != nil {
				return r.rec, nil
			}
			c.done[r.idx] = true
			if c.finished() {
				c.cancel()
				return nil, nil
			}
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		}
	}
}
