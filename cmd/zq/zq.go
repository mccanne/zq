package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/brimsec/zq/cli"
	"github.com/brimsec/zq/cli/inputflags"
	"github.com/brimsec/zq/cli/outputflags"
	"github.com/brimsec/zq/cli/procflags"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/pkg/s3io"
	"github.com/brimsec/zq/pkg/signalctx"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng/resolver"
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

The ZQL query may be specified in a source file using -z, which is particularly
convenient when a large, complex query spans multiple lines.  In this case,
a ZQL query string additionaly specified on the command line will be interpreted
as a file path, which typically will result in a "not found" error.

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
	parallel    bool
	zqlPath     string
	inputFlags  inputflags.Flags
	outputFlags outputflags.Flags
	procFlags   procflags.Flags
	cli         cli.Flags
}

func New(f *flag.FlagSet) (charm.Command, error) {
	c := &Command{}

	// Flags added for writers are -f, -T, -F, -E, -U, and -b
	c.outputFlags.SetFlags(f)

	// Flags added for readers are -i, etc
	c.inputFlags.SetFlags(f)

	c.procFlags.SetFlags(f)

	c.cli.SetFlags(f)

	f.BoolVar(&c.verbose, "v", false, "show verbose details")
	f.BoolVar(&c.stats, "S", false, "display search stats on stderr")
	f.BoolVar(&c.quiet, "q", false, "don't display zql warnings")
	f.BoolVar(&c.stopErr, "e", true, "stop upon input errors")
	f.BoolVar(&c.parallel, "P", false, "read two or more files into parallel-input zql query")
	f.StringVar(&c.zqlPath, "z", "", "source file containing zql query text")
	return c, nil
}

func (c *Command) Run(args []string) error {
	defer c.cli.Cleanup()
	err := c.cli.Init(&c.outputFlags, &c.inputFlags, &c.procFlags)
	if len(args) == 0 {
		return Zq.Exec(c, []string{"help"})
	}
	if err != nil {
		return err
	}
	paths := args
	zqlSrc := "*"
	if cli.FileExists(paths[0]) || s3io.IsS3Path(paths[0]) {
		if c.zqlPath != "" {
			b, err := ioutil.ReadFile(c.zqlPath)
			if err != nil {
				return err
			}
			zqlSrc = string(b)
		}
	} else {
		if c.zqlPath != "" || len(paths) == 1 {
			return fmt.Errorf("not found: %s", paths[0])
		}
		zqlSrc = paths[0]
		paths = paths[1:]
	}
	query, err := compiler.ParseProgram(zqlSrc)
	if err != nil {
		return fmt.Errorf("parse error: %s", err)
	}
	if _, err := rlimit.RaiseOpenFilesLimit(); err != nil {
		return err
	}
	zctx := resolver.NewContext()
	readers, err := c.inputFlags.Open(zctx, paths, c.stopErr)
	if err != nil {
		return err
	}

	ctx, cancel := signalctx.New(os.Interrupt)
	defer cancel()

	writer, err := c.outputFlags.Open(ctx)
	if err != nil {
		return err
	}
	d := driver.NewCLI(writer)
	if !c.quiet {
		d.SetWarningsWriter(os.Stderr)
	}
	if !c.stopErr {
		for i, r := range readers {
			readers[i] = zbuf.NewWarningReader(r, d)
		}
	}
	defer zbuf.CloseReaders(readers)

	if c.parallel {
		if err := driver.RunParallel(ctx, d, query, zctx, readers, driver.Config{}); err != nil {
			writer.Close()
			return err
		}
	} else {
		reader, err := zbuf.MergeReadersByTsAsReader(ctx, readers, zbuf.OrderAsc)
		if err != nil {
			writer.Close()
			return err
		}
		if err := driver.Run(ctx, d, query, zctx, reader, driver.Config{}); err != nil {
			writer.Close()
			return err
		}
	}
	return writer.Close()
}
