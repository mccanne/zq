package anyio

import (
	"fmt"
	"io"

	"github.com/brimdata/zed/zio"
	"github.com/brimdata/zed/zio/csvio"
	"github.com/brimdata/zed/zio/jsonio"
	"github.com/brimdata/zed/zio/lakeio"
	"github.com/brimdata/zed/zio/ndjsonio"
	"github.com/brimdata/zed/zio/parquetio"
	"github.com/brimdata/zed/zio/tableio"
	"github.com/brimdata/zed/zio/textio"
	"github.com/brimdata/zed/zio/tzngio"
	"github.com/brimdata/zed/zio/zeekio"
	"github.com/brimdata/zed/zio/zjsonio"
	"github.com/brimdata/zed/zio/zngio"
	"github.com/brimdata/zed/zio/zsonio"
	"github.com/brimdata/zed/zio/zstio"
	"github.com/brimdata/zed/zng"
)

type WriterOpts struct {
	Format string
	UTF8   bool
	JSON   jsonio.WriterOpts
	Text   textio.WriterOpts
	Zng    zngio.WriterOpts
	ZSON   zsonio.WriterOpts
	Zst    zstio.WriterOpts
}

func NewWriter(w io.WriteCloser, opts WriterOpts) (zio.WriteCloser, error) {
	if opts.Format == "" {
		opts.Format = "tzng"
	}
	switch opts.Format {
	case "null":
		return &nullWriter{}, nil
	case "tzng":
		return tzngio.NewWriter(w), nil
	case "zng":
		return zngio.NewWriter(w, opts.Zng), nil
	case "zeek":
		return zeekio.NewWriter(w, opts.UTF8), nil
	case "ndjson":
		return ndjsonio.NewWriter(w), nil
	case "json":
		return jsonio.NewWriter(w, opts.JSON), nil
	case "zjson":
		return zjsonio.NewWriter(w), nil
	case "zson":
		return zsonio.NewWriter(w, opts.ZSON), nil
	case "zst":
		return zstio.NewWriter(w, opts.Zst)
	case "text":
		return textio.NewWriter(w, opts.UTF8, opts.Text), nil
	case "table":
		return tableio.NewWriter(w, opts.UTF8), nil
	case "csv":
		return csvio.NewWriter(w, csvio.WriterOpts{UTF8: opts.UTF8}), nil
	case "parquet":
		return parquetio.NewWriter(w), nil
	case "lake":
		return lakeio.NewWriter(w), nil
	default:
		return nil, fmt.Errorf("unknown format: %s", opts.Format)
	}
}

type nullWriter struct{}

func (*nullWriter) Write(*zng.Record) error {
	return nil
}

func (*nullWriter) Close() error {
	return nil
}
