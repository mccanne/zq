package info

import (
	"flag"

	"github.com/brimdata/zed/api"
	"github.com/brimdata/zed/cli/outputflags"
	apicmd "github.com/brimdata/zed/cmd/zed/api"
	"github.com/brimdata/zed/pkg/charm"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

var Ls = &charm.Spec{
	Name:  "ls",
	Usage: "ls [glob1 glob2 ...]",
	Short: "list pools or information about a pool",
	Long: `The ls command lists the names and information about pools known to the system.
When run with arguments, only the pools that match the glob-style parameters are shown
much like the traditional unix ls command.`,
	New: NewLs,
}

type LsCommand struct {
	*apicmd.Command
	lflag       bool
	outputFlags outputflags.Flags
}

func NewLs(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &LsCommand{Command: parent.(*apicmd.Command)}
	f.BoolVar(&c.lflag, "l", false, "output full information for each pool")
	c.outputFlags.DefaultFormat = "text"
	c.outputFlags.SetFormatFlags(f)
	return c, nil
}

// Run lists all pools in the current zqd host or if a parameter
// is provided (in glob style) lists the info about that pool.
func (c *LsCommand) Run(args []string) error {
	ctx, cleanup, err := c.Init(&c.outputFlags)
	if err != nil {
		return err
	}
	defer cleanup()
	matches, err := apicmd.PoolGlob(ctx, c.Conn, args...)
	if err != nil {
		if err == apicmd.ErrNoPoolsExist {
			return nil
		}
		return err
	}
	if len(matches) == 0 {
		return apicmd.ErrNoMatch
	}
	if c.lflag {
		return apicmd.WriteOutput(ctx, c.outputFlags, newPoolReader(matches))
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m.Name)
	}
	return apicmd.WriteOutput(ctx, c.outputFlags, apicmd.NewNameReader(names))
}

type poolReader struct {
	idx   int
	mc    *zson.MarshalZNGContext
	pools []api.Pool
}

func newPoolReader(pools []api.Pool) *poolReader {
	return &poolReader{
		pools: pools,
	}
}

func (r *poolReader) Read() (*zng.Record, error) {
	if r.idx >= len(r.pools) {
		return nil, nil
	}
	rec, err := r.mc.MarshalRecord(r.pools[r.idx])
	if err != nil {
		return nil, err
	}
	r.idx++
	return rec, nil
}
