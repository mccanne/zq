package index

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/microindex"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
)

func NewWriter(ctx context.Context, u iosrc.URI, def *Definition) (*Writer, error) {
	rwCh := make(rwChan)
	indexer, err := newIndexer(ctx, u, def, rwCh)
	if err != nil {
		return nil, err
	}
	w := &Writer{
		URI:        u,
		Definition: def,

		done:    make(chan struct{}),
		indexer: indexer,
		rwCh:    rwCh,
	}
	return w, nil
}

type Writer struct {
	Definition *Definition
	URI        iosrc.URI

	done    chan struct{}
	indexer *indexer
	once    sync.Once
	rwCh    rwChan
}

type rwChan chan *zng.Record

func (c rwChan) Read() (*zng.Record, error) {
	return <-c, nil
}

func (w *Writer) Write(rec *zng.Record) error {
	select {
	case <-w.done:
		if err := w.indexer.err.Load(); err != nil {
			return err
		}
		return errors.New("index writer closed")
	default:
		w.once.Do(w.indexer.start)
		w.rwCh <- rec
		return nil
	}
}

func (w *Writer) Close() error {
	// If once has not be called, this means a write has never been called.
	// Abort microindex so no file is written.
	w.once.Do(func() {
		w.indexer.microindex.Abort()
	})
	close(w.done)
	close(w.rwCh)
	return w.indexer.Wait()
}

func (w *Writer) Abort() {
	w.Close()
	w.indexer.microindex.Abort()
}

// onceError is an object that will only store an error once.
type onceError struct {
	sync.Mutex // guards following
	err        error
}

func (a *onceError) Store(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}
func (a *onceError) Load() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}

type indexer struct {
	err        onceError
	cutter     *expr.Cutter
	fgr        zbuf.ReadCloser
	keyType    zng.Type
	microindex *microindex.Writer
	wg         sync.WaitGroup
}

func newIndexer(ctx context.Context, u iosrc.URI, def *Definition, r zbuf.Reader) (*indexer, error) {
	zctx := resolver.NewContext()
	conf := driver.Config{Custom: compile}
	program := &ast.Program{Entry: def.Proc}
	fgr, err := driver.NewReaderWithConfig(ctx, conf, program, zctx, r)
	if err != nil {
		return nil, err
	}
	keys := def.Keys
	if len(keys) == 0 {
		keys = []field.Static{keyName}
	}
	opts := []microindex.Option{microindex.KeyFields(keys...)}
	if def.Framesize > 0 {
		opts = append(opts, microindex.FrameThresh(def.Framesize))
	}
	writer, err := microindex.NewWriterWithContext(ctx, zctx, u.String(), opts...)
	if err != nil {
		return nil, err
	}
	fields, resolvers := compiler.CompileAssignments(keys, keys)
	cutter, err := expr.NewCutter(zctx, fields, resolvers)
	if err != nil {
		return nil, err
	}
	d := &indexer{
		fgr:        fgr,
		cutter:     cutter,
		microindex: writer,
	}
	return d, nil
}

func (d *indexer) start() {
	d.wg.Add(1)
	go func() {
		if err := zbuf.Copy(d, d.fgr); err != nil {
			d.microindex.Abort()
			d.err.Store(err)
		}
		d.err.Store(d.microindex.Close())
		d.wg.Done()
	}()
}

func (d *indexer) Wait() error {
	d.wg.Wait()
	return d.err.Load()
}

func (d *indexer) Write(rec *zng.Record) error {
	key, err := d.cutter.Apply(rec)
	if err != nil {
		return fmt.Errorf("checking index record: %w", err)
	}
	if d.keyType == nil {
		d.keyType = key.Type
	}
	if key.Type.ID() != d.keyType.ID() {
		return fmt.Errorf("key type changed from %s to %s", d.keyType, key.Type)
	}
	return d.microindex.Write(rec)
}
