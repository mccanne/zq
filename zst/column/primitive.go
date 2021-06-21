package column

import (
	"errors"
	"io"

	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

type PrimitiveWriter struct {
	bytes    zcode.Bytes
	spiller  *Spiller
	segments []Segment
}

func NewPrimitiveWriter(spiller *Spiller) *PrimitiveWriter {
	return &PrimitiveWriter{
		spiller: spiller,
	}
}

func (p *PrimitiveWriter) Write(body zcode.Bytes) error {
	p.bytes = zcode.AppendPrimitive(p.bytes, body)
	var err error
	if len(p.bytes) >= p.spiller.Thresh {
		err = p.Flush(false)
	}
	return err
}

func (p *PrimitiveWriter) Flush(eof bool) error {
	var err error
	if len(p.bytes) > 0 {
		p.segments, err = p.spiller.Write(p.segments, p.bytes)
		p.bytes = p.bytes[:0]
	}
	return err
}

func (p *PrimitiveWriter) MarshalZNG(zctx *zson.Context, b *zcode.Builder) (zng.Type, error) {
	b.BeginContainer()
	for _, segment := range p.segments {
		// Add a segmap record to the array for each segment.
		b.BeginContainer()
		b.AppendPrimitive(zng.EncodeInt(segment.Offset))
		b.AppendPrimitive(zng.EncodeInt(segment.Length))
		b.EndContainer()
	}
	b.EndContainer()
	return zson.ParseType(zctx, SegmapTypeString)
}

type Primitive struct {
	iter   zcode.Iter
	segmap []Segment
	reader io.ReaderAt
}

func (p *Primitive) UnmarshalZNG(in zng.Value, reader io.ReaderAt) error {
	p.reader = reader
	return UnmarshalSegmap(in, &p.segmap)
}

func (p *Primitive) Read(b *zcode.Builder) error {
	zv, err := p.read()
	if err == nil {
		b.AppendPrimitive(zv)
	}
	return err
}

func (p *Primitive) read() (zcode.Bytes, error) {
	if p.iter == nil || p.iter.Done() {
		if len(p.segmap) == 0 {
			return nil, io.EOF
		}
		if err := p.next(); err != nil {
			return nil, err
		}
	}
	zv, _, err := p.iter.Next()
	return zv, err
}

func (p *Primitive) next() error {
	segment := p.segmap[0]
	p.segmap = p.segmap[1:]
	if segment.Length > 2*MaxSegmentThresh {
		return errors.New("segment too big")
	}
	b := make([]byte, segment.Length)
	//XXX this where lots of seeks can happen until we put intelligent
	// scheduling in a layer below this informed by the reassembly maps
	// and the query that is going to run.
	n, err := p.reader.ReadAt(b, segment.Offset)
	if err != nil {
		return err
	}
	if n < int(segment.Length) {
		return errors.New("truncated read of zst column")
	}
	p.iter = zcode.Iter(b)
	return nil
}
