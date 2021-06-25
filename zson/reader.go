package zson

import (
	"fmt"
	"io"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/zcode"
)

type Reader struct {
	reader   io.Reader
	zctx     *Context
	parser   *Parser
	analyzer Analyzer
	builder  *zcode.Builder
}

func NewReader(r io.Reader, zctx *Context) *Reader {
	return &Reader{
		reader:   r,
		zctx:     zctx,
		analyzer: NewAnalyzer(),
		builder:  zcode.NewBuilder(),
	}
}

func (r *Reader) Read() (*zed.Record, error) {
	if r.parser == nil {
		var err error
		r.parser, err = NewParser(r.reader)
		if err != nil {
			return nil, err
		}
	}
	ast, err := r.parser.ParseValue()
	if ast == nil || err != nil {
		return nil, err
	}
	val, err := r.analyzer.ConvertValue(r.zctx, ast)
	if err != nil {
		return nil, err
	}
	zv, err := Build(r.builder, val)
	if err != nil {
		return nil, err
	}
	// ZSON can represent value streams that aren't records,
	// but we handle only top-level records here.
	if _, ok := zed.AliasOf(zv.Type).(*zed.TypeRecord); !ok {
		return nil, fmt.Errorf("top-level ZSON value not a record: %s", zv.Type)
	}
	return zed.NewRecord(zv.Type, zv.Bytes), nil
}
