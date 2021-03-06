package service

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/brimdata/zed/api"
	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/driver"
	"github.com/brimdata/zed/lake"
	"github.com/brimdata/zed/pkg/storage"
	"github.com/brimdata/zed/zbuf"
	"github.com/brimdata/zed/zio"
	"github.com/brimdata/zed/zio/anyio"
	"github.com/brimdata/zed/zio/zngio"
	"github.com/brimdata/zed/zqe"
	"github.com/brimdata/zed/zson"
	"github.com/segmentio/ksuid"
)

type LogOp struct {
	bytesTotal   int64
	commit       ksuid.KSUID
	err          error
	readers      []zio.Reader
	readCounters []*readCounter
	warnings     []string
	warningCh    chan string
	zctx         *zson.Context
}

// Logs ingests the provided list of files into the provided pool.
func NewLogOp(ctx context.Context, pool *lake.Pool, req api.LogPostRequest) (*LogOp, error) {
	p := &LogOp{
		warningCh: make(chan string, 5),
		warnings:  make([]string, 0, 5),
		zctx:      zson.NewContext(),
	}
	opts := anyio.ReaderOpts{Zng: zngio.ReaderOpts{Validate: true}}
	proc, err := ast.UnpackJSONAsProc(req.Shaper)
	if err != nil {
		return nil, err
	}
	for _, path := range req.Paths {
		rc, size, err := openIncomingLog(ctx, path)
		if err != nil {
			p.closeFiles()
			return nil, err
		}
		sf, err := anyio.OpenFromNamedReadCloser(p.zctx, rc, path, opts)
		if err != nil {
			rc.Close()
			if req.StopErr {
				p.closeFiles()
				return nil, zqe.ErrInvalid(err)
			}
			p.openWarning(path, err)
			continue
		}
		zr := zio.NewWarningReader(sf, p)

		p.bytesTotal += size
		p.readCounters = append(p.readCounters, rc)

		if proc != nil {
			zr, err = driver.NewReader(ctx, proc, p.zctx, zr)
			if err != nil {
				return nil, err
			}
		}
		p.readers = append(p.readers, zr)
	}
	// this is the only goroutine that calls p.Warn()
	go p.start(ctx, pool)
	return p, nil
}

func (p *LogOp) Warn(msg string) error {
	// warnings received before we've started our goroutine are
	// saved here and will be drained in start()
	if p.warnings != nil {
		p.warnings = append(p.warnings, msg)
		return nil
	}
	p.warningCh <- msg
	return nil
}

func (p *LogOp) openWarning(path string, err error) {
	p.Warn(fmt.Sprintf("%s: %s", path, err))
}

type readCounter struct {
	readCloser io.ReadCloser
	nread      int64
}

func (rc *readCounter) Read(p []byte) (int, error) {
	n, err := rc.readCloser.Read(p)
	atomic.AddInt64(&rc.nread, int64(n))
	return n, err
}

func (rc *readCounter) bytesRead() int64 {
	return atomic.LoadInt64(&rc.nread)
}

func (rc *readCounter) Close() error {
	return rc.readCloser.Close()
}

func openIncomingLog(ctx context.Context, path string) (*readCounter, int64, error) {
	//XXX We will deprecate reading ingesting a file on the file system
	// from an API call.  For now, we create a storage.NewLocalEngine() to
	// be able to (incorrectly) read a file this way in the service endpoint.
	uri, err := storage.ParseURI(path)
	if err != nil {
		return nil, 0, err
	}
	engine := storage.NewLocalEngine()
	rc, err := engine.Get(ctx, uri)
	if err != nil {
		return nil, 0, err
	}
	size, err := storage.Size(rc)
	if err != nil {
		return nil, 0, err
	}
	return &readCounter{readCloser: rc}, size, nil
}

func (p *LogOp) closeFiles() error {
	var retErr error
	for _, rc := range p.readCounters {
		if err := rc.Close(); err != nil {
			retErr = err
		}
	}
	return retErr
}

func (p *LogOp) bytesRead() int64 {
	var read int64
	for _, rc := range p.readCounters {
		read += rc.bytesRead()
	}
	return read
}

func (p *LogOp) start(ctx context.Context, pool *lake.Pool) {
	// first drain warnings
	for _, warning := range p.warnings {
		p.warningCh <- warning
	}
	p.warnings = nil

	defer zio.CloseReaders(p.readers)
	reader, _ := zbuf.MergeReadersByTsAsReader(ctx, p.readers, pool.Layout.Order)
	p.commit, p.err = pool.Add(ctx, reader)
	if err := p.closeFiles(); err != nil && p.err != nil {
		p.err = err
	}
	close(p.warningCh)
}

func (p *LogOp) Stats() api.LogPostStatus {
	return api.LogPostStatus{
		Type:         "LogPostStatus",
		LogTotalSize: p.bytesTotal,
		LogReadSize:  p.bytesRead(),
	}
}

func (p *LogOp) Status() <-chan string {
	return p.warningCh
}

func (p *LogOp) Commit() ksuid.KSUID {
	return p.commit
}

// Error indicates what if any error occurred during import, after the
// Status channel is closed.  The result is undefined while Status is open.
func (p *LogOp) Error() error {
	return p.err
}
