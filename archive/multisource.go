package archive

import (
	"context"
	"io"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/filter"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/scanner"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng/resolver"
)

type multiCloser []io.Closer

func (c multiCloser) Close() error {
	var err error
	for _, closer := range c {
		if closeErr := closer.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

type scannerCloser struct {
	scanner.Scanner
	io.Closer
}

func newSpanScanner(ctx context.Context, ark *Archive, zctx *resolver.Context, f filter.Filter, filterExpr ast.BooleanExpr, si SpanInfo) (sc *scannerCloser, err error) {
	if len(si.Chunks) == 1 {
		rc, err := iosrc.NewReader(ctx, si.Chunks[0].Path(ark))
		if err != nil {
			return nil, err
		}
		sn, err := scanner.NewScanner(ctx, zngio.NewReader(rc, zctx), f, filterExpr, si.Span)
		if err != nil {
			rc.Close()
			return nil, err
		}
		return &scannerCloser{sn, rc}, nil
	}
	closers := make(multiCloser, 0, len(si.Chunks))
	readers := make([]zbuf.Reader, 0, len(si.Chunks))
	for _, chunk := range si.Chunks {
		rc, err := iosrc.NewReader(ctx, chunk.Path(ark))
		if err != nil {
			closers.Close()
			return nil, err
		}
		closers = append(closers, rc)
		readers = append(readers, zngio.NewReader(rc, zctx))
	}
	sn, err := scanner.NewCombiner(ctx, readers, zbuf.RecordCompare(ark.DataSortDirection), f, filterExpr, si.Span)
	if err != nil {
		closers.Close()
		return nil, err
	}
	return &scannerCloser{
		Scanner: sn,
		Closer:  closers,
	}, nil
}

type multiSource struct {
	ark      *Archive
	altPaths []string
}

// NewMultiSource returns a driver.MultiSource for an Archive. If no alternative
// paths are specified, the MultiSource will send a source for each span in the
// driver.SourceFilter span, and report the same ordering as the archive.
//
// Otherwise, the sources come from localizing the given alternative paths to
// each chunk in the archive, recognizing "_" as the chunk file itself, with no
// defined ordering.
func NewMultiSource(ark *Archive, altPaths []string) driver.MultiSource {
	if len(altPaths) == 1 && altPaths[0] == "_" {
		altPaths = nil
	}
	return &multiSource{
		ark:      ark,
		altPaths: altPaths,
	}
}

func (m *multiSource) OrderInfo() (field.Static, bool) {
	if len(m.altPaths) == 0 {
		return field.New("ts"), m.ark.DataSortDirection == zbuf.DirTimeReverse
	}
	return nil, false
}

func (m *multiSource) spanWalk(ctx context.Context, zctx *resolver.Context, sf driver.SourceFilter, srcChan chan<- driver.SourceOpener) error {
	return SpanWalk(ctx, m.ark, sf.Span, func(si SpanInfo) error {
		so := func() (driver.ScannerCloser, error) {
			return newSpanScanner(ctx, m.ark, zctx, sf.Filter, sf.FilterExpr, si)
		}
		select {
		case srcChan <- so:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func (m *multiSource) chunkWalk(ctx context.Context, zctx *resolver.Context, sf driver.SourceFilter, srcChan chan<- driver.SourceOpener) error {
	return Walk(ctx, m.ark, func(chunk Chunk) error {
		so := func() (driver.ScannerCloser, error) {
			paths := make([]string, len(m.altPaths))
			for i, input := range m.altPaths {
				paths[i] = chunk.Localize(m.ark, input).String()
			}
			rc := detector.MultiFileReader(zctx, paths, zio.ReaderOpts{Format: "zng"})
			sn, err := scanner.NewScanner(ctx, rc, sf.Filter, sf.FilterExpr, sf.Span)
			if err != nil {
				return nil, err
			}
			return &scannerCloser{Scanner: sn, Closer: rc}, nil
		}
		select {
		case srcChan <- so:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func (m *multiSource) SendSources(ctx context.Context, zctx *resolver.Context, sf driver.SourceFilter, srcChan chan driver.SourceOpener) error {
	if len(m.altPaths) == 0 {
		return m.spanWalk(ctx, zctx, sf, srcChan)
	}
	return m.chunkWalk(ctx, zctx, sf, srcChan)
}
