package zq

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/brimsec/zq/cli"
	"github.com/brimsec/zq/emitter"
	"github.com/brimsec/zq/proc/sort"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zng/resolver"
	"golang.org/x/crypto/ssh/terminal"
)

// Version is set via the Go linker.  See Makefile.
var Version = "unknown"

type Flags struct {
	cli.Flags
	sortMemMaxMiB float64
}

func (f *Flags) SetFlags(fs *flag.FlagSet) {
	fs.Float64Var(&f.sortMemMaxMiB, "sortmem", float64(sort.MemMaxBytes)/(1024*1024), "maximum memory used by sort, in MiB")
	f.Flags.SetFlags(fs)
}

func (f *Flags) Init() (bool, error) {
	if ok, err := f.Flags.Init(); !ok {
		fmt.Printf("Version: %s\n", Version)
		return false, err
	}
	if f.sortMemMaxMiB <= 0 {
		return false, errors.New("sortmem value must be greater than zero")
	}
	sort.MemMaxBytes = int(f.sortMemMaxMiB * 1024 * 1024)
	return true, nil
}

type OutputFlags struct {
	dir          string
	outputFile   string
	forceBinary  bool
	textShortcut bool
}

func (f *OutputFlags) SetFlags(fs *flag.FlagSet) {
	fs.StringVar(&f.dir, "d", "", "directory for output data files")
	fs.StringVar(&f.outputFile, "o", "", "write data to output file")
	fs.BoolVar(&f.textShortcut, "t", false, "use format tzng independent of -f option")
	fs.BoolVar(&f.forceBinary, "B", false, "allow binary zng be sent to a terminal output")
}

func (f *OutputFlags) Init(opts *zio.WriterOpts) error {
	if f.textShortcut {
		if opts.Format != "zng" {
			return errors.New("cannot use -t with -f")
		}
		opts.Format = "tzng"
	}
	if f.outputFile == "" && opts.Format == "zng" && IsTerminal(os.Stdout) && !f.forceBinary {
		return errors.New("writing binary zng data to terminal; override with -B or use -t for text.")
	}
	return nil
}

func (f *OutputFlags) FileName() string {
	return f.outputFile
}

func (o *OutputFlags) Open(opts zio.WriterOpts) (zbuf.WriteCloser, error) {
	if o.dir != "" {
		d, err := emitter.NewDir(o.dir, o.outputFile, os.Stderr, opts)
		if err != nil {
			return nil, err
		}
		return d, nil
	}
	w, err := emitter.NewFile(o.outputFile, opts)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func OpenInputs(zctx *resolver.Context, opts zio.ReaderOpts, paths []string, stopOnErr bool) ([]zbuf.Reader, error) {
	var readers []zbuf.Reader
	for _, path := range paths {
		if path == "-" {
			path = detector.StdinPath
		}
		file, err := detector.OpenFile(zctx, path, opts)
		if err != nil {
			err = fmt.Errorf("%s: %w", path, err)
			if stopOnErr {
				return nil, err
			}
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		readers = append(readers, file)
	}
	return readers, nil
}

func FileExists(path string) bool {
	if path == "-" {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func IsTerminal(f *os.File) bool {
	return terminal.IsTerminal(int(f.Fd()))
}
