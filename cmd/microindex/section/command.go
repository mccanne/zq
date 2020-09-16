package section

import (
	"errors"
	"flag"
	"os"

	"github.com/brimsec/zq/cli/zq"
	"github.com/brimsec/zq/cmd/microindex/root"
	"github.com/brimsec/zq/microindex"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/mccanne/charm"
	"golang.org/x/crypto/ssh/terminal"
)

var Section = &charm.Spec{
	Name:  "section",
	Usage: "section [flags] path",
	Short: "extract a section of a microindex file",
	Long: `
The section command extracts a section from a microindex file and
writes it to the output.  The -trailer option writes
the microindex trailer to the output in addition to the section if the section
number was specified.

See the microindex command help for a description of a microindex.`,
	New: newCommand,
}

func init() {
	root.MicroIndex.Add(Section)
}

type Command struct {
	*root.Command
	writerFlags zio.WriterFlags
	output      zq.OutputFlags
	trailer     bool
	section     int
}

func newCommand(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	f.BoolVar(&c.trailer, "trailer", false, "include the micro-index trailer in the output")
	f.IntVar(&c.section, "s", -1, "include the indicated section in the output")
	c.writerFlags.SetFlags(f)
	c.output.SetFlags(f)
	return c, nil
}

func isTerminal(f *os.File) bool {
	return terminal.IsTerminal(int(f.Fd()))
}

func (c *Command) Run(args []string) error {
	defer c.Cleanup()
	if ok, err := c.Init(); !ok {
		return err
	}
	if len(args) != 1 {
		return errors.New("microindex section: must be run with a single path argument")
	}
	opts := c.writerFlags.Options()
	if err := c.output.Init(&opts); err != nil {
		return err
	}
	path := args[0]
	reader, err := microindex.NewReader(resolver.NewContext(), path)
	if err != nil {
		return err
	}
	defer reader.Close()
	writer, err := c.output.Open(opts)
	if err != nil {
		return err
	}
	defer func() {
		if writer != nil {
			writer.Close()
		}
	}()
	if c.section >= 0 {
		r, err := reader.NewSectionReader(c.section)
		if err != nil {
			return err
		}
		if err := zbuf.Copy(writer, r); err != nil {
			return err
		}
	}
	if c.trailer {
		r, err := reader.NewTrailerReader()
		if err != nil {
			return err
		}
		if err := zbuf.Copy(writer, r); err != nil {
			return err
		}
	}
	err = writer.Close()
	writer = nil
	return err
}
