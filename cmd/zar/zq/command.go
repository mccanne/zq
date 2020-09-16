package zq

import (
	"flag"
	"os"

	"github.com/brimsec/zq/archive"
	"github.com/brimsec/zq/cli/zq"
	"github.com/brimsec/zq/cmd/zar/root"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/pkg/signalctx"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zql"
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
	stopErr     bool
	writerFlags zio.WriterFlags
	output      zq.OutputFlags
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	f.BoolVar(&c.quiet, "q", false, "don't display zql warnings")
	f.StringVar(&c.root, "R", os.Getenv("ZAR_ROOT"), "root directory of zar archive to walk")
	f.BoolVar(&c.stopErr, "e", true, "stop upon input errors")
	c.writerFlags.SetFlags(f)
	c.output.SetFlags(f)
	return c, nil
}

func (c *Command) Run(args []string) error {
	defer c.Cleanup()
	if ok, err := c.Init(); !ok {
		return err
	}
	opts := c.writerFlags.Options()
	if err := c.output.Init(&opts); err != nil {
		return err
	}

	if _, err := rlimit.RaiseOpenFilesLimit(); err != nil {
		return err
	}

	query, err := zql.ParseProc(args[0])
	if err != nil {
		return err
	}

	ark, err := archive.OpenArchive(c.root, nil)
	if err != nil {
		return err
	}
	msrc := archive.NewMultiSource(ark, args[1:])

	ctx, cancel := signalctx.New(os.Interrupt)
	defer cancel()

	writer, err := c.output.Open(opts)
	if err != nil {
		return err
	}
	d := driver.NewCLI(writer)
	if !c.quiet {
		d.SetWarningsWriter(os.Stderr)
	}
	err = driver.MultiRun(ctx, d, query, resolver.NewContext(), msrc, driver.MultiConfig{})
	if closeErr := writer.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}
