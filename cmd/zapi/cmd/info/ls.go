package info

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/cmd/zapi/cmd"
	"github.com/brimsec/zq/pkg/colw"
	"github.com/mccanne/charm"
	"github.com/mccanne/charm/pkg/termwidth"
)

var Ls = &charm.Spec{
	Name:  "ls",
	Usage: "ls [-l] [glob1 glob2 ...]",
	Short: "list spaces or information about a space",
	Long: `The ls command lists the names and information about spaces known to the system.
When run with arguments, only the spaces that match the glob-style parameters are shown
much like the traditional unix ls command.  When used with "-l", each space specified
is listed with its detailed information as in the info command.`,
	New: NewLs,
}

type LsCommand struct {
	*cmd.Command
	lflag bool
}

func NewLs(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &LsCommand{Command: parent.(*cmd.Command)}
	f.BoolVar(&c.lflag, "l", false, "show detail information about each space listed")
	return c, nil
}

// Run lists all spaces in the current zqd host or if a parameter
// is provided (in glob style) lists the info about that space.
func (c *LsCommand) Run(args []string) error {
	conn := c.Connection()
	matches, err := cmd.SpaceGlob(c.Context(), conn, args...)
	if err != nil {
		if err == cmd.ErrNoSpacesExist {
			fmt.Println("no spaces exist")
			return nil
		}
		return err
	}
	if len(matches) == 0 {
		return cmd.ErrNoMatch
	}
	if c.lflag {
		// print details about each space
		return printSpaceSummaries(matches)
	}
	names := cmd.SpaceNames(matches)
	width := termwidth.Width()
	// print listing laid out in columns like ls
	err = colw.Write(os.Stdout, names, width, 3)
	if err == colw.ErrDoesNotFit {
		fmt.Println(strings.Join(names, "\n"))
	}
	return err
}

func printSpaceSummaries(sl []api.Space) error {
	for _, space := range sl {
		if err := printSpace(space.Name, space); err != nil {
			return err
		}
	}
	return nil
}
