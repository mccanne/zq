package info

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"text/tabwriter"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/cmd/zapi/cmd"
	"github.com/brimsec/zq/cmd/zapi/format"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/mccanne/charm"
)

var Info = &charm.Spec{
	Name:  "info",
	Usage: "info [spacename]",
	Short: "show information about a space",
	Long: `The info command displays the configuration settings and other information
about the currently selected space.`,
	New: New,
}

func init() {
	cmd.CLI.Add(Info)
	cmd.CLI.Add(Ls)
}

type Command struct {
	*cmd.Command
}

func New(parent charm.Command, flags *flag.FlagSet) (charm.Command, error) {
	return &Command{Command: parent.(*cmd.Command)}, nil
}

// Run lists all spaces in the current zqd host or if a parameter
// is provided (in glob style) lists the info about that space.
func (c *Command) Run(args []string) error {
	conn := c.Connection()
	var ids []api.SpaceID
	if len(args) > 0 {
		matches, err := cmd.SpaceGlob(c.Context(), conn, args...)
		if err != nil {
			return err
		}
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
	} else {
		id, err := c.SpaceID()
		if err == cmd.ErrSpaceNotSpecified {
			return errors.New("no space provided")
		}
		if err != nil {
			return err
		}
		ids = []api.SpaceID{id}
	}
	for _, id := range ids {
		info, err := conn.SpaceInfo(c.Context(), id)
		if err != nil {
			return err
		}
		if err := printSpace(info.Name, *info); err != nil {
			return err
		}
	}
	return nil
}

func printIface(w io.Writer, iface interface{}) {
	infoVal := reflect.ValueOf(iface)
	for i := 0; i < infoVal.NumField(); i++ {
		v := infoVal.Field(i)
		t := infoVal.Type().Field(i)
		name := cmd.JSONName(t)
		if v.Kind() == reflect.Ptr && v.IsNil() {
			fmt.Fprintf(w, "  %s:\t%v\n", name, nil)
			continue
		}
		if v.Kind() == reflect.Struct && t.Anonymous {
			printIface(w, v.Interface())
			continue
		}
		v = reflect.Indirect(v)
		vi := v.Interface()
		switch t.Tag.Get("unit") {
		case "bytes":
			vi = format.Bytes(v.Int())
		case "":
			if v.Type() == reflect.TypeOf(nano.Ts(0)) {
				vi = nano.Ts(v.Int()).Time()
			}
		}
		fmt.Fprintf(w, "  %s:\t%v\n", name, vi)
	}
}

func printSpace(name string, iface interface{}) error {
	fmt.Println(name)
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 1, ' ', 0)
	printIface(w, iface)
	return w.Flush()
}
