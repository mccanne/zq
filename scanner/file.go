package scanner

import (
	"errors"
	"fmt"
	"os"

	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
)

type File struct {
	zbuf.Reader
	file *os.File
}

func OpenFile(zctx *resolver.Context, path, ifmt string) (*File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("is a directory")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	r := detector.GzipReader(f)
	var zr zbuf.Reader
	if ifmt == "auto" {
		zr, err = detector.NewReader(r, zctx)
	} else {
		zr, err = detector.LookupReader(ifmt, r, zctx)
	}
	if err != nil {
		return nil, err
	}
	return &File{zr, f}, nil
}

func (r *File) Close() error {
	return r.file.Close()
}

func (r *File) String() string {
	return r.file.Name()
}

// WarningReader returns a zbuf.Reader that reads from zr.  Any error encountered is
// sent to ch, and then a nil *zng.Record and nil error are returned.
func WarningReader(zr zbuf.Reader, ch chan string) zbuf.Reader {
	return &warningReader{zr: zr, ch: ch}
}

type warningReader struct {
	zr zbuf.Reader
	ch chan string
}

func (w *warningReader) Read() (*zng.Record, error) {
	rec, err := w.zr.Read()
	if err != nil {
		w.ch <- fmt.Sprintf("%s: %s", w.zr, err)
		return nil, nil
	}
	return rec, nil
}
