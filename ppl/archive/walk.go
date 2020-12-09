package archive

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/archive/chunk"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zqe"
	"github.com/segmentio/ksuid"
)

const (
	dataDirname = "zd"
	zarExt      = ".zar"
)

// A tsDir is a directory found in the "<DataPath>/zd" directory of the archive,
// and holds all of the data & associated files for a span of time, currently
// fixed to a single day.
type tsDir struct {
	nano.Span
}

func newTsDir(ts nano.Ts) tsDir {
	return tsDir{nano.Span{Ts: ts.Midnight(), Dur: nano.Day}}
}

func parseTsDirName(name string) (tsDir, bool) {
	t, err := time.Parse("20060102", name)
	if err != nil {
		return tsDir{}, false
	}
	return newTsDir(nano.TimeToTs(t)), true
}

func (t tsDir) path(ark *Archive) iosrc.URI {
	return ark.DataPath.AppendPath(dataDirname, t.name())
}

func (t tsDir) name() string {
	return t.Ts.Time().Format("20060102")
}

type tsDirVisitor func(tsd tsDir, unsortedChunks []chunk.Chunk) error

// tsDirVisit calls visitor for each tsDir whose span overlaps with the
// given span. tsDirs are visited in the archive's order, but the
// chunks passed to visitor are not sorted.
func tsDirVisit(ctx context.Context, ark *Archive, filterSpan nano.Span, visitor tsDirVisitor) error {
	zdDir := ark.DataPath.AppendPath(dataDirname)
	dirents, err := iosrc.ReadDir(ctx, zdDir)
	if err != nil {
		return err
	}
	var tsdirs []tsDir
	for _, e := range dirents {
		if !e.IsDir() {
			continue
		}
		tsd, ok := parseTsDirName(e.Name())
		if !ok || !tsd.Overlaps(filterSpan) {
			continue
		}
		tsdirs = append(tsdirs, tsd)
	}
	sort.Slice(tsdirs, func(i, j int) bool {
		if ark.DataOrder == zbuf.OrderAsc {
			return tsdirs[i].Ts < tsdirs[j].Ts
		}
		return tsdirs[j].Ts < tsdirs[i].Ts
	})
	for _, d := range tsdirs {
		dirents, err := iosrc.ReadDir(ctx, zdDir.AppendPath(d.name()))
		if err != nil {
			return err
		}
		chunks, err := tsDirEntriesToChunks(ctx, ark, filterSpan, d, dirents)
		if err != nil {
			return err
		}
		if err := visitor(d, chunks); err != nil {
			return err
		}
	}
	return nil
}

func tsDirEntriesToChunks(ctx context.Context, ark *Archive, filterSpan nano.Span, tsDir tsDir, entries []iosrc.Info) ([]chunk.Chunk, error) {
	type seen struct {
		data bool
		meta bool
	}
	m := make(map[ksuid.KSUID]seen)
	for _, e := range entries {
		if kind, id, ok := chunk.FileMatch(e.Name()); ok {
			if !ark.filterAllowed(id) {
				continue
			}
			s := m[id]
			switch kind {
			case chunk.FileKindData:
				s.data = true
			case chunk.FileKindMetadata:
				s.meta = true
			}
			m[id] = s
		}
	}
	var chunks []chunk.Chunk
	for id, seen := range m {
		if !seen.meta || !seen.data {
			continue
		}
		dir := tsDir.path(ark)
		md, err := chunk.ReadMetadata(ctx, chunk.MetadataPath(dir, id), ark.DataOrder)
		if err != nil {
			if zqe.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		chunk := md.Chunk(dir, id)
		if !filterSpan.Overlaps(chunk.Span()) {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

type Visitor func(chunk chunk.Chunk) error

// Walk calls visitor for every data chunk in the archive.
func Walk(ctx context.Context, ark *Archive, v Visitor) error {
	return tsDirVisit(ctx, ark, nano.MaxSpan, func(_ tsDir, chunks []chunk.Chunk) error {
		chunk.Sort(ark.DataOrder, chunks)
		for _, c := range chunks {
			if err := iosrc.MkdirAll(c.ZarDir(), 0700); err != nil {
				return err
			}
			if err := v(c); err != nil {
				return err
			}
		}
		return nil
	})
}

// RmDirs descends a directory hierarchy looking for zar dirs and remove
// each such directory and all of its contents.
func RmDirs(ctx context.Context, ark *Archive) error {
	return Walk(ctx, ark, func(chunk chunk.Chunk) error {
		return ark.dataSrc.RemoveAll(ctx, chunk.ZarDir())
	})
}

// A SpanInfo is a logical view of the records within a time span, stored
// in one or more Chunks.
type SpanInfo struct {
	Span nano.Span

	// Chunks are the data files that contain records within this SpanInfo.
	// The Chunks may have spans that extend beyond this SpanInfo, so any
	// records from these Chunks should be limited to those that fall within
	// this SpanInfo's Span.
	Chunks []chunk.Chunk
}

func (s SpanInfo) ChunkRange(order zbuf.Order, chunkIdx int) string {
	first, last := spanToFirstLast(order, s.Span)
	c := s.Chunks[chunkIdx]
	return fmt.Sprintf("[%d-%d,%d-%d]", first, last, c.First, c.Last)
}

func (si *SpanInfo) RemoveMasked() []chunk.Chunk {
	if len(si.Chunks) == 1 {
		return nil
	}
	var maskIds []ksuid.KSUID
	for _, c := range si.Chunks {
		for _, mid := range c.Masks {
			maskIds = append(maskIds, mid)
		}
	}
	if len(maskIds) == 0 {
		return nil
	}
	var chunks, removed []chunk.Chunk
	for _, c := range si.Chunks {
		var masked bool
		for _, mid := range maskIds {
			if mid == c.Id {
				masked = true
				removed = append(removed, c)
				break
			}
		}
		if !masked {
			chunks = append(chunks, c)
		}
	}
	// No need to sort chunks since we perform the mask removal in-order.
	si.Chunks = chunks
	return removed
}

// SpanWalk calls visitor with each SpanInfo within the filter span.
func SpanWalk(ctx context.Context, ark *Archive, filter nano.Span, visitor func(si SpanInfo) error) error {
	return tsDirVisit(ctx, ark, filter, func(_ tsDir, chunks []chunk.Chunk) error {
		sinfos := mergeChunksToSpans(chunks, ark.DataOrder, filter)
		for _, s := range sinfos {
			for _, c := range s.Chunks {
				if err := iosrc.MkdirAll(c.ZarDir(), 0700); err != nil {
					return err
				}
			}
			if err := visitor(s); err != nil {
				return err
			}
		}
		return nil
	})
}
