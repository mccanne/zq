package root

import (
	"flag"
	"log"
	"strings"

	"github.com/brimsec/zq/cli/zq"
	"github.com/mccanne/charm"
)

var Zqd = &charm.Spec{
	Name:  "zqd",
	Usage: "zqd [global options] command [options] [arguments...]",
	Short: "use zqd to server zq searches",
	Long: `
`,
	New: New,
}

type Command struct {
	charm.Command
	cli zq.Flags
}

func init() {
	Zqd.Add(charm.Help)
}

func Servers(s string) []string {
	return strings.Split(s, ",")
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{}
	log.SetPrefix("zqd") // XXX switch to zapper
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
	return Zqd.Exec(c, []string{"help"})
}
