package root

import (
	"flag"

	"github.com/brimsec/zq/cli/zq"
	"github.com/mccanne/charm"
)

var Zar = &charm.Spec{
	Name:  "zar",
	Usage: "zar [global options] command [options] [arguments...]",
	Short: "create and search zng archives",
	Long: `
zar operates on collections of ZNG files partitioned by time and stored either
on a filesystem or an S3 compatible object store. An individual
item of data (a file or object) is called a chunk, and each chunk may have
other named ZNG files associated with it, stored "near" to the chunk. For
filesystem archives, the associated files are stored in a directory next
to the chunk file.

An example of a chunk associated file is a micro-index: a ZNG file that holds
keyed records and supports very fast lookup of keys. When the key represents
a value in the associated chunk file, micro-indexes can be used to to make
searching an archive very fast.

See the zar README in the zq github repo for more information:
https://github.com/brimsec/zq/blob/master/cmd/zar/README.md
`,
	New: New,
}

type Command struct {
	charm.Command
	cli zq.Flags
}

func init() {
	Zar.Add(charm.Help)
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{}
	c.cli.SetFlags(f)
	return c, nil
}

func (c *Command) Cleanup() {
	c.cli.Cleanup()
}

func (c *Command) Init() (bool, error) {
	return c.cli.Init()
}

func (c *Command) Run(args []string) error {
	defer c.cli.Cleanup()
	if ok, err := c.cli.Init(); !ok {
		return err
	}
	return Zar.Exec(c, []string{"help"})
}
