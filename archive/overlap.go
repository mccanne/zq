package archive

import (
	"context"
	"runtime"
	"sort"

	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/segmentio/ksuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// mergeChunksToSpans takes an unordered set of Chunks with possibly overlapping
// spans, and returns an ordered list of spanInfos, whose spans will be bounded
// by filter, and where each SpanInfo contains one or more Chunks whose data
// falls into the SpanInfo's span.
func mergeChunksToSpans(chunks []Chunk, order zbuf.Order, filter nano.Span) []SpanInfo {
	sinfos := alignChunksToSpans(chunks, order, filter)
	removeMaskedChunks(sinfos, false)
	return mergeLargestChunkSpanInfos(sinfos, order)
}

// alignChunksToSpans creates an ordered slice of SpanInfo's whose boundaries
// match either the boundaries of a chunk or, in the case of overlapping chunks,
// the boundaries of each portion of an overlap.
func alignChunksToSpans(chunks []Chunk, order zbuf.Order, filter nano.Span) []SpanInfo {
	var siChunks []Chunk // accumulating chunks for next SpanInfo
	var siFirst nano.Ts  // first timestamp for next SpanInfo
	var result []SpanInfo
	boundaries(chunks, order, func(ts nano.Ts, firstChunks, lastChunks []Chunk) {
		if len(firstChunks) > 0 {
			// ts is the 'First' timestamp for these chunks.
			if len(siChunks) > 0 && ts != siFirst {
				// We have accumulated chunks; create a span with them whose
				// last timestamp was just before ts.
				siSpan := firstLastToSpan(siFirst, prevTs(ts, order))
				if filter.Overlaps(siSpan) {
					chunks := copyChunks(siChunks, nil)
					chunksSort(order, chunks)
					result = append(result, SpanInfo{
						Span:   filter.Intersect(siSpan),
						Chunks: chunks,
					})
				}
			}
			// Accumulate these chunks whose first timestamp is ts.
			siChunks = append(siChunks, firstChunks...)
			siFirst = ts
		}
		if len(lastChunks) > 0 {
			// ts is the 'Last' timestamp for these chunks.
			siSpan := firstLastToSpan(siFirst, ts)
			if filter.Overlaps(siSpan) {
				chunks := copyChunks(siChunks, nil)
				chunksSort(order, chunks)
				result = append(result, SpanInfo{
					Span:   filter.Intersect(siSpan),
					Chunks: chunks,
				})
			}
			// Drop the chunks that ended from our accumulation.
			siChunks = copyChunks(siChunks, lastChunks)
			siFirst = nextTs(ts, order)
		}
	})
	return result
}

func copyChunks(src []Chunk, skip []Chunk) (dst []Chunk) {
outer:
	for i := range src {
		for j := range skip {
			if src[i].Id == skip[j].Id {
				continue outer
			}
		}
		dst = append(dst, src[i])
	}
	return
}

// firstLastToSpan returns a span that includes x and y and does not require
// them to be in any order.
func firstLastToSpan(x, y nano.Ts) nano.Span {
	return nano.Span{Ts: x, Dur: 1}.Union(nano.Span{Ts: y, Dur: 1})
}

func spanToFirstLast(order zbuf.Order, span nano.Span) (nano.Ts, nano.Ts) {
	if order == zbuf.OrderAsc {
		return span.Ts, span.End() - 1
	}
	return span.End() - 1, span.Ts
}

func nextTs(ts nano.Ts, order zbuf.Order) nano.Ts {
	if order == zbuf.OrderAsc {
		return ts + 1
	}
	return ts - 1
}

func prevTs(ts nano.Ts, order zbuf.Order) nano.Ts {
	if order == zbuf.OrderAsc {
		return ts - 1
	}
	return ts + 1
}

type point struct {
	idx   int
	first bool
	ts    nano.Ts
}

// boundaries sorts the given chunks, then calls fn with each timestamp that
// acts as a first and/or last timestamp of one or more of the chunks.
func boundaries(chunks []Chunk, order zbuf.Order, fn func(ts nano.Ts, firstChunks, lastChunks []Chunk)) {
	points := make([]point, 2*len(chunks))
	for i, c := range chunks {
		points[2*i] = point{idx: i, first: true, ts: c.First}
		points[2*i+1] = point{idx: i, ts: c.Last}
	}
	sort.Slice(points, func(i, j int) bool {
		if order == zbuf.OrderAsc {
			return points[i].ts < points[j].ts
		}
		return points[j].ts < points[i].ts
	})
	firstChunks := make([]Chunk, 0, len(chunks))
	lastChunks := make([]Chunk, 0, len(chunks))
	for i := 0; i < len(points); {
		j := i + 1
		for ; j < len(points); j++ {
			if points[i].ts != points[j].ts {
				break
			}
		}
		firstChunks = firstChunks[:0]
		lastChunks = lastChunks[:0]
		for _, p := range points[i:j] {
			if p.first {
				firstChunks = append(firstChunks, chunks[p.idx])
			} else {
				lastChunks = append(lastChunks, chunks[p.idx])
			}
		}
		ts := points[i].ts
		i = j
		fn(ts, firstChunks, lastChunks)
	}
}

func largestChunk(si SpanInfo) Chunk {
	res := si.Chunks[0]
	for _, c := range si.Chunks {
		if c.RecordCount > res.RecordCount {
			res = c
		}
	}
	return res
}

func spanInfoContainsChunk(si SpanInfo, cid ksuid.KSUID) bool {
	for _, c := range si.Chunks {
		if c.Id == cid {
			return true
		}
	}
	return false
}

func mergeSpanInfos(sis []SpanInfo, order zbuf.Order) SpanInfo {
	if len(sis) == 1 {
		return sis[0]
	}
	var res SpanInfo
	res.Span = sis[0].Span
	for _, si := range sis {
		res.Span = res.Span.Union(si.Span)
		for _, c := range si.Chunks {
			if !spanInfoContainsChunk(res, c.Id) {
				res.Chunks = append(res.Chunks, c)
			}
		}
	}
	chunksSort(order, res.Chunks)
	return res
}

// mergeLargestChunkSpanInfos merges contiguous SpanInfo's whose largest
// Chunk by RecordCount is the same.
func mergeLargestChunkSpanInfos(spans []SpanInfo, order zbuf.Order) []SpanInfo {
	if len(spans) < 2 {
		return spans
	}
	var res []SpanInfo
	run := []SpanInfo{spans[0]}
	runLargest := largestChunk(spans[0])
	for _, s := range spans[1:] {
		largest := largestChunk(s)
		if largest.Id != runLargest.Id {
			res = append(res, mergeSpanInfos(run, order))
			run = []SpanInfo{s}
			runLargest = largest
		} else {
			run = append(run, s)
		}
	}
	return append(res, mergeSpanInfos(run, order))
}

// removeMaskedChunks performs an in-place edit of the spans input slice,
// updating each SpanInfo to remove chunks that mask other chunks within the
// same SpanInfo.
// If trackMasked is true, it will return a slice of chunks that have been
// masked and are no longer present in any SpanInfo.
func removeMaskedChunks(spans []SpanInfo, trackMasked bool) []Chunk {
	var maskedChunks map[ksuid.KSUID]Chunk
	for i := range spans {
		rem := spans[i].RemoveMasked()
		if trackMasked && len(rem) > 0 {
			if maskedChunks == nil {
				maskedChunks = make(map[ksuid.KSUID]Chunk)
			}
			for _, c := range rem {
				maskedChunks[c.Id] = c
			}
		}
	}
	var mc []Chunk
	if trackMasked {
	outer:
		for id := range maskedChunks {
			// This check ensures that only chunks that are completely masked
			// within the input []SpanInfo are returned.
			for _, si := range spans {
				if spanInfoContainsChunk(si, id) {
					continue outer
				}
			}
			mc = append(mc, maskedChunks[id])
		}
	}
	return mc
}

type compactWriter struct {
	ark   *Archive
	ctx   context.Context
	masks []ksuid.KSUID
	tsd   tsDir
	w     *chunkWriter
}

func (cw *compactWriter) Write(rec *zng.Record) error {
	if cw.w != nil {
		pos, firstTs, lastTs := cw.w.position()
		if pos > cw.ark.LogSizeThreshold && lastTs != rec.Ts() {
			// If we need to create a new chunk writer, we must ensure that the
			// span for the current chunk leaves no gap between its last timestamp
			// and the first timestamp of our next chunk, to ensure these chunks
			// are seen as covering the entire span of the source chunks. Hence
			// the use of 'chunkLastTs', and the lastTs check above to ensure we are
			// not in a run of records with the same timestamp.
			chunkLastTs := prevTs(rec.Ts(), cw.ark.DataOrder)
			if _, err := cw.w.closeWithTs(cw.ctx, firstTs, chunkLastTs); err != nil {
				return err
			}
			cw.w = nil
		}
	}
	if cw.w == nil {
		var err error
		cw.w, err = newChunkWriter(cw.ctx, cw.ark, cw.tsd, cw.masks)
		if err != nil {
			return err
		}
	}
	return cw.w.Write(rec)
}

func (cw *compactWriter) abort() {
	if cw.w != nil {
		cw.w.abort()
		cw.w = nil
	}
}

func (cw *compactWriter) close(lastTs nano.Ts) error {
	if cw.w == nil {
		return nil
	}
	_, firstTs, _ := cw.w.position()
	if _, err := cw.w.closeWithTs(cw.ctx, firstTs, lastTs); err != nil {
		return err
	}
	cw.w = nil
	return nil
}

func compactOverlaps(ctx context.Context, ark *Archive, s SpanInfo) error {
	if len(s.Chunks) == 1 {
		return nil
	}
	ss, _, err := newSpanScanner(ctx, ark, resolver.NewContext(), driver.SourceFilter{Span: nano.MaxSpan}, s)
	if err != nil {
		return err
	}
	defer ss.Close()
	var masks []ksuid.KSUID
	for _, c := range s.Chunks {
		masks = append(masks, c.Id)
	}
	mw := &compactWriter{
		ark:   ark,
		ctx:   ctx,
		masks: masks,
		tsd:   newTsDir(s.Span.Ts),
	}
	_, slast := spanToFirstLast(ark.DataOrder, s.Span)
	if err := zbuf.CopyWithContext(ctx, mw, zbuf.PullerReader(ss)); err != nil {
		mw.abort()
		return err
	}
	return mw.close(slast)
}

// mergeCommonChunkSpans generates a SpanInfo slice by merging runs of
// SpanInfos where a SpanInfo has some chunk in common with its previous
// SpanInfo.
func mergeCommonChunkSpans(spans []SpanInfo, order zbuf.Order) []SpanInfo {
	if len(spans) < 2 {
		return spans
	}
	var res []SpanInfo
	run := []SpanInfo{spans[0]}
outer:
	for _, s := range spans[1:] {
		for _, c := range s.Chunks {
			if spanInfoContainsChunk(run[len(run)-1], c.Id) {
				run = append(run, s)
				continue outer
			}
		}
		res = append(res, mergeSpanInfos(run, order))
		run = []SpanInfo{s}
	}
	return append(res, mergeSpanInfos(run, order))
}

func Compact(ctx context.Context, ark *Archive) error {
	group, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(int64(runtime.GOMAXPROCS(0)))
	group.Go(func() error {
		return tsDirVisit(ctx, ark, nano.MaxSpan, func(_ tsDir, chunks []Chunk) error {
			spans := alignChunksToSpans(chunks, ark.DataOrder, nano.MaxSpan)
			removeMaskedChunks(spans, false)
			spans = mergeCommonChunkSpans(spans, ark.DataOrder)
			for _, s := range spans {
				if err := sem.Acquire(ctx, 1); err != nil {
					return err
				}
				s := s
				group.Go(func() error {
					return compactOverlaps(ctx, ark, s)
				})
			}
			return nil
		})
	})
	return group.Wait()
}

func Purge(ctx context.Context, ark *Archive) error {
	return tsDirVisit(ctx, ark, nano.MaxSpan, func(_ tsDir, chunks []Chunk) error {
		spans := alignChunksToSpans(chunks, ark.DataOrder, nano.MaxSpan)
		maskedChunks := removeMaskedChunks(spans, true)
		if len(maskedChunks) == 0 {
			return nil
		}
		for _, c := range maskedChunks {
			if err := c.Remove(ctx, ark); err != nil {
				return err
			}
		}
		return nil
	})
}
