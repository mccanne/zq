package detector

import (
	"errors"
	"os"
	"regexp"

	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/ndjsonio"
	"github.com/brimsec/zq/zng/resolver"
)

type OpenConfig struct {
	Format         string
	DashStdin      bool
	JSONTypeConfig *ndjsonio.TypeConfig
	JSONPathRegex  string
}

// OpenFile creates and returns zbuf.File for the indicated path.  If the path is
// a directory or can't otherwise be open as a file, then an error is returned.
func OpenFile(zctx *resolver.Context, path string, cfg OpenConfig) (*zbuf.File, error) {
	var f *os.File
	var err error
	if cfg.DashStdin && path == "-" {
		f = os.Stdin
	} else {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, errors.New("is a directory")
		}
		f, err = os.Open(path)
		if err != nil {
			return nil, err
		}
	}
	r := GzipReader(f)
	var zr zbuf.Reader
	if cfg.Format == "" || cfg.Format == "auto" {
		zr, err = NewReader(r, zctx)
	} else {
		zr, err = LookupReader(r, zctx, cfg.Format)
	}
	if err != nil {
		return nil, err
	}

	if jr, ok := zr.(*ndjsonio.Reader); ok && cfg.JSONTypeConfig != nil {
		if err = jsonConfig(cfg, jr, path); err != nil {
			return nil, err
		}
	}

	return zbuf.NewFile(zr, f), nil
}

func OpenFiles(zctx *resolver.Context, paths ...string) (*zbuf.Combiner, error) {
	var readers []zbuf.Reader
	for _, path := range paths {
		reader, err := OpenFile(zctx, path, OpenConfig{})
		if err != nil {
			return nil, err
		}
		readers = append(readers, reader)
	}
	return zbuf.NewCombiner(readers), nil
}

func jsonConfig(cfg OpenConfig, jr *ndjsonio.Reader, filename string) error {
	var path string
	re := regexp.MustCompile(cfg.JSONPathRegex)
	match := re.FindStringSubmatch(filename)
	if len(match) == 2 {
		path = match[1]
	}
	return jr.ConfigureTypes(*cfg.JSONTypeConfig, path)
}
