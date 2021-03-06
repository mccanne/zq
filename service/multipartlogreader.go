package service

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"sync/atomic"

	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/driver"
	"github.com/brimdata/zed/zio"
	"github.com/brimdata/zed/zio/anyio"
	"github.com/brimdata/zed/zio/zngio"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zqe"
	"github.com/brimdata/zed/zson"
)

const maxShaperAstBytes = 1000 * 1000

type MultipartLogReader struct {
	mr        *multipart.Reader
	opts      anyio.ReaderOpts
	stopErr   bool
	shaperAST ast.Proc
	warnings  []string
	zreader   zio.ReadCloser
	zctx      *zson.Context
	nread     int64
}

func NewMultipartLogReader(mr *multipart.Reader, zctx *zson.Context) *MultipartLogReader {
	return &MultipartLogReader{
		mr:   mr,
		opts: anyio.ReaderOpts{Zng: zngio.ReaderOpts{Validate: true}},
		zctx: zctx,
	}
}

func (m *MultipartLogReader) SetStopOnError() {
	m.stopErr = true
}

func (m *MultipartLogReader) Read() (*zng.Record, error) {
read:
	if m.zreader == nil {
		zr, err := m.next()
		if zr == nil || err != nil {
			return nil, err
		}
		m.zreader = zr
	}
	rec, err := m.zreader.Read()
	if err != nil || rec == nil {
		zr := m.zreader
		m.zreader.Close()
		m.zreader = nil
		if err != nil {
			if m.stopErr {
				return nil, err
			}
			m.appendWarning(zr, err)
		}
		goto read
	}
	return rec, err
}

func (m *MultipartLogReader) next() (zio.ReadCloser, error) {
next:
	if m.mr == nil {
		return nil, nil
	}
	part, err := m.mr.NextPart()
	if err != nil {
		if err == io.EOF {
			m.mr, err = nil, nil
		}
		return nil, err
	}
	if part.FormName() == "shaper_ast" {
		raw, err := io.ReadAll(io.LimitReader(part, maxShaperAstBytes))
		if err != nil {
			return nil, zqe.ErrInvalid("shaper_ast too big")
		}
		proc, err := ast.UnpackJSONAsProc(raw)
		if err != nil {
			return nil, err
		}
		m.shaperAST = proc
		goto next
	}
	name := part.FileName()
	counter := &mpcounter{part, &m.nread}
	var zr zio.ReadCloser
	zr, err = anyio.OpenFromNamedReadCloser(m.zctx, counter, name, m.opts)
	if err != nil {
		part.Close()
		if m.stopErr {
			return nil, err
		}
		m.appendWarning(zr, err)
		goto next
	}
	if m.shaperAST != nil {
		zr, err = driver.NewReader(context.Background(), m.shaperAST, m.zctx, zr)
		if err != nil {
			return nil, err
		}
	}
	return zr, err
}

func (m *MultipartLogReader) appendWarning(zr zio.Reader, err error) {
	m.warnings = append(m.warnings, fmt.Sprintf("%s: %s", zr, err))
}

func (m *MultipartLogReader) Warnings() []string {
	return m.warnings
}

func (m *MultipartLogReader) BytesRead() int64 {
	return atomic.LoadInt64(&m.nread)
}

type mpcounter struct {
	*multipart.Part
	nread *int64
}

func (r *mpcounter) Read(b []byte) (int, error) {
	n, err := r.Part.Read(b)
	atomic.AddInt64(r.nread, int64(n))
	return n, err
}
