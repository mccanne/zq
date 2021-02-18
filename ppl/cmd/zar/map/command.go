package zarmap

import (
	"context"
	"errors"
	"flag"
	"os"

	"github.com/brimsec/zq/cli/outputflags"
	"github.com/brimsec/zq/cli/procflags"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/emitter"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/pkg/signalctx"
	"github.com/brimsec/zq/ppl/cmd/zar/root"
	"github.com/brimsec/zq/ppl/lake"
	"github.com/brimsec/zq/ppl/lake/chunk"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/mccanne/charm"
)

var Map = &charm.Spec{
	Name:  "map",
	Usage: "map [-R root] [options] zql [file...]",
	Short: "execute ZQL for each archive directory",
	Long: `
"zar map" executes a ZQL query on one or more files in each of the
chunk directories of a zar archive, sending its output to either stdout,
or to a per-directory file, specified via "-o". By default, the chunk file
is the sole input file; alternatively, one or more file names relative to
each zar subdirectory may be given, using the special name "_" to refer to the
chunk file itself.
`,
	New: New,
}

func init() {
	root.Zar.Add(Map)
}

type Command struct {
	*root.Command
	quiet       bool
	root        string
	stopErr     bool
	outputFlags outputflags.Flags
	procFlags   procflags.Flags
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	f.BoolVar(&c.quiet, "q", false, "don't display zql warnings")
	f.StringVar(&c.root, "R", os.Getenv("ZAR_ROOT"), "root directory of zar archive to walk")
	f.BoolVar(&c.stopErr, "e", true, "stop upon input errors")
	c.outputFlags.SetFlags(f)
	c.procFlags.SetFlags(f)
	return c, nil
}

//XXX lots here copied from zq command... we should refactor into a tools package
func (c *Command) Run(args []string) error {
	defer c.Cleanup()
	if err := c.Init(&c.outputFlags, &c.procFlags); err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("zar map needs input arguments")
	}
	query, err := compiler.ParseProgram(args[0])
	if err != nil {
		return err
	}
	inputs := args[1:]
	if len(inputs) == 0 {
		inputs = []string{"_"}
	}

	// Don't allow non-zng to be written inside the archive.
	// XXX we should allow outputFlags to parameterize this so help doesn't show the other formats
	if c.outputFlags.FileName() != "" && c.outputFlags.Format != "zng" {
		return errors.New("only zng format allowed for chunk associated files")
	}

	if _, err := rlimit.RaiseOpenFilesLimit(); err != nil {
		return err
	}

	ctx, cancel := signalctx.New(os.Interrupt)
	defer cancel()

	lk, err := lake.OpenLakeWithContext(ctx, c.root, nil)
	if err != nil {
		return err
	}

	// XXX this is parallelizable except for writing to stdout when
	// concatenating results
	return lake.Walk(ctx, lk, func(chunk chunk.Chunk) error {
		zardir := chunk.ZarDir()
		var paths []string
		for _, input := range inputs {
			paths = append(paths, chunk.Localize(input).String())
		}
		zctx := resolver.NewContext()
		opts := zio.ReaderOpts{Format: "zng"}
		rc := detector.MultiFileReader(zctx, paths, opts)
		defer rc.Close()
		reader := zbuf.Reader(rc)
		writer, err := c.openOutput(ctx, zardir)
		if err != nil {
			return err
		}
		d := driver.NewCLI(writer)
		if !c.stopErr {
			reader = zbuf.NewWarningReader(reader, d)
		}
		if !c.quiet {
			d.SetWarningsWriter(os.Stderr)
		}
		err = driver.Run(ctx, d, query, zctx, reader, driver.Config{
			ReaderSortKey:     "ts",
			ReaderSortReverse: lk.DataOrder == zbuf.OrderDesc,
		})
		if closeErr := writer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		return err
	})
}

func (c *Command) openOutput(ctx context.Context, zardir iosrc.URI) (zbuf.WriteCloser, error) {
	var path string
	if filename := c.outputFlags.FileName(); filename != "" {
		path = zardir.AppendPath(filename).String()
	}
	w, err := emitter.NewFile(ctx, path, c.outputFlags.Options())
	if err != nil {
		return nil, err
	}
	return w, nil
}
