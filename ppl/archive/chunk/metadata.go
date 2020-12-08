package chunk

import (
	"context"
	"fmt"

	"github.com/brimsec/zq/pkg/bufwriter"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type Metadata struct {
	First       nano.Ts
	Last        nano.Ts
	RecordCount uint64
	Masks       []ksuid.KSUID
	Size        int64
}

func ReadMetadata(ctx context.Context, uri iosrc.URI, order zbuf.Order) (Metadata, error) {
	in, err := iosrc.NewReader(ctx, uri)
	if err != nil {
		return Metadata{}, err
	}
	defer in.Close()
	zctx := resolver.NewContext()
	zr := zngio.NewReader(in, zctx)
	rec, err := zr.Read()
	if err != nil {
		return Metadata{}, err
	}
	var md Metadata
	if err := resolver.UnmarshalRecord(zctx, rec, &md); err != nil {
		return Metadata{}, err
	}
	if err := mdTsOrderCheck(uri, "read", order, md.First, md.Last); err != nil {
		return Metadata{}, err
	}
	return md, nil
}

func (m Metadata) Chunk(dir iosrc.URI, id ksuid.KSUID) Chunk {
	return Chunk{
		Dir:         dir,
		Id:          id,
		First:       m.First,
		Last:        m.Last,
		RecordCount: m.RecordCount,
		Masks:       m.Masks,
		Size:        m.Size,
	}
}

func (m Metadata) Write(ctx context.Context, uri iosrc.URI, order zbuf.Order) error {
	if err := mdTsOrderCheck(uri, "write", order, m.First, m.Last); err != nil {
		return err
	}
	zctx := resolver.NewContext()
	rec, err := resolver.MarshalRecord(zctx, m)
	if err != nil {
		return err
	}
	out, err := iosrc.NewWriter(ctx, uri)
	if err != nil {
		return err
	}
	zw := zngio.NewWriter(bufwriter.New(out), zngio.WriterOpts{})
	if err := zw.Write(rec); err != nil {
		zw.Close()
		return err
	}
	return zw.Close()
}

func MetadataPath(dir iosrc.URI, id ksuid.KSUID) iosrc.URI {
	return dir.AppendPath(fmt.Sprintf("%s-%s.zng", FileKindMetadata, id))
}

func mdTsOrderCheck(u iosrc.URI, op string, order zbuf.Order, first, last nano.Ts) error {
	x, y := first, last
	if order == zbuf.OrderDesc {
		x, y = y, x
	}
	if x <= y {
		return nil
	}
	err := fmt.Errorf("metadata failed order check %s op %s order %s first %v last %v", u, op, order, int64(first), int64(last))
	zap.L().Error("mdTsOrderCheck failed", zap.Error(err), zap.Stack("stack"))
	return err
}
