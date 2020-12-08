package chunk

import (
	"context"
	"io"

	"github.com/brimsec/zq/pkg/bufwriter"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/archive/seekindex"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng"
	"github.com/segmentio/ksuid"
)

// Writer is a zbuf.Writer that writes a stream of sorted records into an
// archive chunk file.
type Writer struct {
	byteCounter    *writeCounter
	count          uint64
	dataFileWriter *zngio.Writer
	firstTs        nano.Ts
	id             ksuid.KSUID
	lastTs         nano.Ts
	masks          []ksuid.KSUID
	needIndexWrite bool
	order          zbuf.Order
	seekIndex      *seekindex.Builder
	dir            iosrc.URI
	wroteFirst     bool
}

func NewWriter(ctx context.Context, dir iosrc.URI, order zbuf.Order, masks []ksuid.KSUID, opts zngio.WriterOpts) (*Writer, error) {
	id := ksuid.New()
	out, err := iosrc.NewWriter(ctx, dir.AppendPath(ChunkFileName(id)))
	if err != nil {
		return nil, err
	}
	counter := &writeCounter{bufwriter.New(out), 0}
	dataFileWriter := zngio.NewWriter(counter, opts)
	seekIndexPath := chunkSeekIndexPath(dir, id)
	seekIndex, err := seekindex.NewBuilder(ctx, seekIndexPath.String(), order)
	if err != nil {
		return nil, err
	}
	return &Writer{
		byteCounter:    counter,
		dataFileWriter: dataFileWriter,
		id:             id,
		seekIndex:      seekIndex,
		masks:          masks,
		order:          order,
		dir:            dir,
	}, nil
}

func (cw *Writer) Position() (int64, nano.Ts, nano.Ts) {
	return cw.dataFileWriter.Position(), cw.firstTs, cw.lastTs
}

func (cw *Writer) Write(rec *zng.Record) error {
	// We want to index the start of stream (SOS) position of the data file by
	// record timestamps; we don't know when we've started a new stream until
	// after we written the first record in the stream.
	sos := cw.dataFileWriter.LastSOS()
	if err := cw.dataFileWriter.Write(rec); err != nil {
		return err
	}
	ts := rec.Ts()
	if !cw.wroteFirst || (cw.needIndexWrite && ts != cw.lastTs) {
		if err := cw.seekIndex.Enter(ts, sos); err != nil {
			return err
		}
		cw.needIndexWrite = false
	}
	if cw.dataFileWriter.LastSOS() != sos {
		cw.needIndexWrite = true
	}
	if !cw.wroteFirst {
		cw.firstTs = ts
		cw.wroteFirst = true
	}
	cw.lastTs = ts
	cw.count++
	return nil
}

// abort should be called when an error occurs during write. Errors are ignored
// because the write error will be more informative and should be returned.
func (cw *Writer) Abort() {
	cw.dataFileWriter.Close()
	cw.seekIndex.Abort()
}

func (cw *Writer) Close(ctx context.Context) (Chunk, error) {
	return cw.CloseWithTs(ctx, cw.firstTs, cw.lastTs)
}

func (cw *Writer) CloseWithTs(ctx context.Context, firstTs, lastTs nano.Ts) (Chunk, error) {
	err := cw.dataFileWriter.Close()
	if err != nil {
		cw.seekIndex.Abort()
		return Chunk{}, err
	}
	metadata := Metadata{
		First:       firstTs,
		Last:        lastTs,
		RecordCount: cw.count,
		Masks:       cw.masks,
		Size:        cw.dataFileWriter.Position(),
	}
	if err := metadata.Write(ctx, MetadataPath(cw.dir, cw.id), cw.order); err != nil {
		cw.seekIndex.Abort()
		return Chunk{}, err
	}
	if err := cw.seekIndex.Close(); err != nil {
		return Chunk{}, err
	}
	// TODO: zq#1264
	// Add an entry to the update log for S3 backed stores containing the
	// location of the just added data & index file.
	return metadata.Chunk(cw.dir, cw.id), nil
}

func (cw *Writer) BytesWritten() int64 {
	return cw.byteCounter.size
}

func (cw *Writer) RecordsWritten() uint64 {
	return cw.count
}

type writeCounter struct {
	io.WriteCloser
	size int64
}

func (w *writeCounter) Write(b []byte) (int, error) {
	n, err := w.WriteCloser.Write(b)
	w.size += int64(n)
	return n, err
}
