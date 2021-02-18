package zq

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/brimsec/zq/cli/outputflags"
	"github.com/brimsec/zq/cli/procflags"
	"github.com/brimsec/zq/cli/searchflags"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/pkg/signalctx"
	"github.com/brimsec/zq/ppl/cmd/zar/root"
	"github.com/brimsec/zq/ppl/lake"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/mccanne/charm"
)

var Zq = &charm.Spec{
	Name:  "zq",
	Usage: "zq [-R root] [options] zql [file...]",
	Short: "execute ZQL against all archive directories",
	Long: `
"zar zq" executes a ZQL query against one or more files from all the directories
of an archive, generating a single result stream. By default, the chunk file in
each directory is used, but one or more files may be specified. The special file
name "_" refers to the chunk file itself, and other names are interpreted
relative to each chunk's directory.
`,
	New: New,
}

func init() {
	root.Zar.Add(Zq)
}

type Command struct {
	*root.Command
	quiet       bool
	root        string
	stats       bool
	stopErr     bool
	outputFlags outputflags.Flags
	procFlags   procflags.Flags
	searchFlags searchflags.Flags
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	f.BoolVar(&c.quiet, "q", false, "don't display zql warnings")
	f.StringVar(&c.root, "R", os.Getenv("ZAR_ROOT"), "root directory of zar archive to walk")
	f.BoolVar(&c.stats, "s", false, "print search stats to stderr on successful completion")
	f.BoolVar(&c.stopErr, "e", true, "stop upon input errors")
	c.outputFlags.SetFlags(f)
	c.procFlags.SetFlags(f)
	c.searchFlags.SetFlags(f)
	return c, nil
}

func (c *Command) Run(args []string) error {
	defer c.Cleanup()
	if err := c.Init(&c.outputFlags, &c.procFlags, &c.searchFlags); err != nil {
		return err
	}

	if _, err := rlimit.RaiseOpenFilesLimit(); err != nil {
		return err
	}

	query, err := compiler.ParseProgram(args[0])
	if err != nil {
		return err
	}

	lk, err := lake.OpenLake(c.root, nil)
	if err != nil {
		return err
	}
	msrc := lake.NewMultiSource(lk, args[1:])

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
	err = driver.MultiRun(ctx, d, query, resolver.NewContext(), msrc, driver.MultiConfig{
		Span: c.searchFlags.Span(),
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		c.printStats(msrc)
	}
	return err
}

func (c *Command) printStats(msrc lake.MultiSource) {
	if c.stats {
		stats := msrc.Stats()
		w := tabwriter.NewWriter(os.Stderr, 0, 0, 1, ' ', 0)
		fmt.Fprintf(w, "data opened:\t%d\n", stats.ChunksOpenedBytes)
		fmt.Fprintf(w, "data read:\t%d\n", stats.ChunksReadBytes)
		w.Flush()
	}
}
