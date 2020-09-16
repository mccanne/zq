package lookup

import (
	"context"
	"errors"
	"flag"
	"strings"

	"github.com/brimsec/zq/cli/zq"
	"github.com/brimsec/zq/cmd/microindex/root"
	"github.com/brimsec/zq/microindex"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/mccanne/charm"
)

var Lookup = &charm.Spec{
	Name:  "lookup",
	Usage: "lookup -k key[,key...] index",
	Short: "lookup a key in a microindex file and print value as zng record",
	Long: `
The lookup command locates the specified key(s) in the base layer of a
microindex and displays the result as a zng record.
If the index has multiple keys, then multiple records may be returned for
all the records that match the supplied keys.
Each key argument specifies a value to look up in the table and must be parseable
as the zng type of the key that was originally indexed where the keys refer to the leaf
values in left-to-right order of the keys represented as a record, inclusive
of any nested records.`,
	New: newLookupCommand,
}

func init() {
	root.MicroIndex.Add(Lookup)
}

type LookupCommand struct {
	*root.Command
	keys        string
	WriterFlags zio.WriterFlags
	closest     bool
	output      zq.OutputFlags
}

func newLookupCommand(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &LookupCommand{Command: parent.(*root.Command)}
	f.StringVar(&c.keys, "k", "", "key(s) to search")
	f.BoolVar(&c.closest, "c", false, "find closest insead of exact match")
	c.WriterFlags.SetFlags(f)
	c.output.SetFlags(f)
	return c, nil
}

func (c *LookupCommand) Run(args []string) error {
	defer c.Cleanup()
	if ok, err := c.Init(); !ok {
		return err
	}
	if len(args) != 1 {
		return errors.New("microindex lookup: must be run with a single file argument")
	}
	opts := c.WriterFlags.Options()
	if err := c.output.Init(&opts); err != nil {
		return err
	}
	path := args[0]
	if c.keys == "" {
		return errors.New("must specify one or more comma-separated keys")
	}
	uri, err := iosrc.ParseURI(path)
	if err != nil {
		return err
	}
	finder := microindex.NewFinder(resolver.NewContext(), uri)
	if err := finder.Open(); err != nil {
		return err
	}
	defer finder.Close()
	keys, err := finder.ParseKeys(strings.Split(c.keys, ","))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hits := make(chan *zng.Record)
	var searchErr error
	go func() {
		if c.closest {
			var rec *zng.Record
			rec, searchErr = finder.LookupClosest(keys)
			if rec != nil {
				hits <- rec
			}
		} else {
			searchErr = finder.LookupAll(ctx, hits, keys)
		}
		close(hits)
	}()
	writer, err := c.output.Open(opts)
	if err != nil {
		return err
	}
	for hit := range hits {
		if err := writer.Write(hit); err != nil {
			writer.Close()
			return err
		}
	}
	err = writer.Close()
	if searchErr != nil {
		err = searchErr
	}
	return err
}
