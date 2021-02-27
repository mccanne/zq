package lake

import (
	"context"
	"errors"
	"io"
	"path"
	"path/filepath"
	"sync/atomic"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/lake/chunk"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zqe"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
)

type MultiSource interface {
	driver.MultiSource
	Stats() ChunkStats
}

type multiCloser []io.Closer

func (c multiCloser) Close() (err error) {
	for _, closer := range c {
		if closeErr := closer.Close(); closeErr != nil {
			err = multierr.Append(err, closeErr)
		}
	}
	return
}

type pullerCloser struct {
	zbuf.Puller
	zbuf.MultiStats
	io.Closer
}

func newSpanScanner(ctx context.Context, lk *Lake, zctx *resolver.Context, sf driver.SourceFilter, si SpanInfo) (sc *pullerCloser, stats ChunkStats, err error) {
	closers := make(multiCloser, 0, len(si.Chunks))
	pullers := make([]zbuf.Puller, 0, len(si.Chunks))
	scanners := make([]zbuf.Scanner, 0, len(si.Chunks))
	for _, chk := range si.Chunks {
		rc, err := chunk.NewReader(ctx, chk, si.Span)
		if err != nil {
			closers.Close()
			return nil, stats, err
		}
		stats.ChunksOpenedBytes += rc.TotalSize
		stats.ChunksReadBytes += rc.ReadSize
		closers = append(closers, rc)
		reader := zngio.NewReader(rc, zctx)
		scanner, err := reader.NewScanner(ctx, sf.Runtime, si.Span)
		if err != nil {
			closers.Close()
			return nil, stats, err
		}
		scanners = append(scanners, scanner)
		pullers = append(pullers, scanner)
	}
	return &pullerCloser{
		Puller:     zbuf.MergeByTs(ctx, pullers, lk.DataOrder),
		MultiStats: scanners,
		Closer:     closers,
	}, stats, nil
}

// NewMultiSource returns a driver.MultiSource for an Lake. If no alternative
// paths are specified, the MultiSource will send a source for each span in the
// driver.SourceFilter span, and report the same ordering as the archive.
//
// Otherwise, the sources come from localizing the given alternative paths to
// each chunk in the archive, recognizing "_" as the chunk file itself, with no
// defined ordering.
func NewMultiSource(lk *Lake, altPaths []string) MultiSource {
	if len(altPaths) == 1 && altPaths[0] == "_" {
		altPaths = nil
	}
	if len(altPaths) != 0 {
		return &chunkMultiSource{
			lk:       lk,
			altPaths: altPaths,
			stats:    &ChunkStats{},
		}
	}
	return &spanMultiSource{lk, &ChunkStats{}}
}

type spanMultiSource struct {
	lk    *Lake
	stats *ChunkStats
}

func (m *spanMultiSource) OrderInfo() (field.Static, bool) {
	return field.New("ts"), m.lk.DataOrder == zbuf.OrderDesc
}

func (m *spanMultiSource) SendSources(ctx context.Context, span nano.Span, srcChan chan driver.Source) error {
	// We keep a channel of []SpanInfos filled to reduce the time
	// query workers are waiting for the next driver.Source.
	const tsDirPreFetch = 10
	sinfosChan := make(chan []SpanInfo, tsDirPreFetch)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		err := tsDirVisit(ctx, m.lk, span, func(_ tsDir, chunks []chunk.Chunk) error {
			sinfos := mergeChunksToSpans(chunks, m.lk.DataOrder, span)
			select {
			case sinfosChan <- sinfos:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		close(sinfosChan)
		return err
	})
	g.Go(func() error {
		for sinfos := range sinfosChan {
			for _, si := range sinfos {
				select {
				case srcChan <- &spanSource{lk: m.lk, spanInfo: si, stats: m.stats}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	})
	return g.Wait()
}

func (m *spanMultiSource) SourceFromRequest(ctx context.Context, req *api.WorkerChunkRequest) (driver.Source, error) {
	si := SpanInfo{Span: req.Span}
	for _, p := range req.ChunkPaths {
		dir, file := path.Split(filepath.ToSlash(p))
		_, id, ok := chunk.FileMatch(file)
		if !ok {
			return nil, zqe.E(zqe.Invalid, "invalid chunk path: %v", p)
		}
		tsdir, ok := parseTsDirName(path.Base(dir))
		if !ok {
			return nil, zqe.E(zqe.Invalid, "invalid chunk path: %v", p)
		}
		uri := tsdir.path(m.lk)
		mdPath := chunk.MetadataPath(uri, id)

		b, err := m.lk.immfiles.ReadFile(ctx, mdPath)
		if err != nil {
			return nil, err
		}

		md, err := chunk.UnmarshalMetadata(b, m.lk.DataOrder)
		if err != nil {
			return nil, zqe.E("failed to read chunk metadata from %s: %w", mdPath.String(), err)
		}
		si.Chunks = append(si.Chunks, md.Chunk(uri, id))
	}
	return &spanSource{
		lk:       m.lk,
		spanInfo: si,
		stats:    m.stats,
	}, nil
}

func (m *spanMultiSource) Stats() ChunkStats {
	return m.stats.Copy()
}

type spanSource struct {
	lk       *Lake
	spanInfo SpanInfo
	stats    *ChunkStats
}

func (s *spanSource) Open(ctx context.Context, zctx *resolver.Context, sf driver.SourceFilter) (driver.ScannerCloser, error) {
	scn, stats, err := newSpanScanner(ctx, s.lk, zctx, sf, s.spanInfo)
	s.stats.Accumulate(stats)
	return scn, err
}

func (s *spanSource) ToRequest(req *api.WorkerChunkRequest) error {
	req.Span = s.spanInfo.Span
	req.DataPath = s.lk.DataPath.String()
	for _, c := range s.spanInfo.Chunks {
		req.ChunkPaths = append(req.ChunkPaths, s.lk.Root.RelPath(c.Path()))
	}
	return nil
}

// A chunkMultiSource uses the lake.Walk call to provide a driver.Source
// for each chunk in the archive, possibly combining its data with files named
// by altPaths located in the chunk's zar directory.
type chunkMultiSource struct {
	lk       *Lake
	altPaths []string
	stats    *ChunkStats
}

func (cms *chunkMultiSource) OrderInfo() (field.Static, bool) {
	return nil, false
}

func (cms *chunkMultiSource) SendSources(ctx context.Context, span nano.Span, srcChan chan driver.Source) error {
	return Walk(ctx, cms.lk, func(chunk chunk.Chunk) error {
		select {
		case srcChan <- &chunkSource{cms: cms, chunk: chunk, stats: cms.stats}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

var errReqForChunk = errors.New("no request support for chunk sources")

func (cms *chunkMultiSource) SourceFromRequest(context.Context, *api.WorkerChunkRequest) (driver.Source, error) {
	return nil, errReqForChunk
}

func (m *chunkMultiSource) Stats() ChunkStats {
	return m.stats.Copy()
}

type scannerCloser struct {
	zbuf.Scanner
	io.Closer
}

type chunkSource struct {
	cms   *chunkMultiSource
	chunk chunk.Chunk
	stats *ChunkStats
}

func (s *chunkSource) Open(ctx context.Context, zctx *resolver.Context, sf driver.SourceFilter) (driver.ScannerCloser, error) {
	var size int64
	paths := make([]string, len(s.cms.altPaths))
	for i, input := range s.cms.altPaths {
		u := s.chunk.Localize(input)
		stat, err := iosrc.Stat(ctx, u)
		if err != nil {
			return nil, err
		}
		size += stat.Size()
		paths[i] = u.String()
	}
	rc := detector.MultiFileReader(zctx, paths, zio.ReaderOpts{Format: "zng"})
	sn, err := zbuf.NewScanner(ctx, rc, sf.Runtime, sf.Span)
	if err != nil {
		rc.Close()
		return nil, err
	}
	s.stats.Accumulate(ChunkStats{size, size})
	return &scannerCloser{Scanner: sn, Closer: rc}, nil
}

func (s *chunkSource) ToRequest(*api.WorkerChunkRequest) error {
	return errReqForChunk
}

type ChunkStats struct {
	// ChunksOpenedBytes is the cumulative size of all the chunks read.
	ChunksOpenedBytes int64
	// ChunksReadBytes is the amount of bytes read from all chunks. If seek
	// indicies are used this number should be less than OpenedChunkSize.
	ChunksReadBytes int64
}

func (s *ChunkStats) Accumulate(a ChunkStats) {
	atomic.AddInt64(&s.ChunksOpenedBytes, a.ChunksOpenedBytes)
	atomic.AddInt64(&s.ChunksReadBytes, a.ChunksReadBytes)
}

func (s *ChunkStats) Copy() ChunkStats {
	return ChunkStats{
		ChunksOpenedBytes: atomic.LoadInt64(&s.ChunksOpenedBytes),
		ChunksReadBytes:   atomic.LoadInt64(&s.ChunksReadBytes),
	}
}
