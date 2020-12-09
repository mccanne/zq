package seekindex

import (
	"context"
	"fmt"
	"math"

	"github.com/brimsec/zq/microindex"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
)

type SeekIndex struct {
	finder *microindex.Finder
	uri    iosrc.URI
}

func Open(ctx context.Context, uri iosrc.URI) (*SeekIndex, error) {
	finder, err := microindex.NewFinder(ctx, resolver.NewContext(), uri)
	if err != nil {
		return nil, err
	}
	return &SeekIndex{finder: finder, uri: uri}, nil
}

func (s *SeekIndex) Lookup(ctx context.Context, span nano.Span) (rg Range, err error) {
	if s.finder.Order() == zbuf.OrderDesc {
		rg, err = s.lookupDesc(ctx, span)
	} else {
		rg, err = s.lookupAsc(ctx, span)
	}
	if err != nil {
		return rg, fmt.Errorf("seekindex %s: %w", s.uri, err)
	}
	if rg.Start == -1 {
		rg.Start = 0
	}
	if rg.End == -1 {
		rg.End = math.MaxInt64
	}
	return
}

func (s *SeekIndex) lookupAsc(ctx context.Context, span nano.Span) (rg Range, err error) {
	rg.Start, err = s.offsetAt(ctx, span.Ts, true)
	if err != nil {
		return
	}
	rg.End, err = s.offsetAt(ctx, span.End(), false)
	return
}

func (s *SeekIndex) lookupDesc(ctx context.Context, span nano.Span) (rg Range, err error) {
	rg.Start, err = s.offsetAt(ctx, span.End()-1, false)
	if err != nil {
		return
	}
	rg.End, err = s.offsetAt(ctx, span.Ts-1, true)
	return
}

func (s *SeekIndex) offsetAt(ctx context.Context, ts nano.Ts, less bool) (int64, error) {
	key, err := s.finder.ParseKeys(ts.StringFloat())
	if err != nil {
		return -1, err
	}
	var rec *zng.Record
	// XXX These calls to finder should have the ability to pass context.
	if less {
		rec, err = s.finder.ClosestLTE(key)
	} else {
		rec, err = s.finder.ClosestGTE(key)
	}
	if rec == nil || err != nil {
		return -1, err
	}
	return rec.AccessInt("offset")
}

func (s *SeekIndex) Close() error {
	return s.finder.Close()
}
