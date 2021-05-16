package api

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/brimdata/zed/api/client"
	"github.com/brimdata/zed/cli"
	"github.com/brimdata/zed/cli/outputflags"
	"github.com/brimdata/zed/cmd/zed/root"
	"github.com/brimdata/zed/pkg/charm"
	"github.com/brimdata/zed/pkg/storage"
	"github.com/brimdata/zed/pkg/terminal"
	"github.com/brimdata/zed/zio"
	"github.com/segmentio/ksuid"
)

var Get *charm.Spec

var Cmd = &charm.Spec{
	Name:  "api",
	Usage: "api [global options] command [options] [arguments...]",
	Short: "issue commands to zed lake service",
	Long: `
The "zed api" command-line tool is used to talk to a zed lake service endpoint.
This service could be "zed server" running on your laptop or in the cloud.
If you have installed the shortcuts,
"zapi" (prounounced "zappy") is a shortcut for the "zed api" command.

With "zed api" you can create pools, list pools, post data to pools, and run queries.

The Brim application and the "zed api" client use the same REST API
for interacting with a zed lake.
`,
	New: New,
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	// If not a terminal make nofancy on by default.
	c.NoFancy = !terminal.IsTerminalFile(os.Stdout)
	defaultHost := "localhost:9867"
	f.StringVar(&c.Host, "host", defaultHost, "host[:port]")
	f.StringVar(&c.PoolName, "p", c.PoolName, "pool")
	f.BoolVar(&c.NoFancy, "nofancy", c.NoFancy, "disable fancy CLI output (true if stdout is not a tty)")
	c.LocalConfig.SetFlags(f)
	return c, nil
}

type Command struct {
	*root.Command
	Conn        *client.Connection
	Host        string
	LocalConfig LocalConfigFlags
	NoFancy     bool
	PoolName    string
	PoolID      ksuid.KSUID
	cli         cli.Flags
}

func (c *Command) Init(all ...cli.Initializer) (context.Context, func(), error) {
	ctx, cleanup, err := c.Command.Init(all...)
	if err != nil {
		return nil, nil, err
	}
	if err := c.Login(); err != nil {
		return nil, nil, err
	}
	// XXX This will go away once we do the work to make zapi and zed lake use
	// the same charm commands.
	if c.PoolName != "" {
		c.PoolID, err = LookupPoolID(ctx, c.Conn, c.PoolName)
	}
	return ctx, cleanup, nil
}

// Run is called by charm when there are no sub-commands on the main
// zqd command line.
func (c *Command) Run(args []string) error {
	_, cleanup, err := c.Init()
	if err != nil {
		return err
	}
	defer cleanup()
	if len(args) == 0 {
		return charm.NeedHelp
	}
	return charm.ErrNoRun
}

func (c *Command) Login() error {
	if _, _, err := net.SplitHostPort(c.Host); err == nil {
		c.Host = "http://" + c.Host
	}
	c.Conn = client.NewConnectionTo(c.Host)
	creds, err := c.LocalConfig.LoadCredentials()
	if err != nil {
		return err
	}
	if tokens, ok := creds.ServiceTokens(c.Host); ok {
		c.Conn.SetAuthToken(tokens.Access)
	}
	return nil
}

func (c *Command) Prompt() string {
	return c.PoolName + "> "
}

func Errorf(spec string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, Cmd.Name+": "+spec, args...)
}

func WriteOutput(ctx context.Context, flags outputflags.Flags, r zio.Reader) error {
	wc, err := flags.Open(ctx, storage.NewLocalEngine())
	if err != nil {
		return err
	}
	err = zio.CopyWithContext(ctx, wc, r)
	if closeErr := wc.Close(); err == nil {
		err = closeErr
	}
	return err
}
