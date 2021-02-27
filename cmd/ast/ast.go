package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os/exec"
	"strings"

	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/compiler/ast"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/zfmt"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zql"
	"github.com/mccanne/charm"
	"github.com/peterh/liner"
)

var Ast = &charm.Spec{
	Name:  "ast",
	Usage: "ast [ options ] zql",
	Short: "tool for inspecting zql abtract-syntax trees",
	Long: `
The ast command parses a zql expression and prints the resulting abstract-syntax
tree as JSON object to standard output.  This serves a tool for dev and test
but could also be used by power users trying to understand how zql syntax is
translated into the analytics requests that is sent to the zqd search endpoint.

By default, it runs the built-in PEG parser built into this go binary.
If you specify -js, it will try to run a javascript version of the parser
by execing node in the currrent directory running the javascript in ./zql/run.js.

The -O flag is handy for turning on and off the compiler, which lets you see
how the parsed AST is transformed into a runtime object comprised of the
Z kernel operators.
`,
	New: func(parent charm.Command, flags *flag.FlagSet) (charm.Command, error) {
		return New(flags)
	},
}

func init() {
	Ast.Add(charm.Help)
}

type Command struct {
	repl     bool
	js       bool
	pigeon   bool
	proc     bool
	canon    bool
	filter   bool
	semantic bool
	optimize bool
	parallel int
	sortKey  string
	sortRev  bool
	n        int
	includes includes
}

func New(f *flag.FlagSet) (charm.Command, error) {
	c := &Command{}
	f.BoolVar(&c.repl, "repl", false, "enter repl")
	f.BoolVar(&c.js, "js", false, "run javascript version of peg parser")
	f.BoolVar(&c.pigeon, "pigeon", false, "run pigeon version of peg parser")
	f.BoolVar(&c.proc, "proc", false, "run pigeon version of peg parser and marshal into ast.Proc")
	f.BoolVar(&c.semantic, "s", false, "display semantically analyzed AST (implies -proc)")
	f.BoolVar(&c.optimize, "O", false, "display optimized, non-filter AST (implies -proc)")
	f.BoolVar(&c.filter, "f", false, "display AST of lifted filter (implies -proc)")
	f.IntVar(&c.parallel, "P", 0, "display parallelized AST (implies -proc)")
	f.StringVar(&c.sortKey, "sortKey", "", "input field for expected sorted data")
	f.BoolVar(&c.sortRev, "sortRev", false, "true if input is expected in reverse order")
	f.BoolVar(&c.canon, "C", false, "display AST in Z canonical format (implies -proc)")
	f.Var(&c.includes, "I", "source file containing Z query text (may be repeated)")
	return c, nil
}

type includes []string

func (i includes) String() string {
	return strings.Join(i, ",")
}

func (i *includes) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (c *Command) Run(args []string) error {
	if len(args) == 0 && len(c.includes) == 0 {
		return Ast.Exec(c, []string{"help"})
	}
	c.n = 0
	if c.js {
		c.n++
	}
	if c.pigeon {
		c.n++
	}
	if c.proc {
		c.n++
	}
	if c.semantic {
		c.n++
	}
	if c.optimize {
		c.n++
	}
	if c.filter {
		c.n++
	}
	if c.parallel > 0 {
		c.n++
	}
	if c.n == 0 {
		if c.canon {
			c.proc = true
		} else {
			c.pigeon = true
		}
	}
	if c.repl {
		c.interactive()
		return nil
	}
	var src string
	if len(c.includes) > 0 {
		for _, path := range c.includes {
			b, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			src += "\n" + string(b)
		}
	}
	src += strings.Join(args, " ")
	return c.parse(src)
}

func (c *Command) header(msg string) {
	bars := strings.Repeat("=", len(msg))
	if c.n > 1 {
		fmt.Printf("/%s\\\n", bars)
		fmt.Printf("|%s|\n", msg)
		fmt.Printf("\\%s/\n", bars)
	}
}

func (c *Command) parse(z string) error {
	if c.js {
		s, err := parsePEGjs(z)
		if err != nil {
			return err
		}
		c.header("pegjs")
		fmt.Println(s)
	}
	if c.pigeon {
		s, err := parsePigeon(z)
		if err != nil {
			return err
		}
		c.header("pigeon")
		fmt.Println(s)
	}
	if c.proc {
		p, err := compiler.ParseProc(z)
		if err != nil {
			return err
		}
		c.header("proc")
		c.writeProc(p)

	}
	if c.semantic {
		runtime, err := c.compile(z)
		if err != nil {
			return err
		}
		c.header("semantic")
		c.writeProc(runtime.Entry())
	}
	if c.filter {
		runtime, err := c.compile(z)
		if err != nil {
			return err
		}
		if err := runtime.Optimize(); err != nil {
			return err
		}
		c.header("lifted filter")
		c.writeProc(runtime.AsProc())
	}
	if c.optimize {
		runtime, err := c.compile(z)
		if err != nil {
			return err
		}
		if err := runtime.Optimize(); err != nil {
			return err
		}
		c.header("optimized")
		c.writeProc(runtime.Entry())
	}
	if c.parallel > 0 {
		runtime, err := c.compile(z)
		if err != nil {
			return err
		}
		if err := runtime.Optimize(); err != nil {
			return err
		}
		if ok := runtime.Parallelize(c.parallel); !ok {
			return errors.New("parallellize failed")
		}
		c.header("parallelized")
		c.writeProc(runtime.Entry())
	}
	return nil
}

func (c *Command) writeProc(p ast.Proc) {
	s, err := procFmt(p, c.canon)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(s)
	}
}

func (c *Command) compile(z string) (*compiler.Runtime, error) {
	proc, err := compiler.ParseProc(z)
	if err != nil {
		return nil, err
	}
	if c.sortKey == "" {
		return compiler.New(resolver.NewContext(), proc)
	}
	sortKey := field.Dotted(c.sortKey)
	return compiler.NewWithSortedInput(resolver.NewContext(), proc, sortKey, c.sortRev)
}

const nodeProblem = `
Failed to run node on ./zql/run.js.  The "-js" flag is for PEG
development and should only be used when running ast in the root
directory of the zq repo.`

func (c *Command) interactive() {
	rl := liner.NewLiner()
	defer rl.Close()
	for {
		line, err := rl.Prompt("> ")
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		rl.AppendHistory(line)
		if err := c.parse(line); err != nil {
			log.Println(err)
		}
	}
}

func runNode(dir, line string) ([]byte, error) {
	cmd := exec.Command("node", "./zql/run.js", "-e", "start")
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = strings.NewReader(line)
	return cmd.Output()
}

func normalize(b []byte) (string, error) {
	var v interface{}
	err := json.Unmarshal(b, &v)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(v, "", "    ")
	return string(out), err
}

func parsePEGjs(z string) (string, error) {
	b, err := runNode("", z)
	if err != nil {
		// parse errors don't cause this... this is only
		// caused by a problem running node.
		return "", errors.New(strings.TrimSpace(nodeProblem))
	}
	return normalize(b)
}

func procFmt(proc ast.Proc, canon bool) (string, error) {
	if canon {
		return zfmt.Canonical(proc), nil
	}
	procJSON, err := json.Marshal(proc)
	if err != nil {
		return "", err
	}
	return normalize(procJSON)
}

func parsePigeon(z string) (string, error) {
	ast, err := zql.Parse("", []byte(z))
	if err != nil {
		return "", err
	}
	goPEGJSON, err := json.Marshal(ast)
	if err != nil {
		return "", errors.New("go peg parser returned bad value for: " + z)
	}
	return normalize(goPEGJSON)
}
