package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/cli/zq"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/pkg/s3io"
	"github.com/brimsec/zq/pkg/signalctx"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zql"
	"github.com/mccanne/charm"
)

var Zq = &charm.Spec{
	Name:        "zq",
	Usage:       "zq [ options ] [ zql ] file [ file ... ]",
	Short:       "command line logs processor",
	HiddenFlags: "cpuprofile,memprofile,pathregexp",
	Long: `
zq is a command-line tool for processing logs.  It applies boolean logic
to filter each log value, optionally computes analytics and transformations,
and writes the output to one or more files or standard output.

zq must be run with at least one input file specified.  As with awk, standard input
can be specified with a "-" in the place of the file name.  Output is sent to
standard output unless a -o or -d argument is provided, in which case output is
sent to the indicated file comforming to the type implied by the extension (unless
-f explicitly indicates the output type).

Supported input formats include binary and text zng, NDJSON, and
the Zeek log format.  Supported output formats include
all the input formats along with text and tabular formats.

The input file format is inferred from the data.  If multiple files are
specified, each file format is determined independently so you can mix and
match input types.  If multiple files are concatenated into a stream and
presented as standard input, the files must all be of the same type as the
beginning of stream will determine the format.

The output format is text zng by default, but can be overridden with -f.

After the options, the query may be specified as a
single argument conforming with ZQL syntax; i.e., it should be quoted as
a single string in the shell.
If the first argument is a path to a valid file rather than a ZQL query,
then the ZQL expression is assumed to be "*", i.e., match and output all
of the input.  If the first argument is both valid ZQL and an existing file,
then the file overrides.

See the zq source repository for more information:

https://github.com/brimsec/zq
`,
	New: func(parent charm.Command, flags *flag.FlagSet) (charm.Command, error) {
		return New(flags)
	},
}

func init() {
	Zq.Add(charm.Help)
}

type Command struct {
	verbose     bool
	stats       bool
	quiet       bool
	stopErr     bool
	readerFlags zio.ReaderFlags
	writerFlags zio.WriterFlags
	output      zq.OutputFlags
	cli         zq.Flags
}

func New(f *flag.FlagSet) (charm.Command, error) {
	c := &Command{}

	// Flags added for writers are -f, -T, -F, -E, -U, and -b
	c.writerFlags.SetFlags(f)

	// Flags added for readers are -i, etc
	c.readerFlags.SetFlags(f)

	c.cli.SetFlags(f)
	c.output.SetFlags(f)

	f.BoolVar(&c.verbose, "v", false, "show verbose details")
	f.BoolVar(&c.stats, "S", false, "display search stats on stderr")
	f.BoolVar(&c.quiet, "q", false, "don't display zql warnings")
	f.BoolVar(&c.stopErr, "e", true, "stop upon input errors")
	return c, nil
}

func (c *Command) Run(args []string) error {
	defer c.cli.Cleanup()
	if ok, err := c.cli.Init(); !ok {
		return err
	}
	if len(args) == 0 {
		return Zq.Exec(c, []string{"help"})
	}
	if err := c.readerFlags.Init(); err != nil {
		return err
	}
	writerOpts := c.writerFlags.Options()
	if err := c.output.Init(&writerOpts); err != nil {
		return err
	}
	paths := args
	var query ast.Proc
	var err error
	if zq.FileExists(paths[0]) || s3io.IsS3Path(paths[0]) {
		query, err = zql.ParseProc("*")
		if err != nil {
			return err
		}
	} else {
		paths = paths[1:]
		if len(paths) == 0 {
			return fmt.Errorf("file not found: %s", args[0])
		}
		query, err = zql.ParseProc(args[0])
		if err != nil {
			return fmt.Errorf("parse error: %s", err)
		}
	}
	if _, err := rlimit.RaiseOpenFilesLimit(); err != nil {
		return err
	}
	zctx := resolver.NewContext()
	readers, err := zq.OpenInputs(zctx, c.readerFlags.Options(), paths, c.stopErr)
	if err != nil {
		return err
	}

	wch := make(chan string, 5)
	if !c.stopErr {
		for i, r := range readers {
			readers[i] = zbuf.NewWarningReader(r, wch)
		}
	}
	reader := zbuf.NewCombiner(readers, zbuf.CmpTimeForward)
	defer reader.Close()

	writer, err := c.output.Open(writerOpts)
	if err != nil {
		return err
	}
	d := driver.NewCLI(writer)
	if !c.quiet {
		d.SetWarningsWriter(os.Stderr)
	}
	ctx, cancel := signalctx.New(os.Interrupt)
	defer cancel()
	if err := driver.Run(ctx, d, query, zctx, reader, driver.Config{
		Warnings: wch,
	}); err != nil {
		writer.Close()
		return err
	}
	return writer.Close()
}
