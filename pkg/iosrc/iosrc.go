//go:generate mockgen -destination=./mock/mock_source.go -package=mock github.com/brimsec/zq/pkg/iosrc Source

package iosrc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const FileScheme = "file"

var schemes = map[string]Source{
	"file":  DefaultFileSource,
	"stdio": defaultStdioSource,
	"s3":    defaultS3Source,
}

type Reader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

type Source interface {
	NewReader(context.Context, URI) (Reader, error)
	NewWriter(context.Context, URI) (io.WriteCloser, error)
	ReadFile(context.Context, URI) ([]byte, error)
	WriteFile(context.Context, []byte, URI) error
	Remove(context.Context, URI) error
	RemoveAll(context.Context, URI) error
	// Exists returns true if the specified uri exists and an error is there
	// was an error finding this information.
	Exists(context.Context, URI) (bool, error)
	Stat(context.Context, URI) (Info, error)
	ReadDir(context.Context, URI) ([]Info, error)
}

type Info interface {
	Name() string
	Size() int64
	ModTime() time.Time
	IsDir() bool
}

type DirMaker interface {
	MkdirAll(URI, os.FileMode) error
}

type Replacer interface {
	io.WriteCloser
	Abort()
}

// A ReplacerAble source supports atomic updates to a URI.
type ReplacerAble interface {
	NewReplacer(context.Context, URI) (Replacer, error)
}

func NewReader(ctx context.Context, uri URI) (Reader, error) {
	source, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	return source.NewReader(ctx, uri)
}

func NewWriter(ctx context.Context, uri URI) (io.WriteCloser, error) {
	source, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	return source.NewWriter(ctx, uri)
}

func ReadFile(ctx context.Context, uri URI) ([]byte, error) {
	source, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	return source.ReadFile(ctx, uri)
}

func WriteFile(ctx context.Context, uri URI, d []byte) error {
	source, err := GetSource(uri)
	if err != nil {
		return err
	}
	return source.WriteFile(ctx, d, uri)
}

func Exists(ctx context.Context, uri URI) (bool, error) {
	source, err := GetSource(uri)
	if err != nil {
		return false, err
	}
	return source.Exists(ctx, uri)
}

func Remove(ctx context.Context, uri URI) error {
	source, err := GetSource(uri)
	if err != nil {
		return err
	}
	return source.Remove(ctx, uri)
}

func RemoveAll(ctx context.Context, uri URI) error {
	source, err := GetSource(uri)
	if err != nil {
		return err
	}
	return source.RemoveAll(ctx, uri)
}

func Stat(ctx context.Context, uri URI) (Info, error) {
	source, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	return source.Stat(ctx, uri)
}

func ReadDir(ctx context.Context, uri URI) ([]Info, error) {
	source, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	return source.ReadDir(ctx, uri)
}

func GetSource(uri URI) (Source, error) {
	scheme := getScheme(uri)
	source, ok := schemes[scheme]
	if !ok {
		return nil, fmt.Errorf("unknown scheme: %q", scheme)
	}
	return source, nil
}

func NewReplacer(ctx context.Context, uri URI) (Replacer, error) {
	src, err := GetSource(uri)
	if err != nil {
		return nil, err
	}
	replacerAble, ok := src.(ReplacerAble)
	if !ok {
		return nil, errors.New("source does not support replacement")
	}
	return replacerAble.NewReplacer(ctx, uri)
}

func Replace(ctx context.Context, uri URI, fn func(w io.Writer) error) error {
	src, err := GetSource(uri)
	if err != nil {
		return err
	}
	replacerAble, ok := src.(ReplacerAble)
	if !ok {
		return errors.New("source does not support replacement")
	}
	r, err := replacerAble.NewReplacer(ctx, uri)
	if err != nil {
		return err
	}
	if err := fn(r); err != nil {
		r.Abort()
		return err
	}
	return r.Close()
}

func getScheme(uri URI) string {
	if uri.Scheme == "" {
		return FileScheme
	}
	return uri.Scheme
}
