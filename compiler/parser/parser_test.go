package parser_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/brimdata/zed/compiler"
	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/compiler/parser"
	"github.com/brimdata/zed/pkg/fs"
	"github.com/brimdata/zed/ztest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func searchForZqls() ([]string, error) {
	var zqls []string
	pattern := fmt.Sprintf(`.*ztests\%c.*\.yaml$`, filepath.Separator)
	re := regexp.MustCompile(pattern)
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, ".yaml") && re.MatchString(path) {
			zt, err := ztest.FromYAMLFile(path)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			z := zt.Zed
			if z == "" || z == "*" {
				return nil
			}
			zqls = append(zqls, z)
		}
		return err
	})
	return zqls, err
}

func parsePEGjs(z string) ([]byte, error) {
	cmd := exec.Command("node", "run.js", "-e", "start")
	cmd.Stdin = strings.NewReader(z)
	return cmd.Output()
}

func parseProc(z string) ([]byte, error) {
	proc, err := compiler.ParseProc(z)
	if err != nil {
		return nil, err
	}
	return json.Marshal(proc)
}

func parsePigeon(z string) ([]byte, error) {
	ast, err := parser.Parse("", []byte(z))
	if err != nil {
		return nil, err
	}
	return json.Marshal(ast)
}

// testZQL parses the zql query in line by both the Go and Javascript
// parsers.  It checks both that the parse is successful and that the
// two resulting ASTs are equivalent.  On the go side, we take a round
// trip through json marshal and unmarshal to turn the parse-tree types
// into generic JSON types.
func testZQL(t *testing.T, line string) {
	pigeonJSON, err := parsePigeon(line)
	assert.NoError(t, err, "parsePigeon: %q", line)

	astJSON, err := parseProc(line)
	assert.NoError(t, err, "parseProc: %q", line)

	assert.JSONEq(t, string(pigeonJSON), string(astJSON), "pigeon and ast.Proc mismatch: %q", line)

	if runtime.GOOS != "windows" {
		pegJSON, err := parsePEGjs(line)
		assert.NoError(t, err, "parsePEGjs: %q", line)
		assert.JSONEq(t, string(pigeonJSON), string(pegJSON), "pigeon and PEGjs mismatch: %q", line)
	}
}

func TestValid(t *testing.T) {
	file, err := fs.Open("valid.zed")
	require.NoError(t, err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		testZQL(t, string(line))
	}
}

func TestZtestZqls(t *testing.T) {
	zqls, err := searchForZqls()
	require.NoError(t, err)
	for _, z := range zqls {
		testZQL(t, z)
	}
}

func TestInvalid(t *testing.T) {
	file, err := fs.Open("invalid.zed")
	require.NoError(t, err)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, err := parser.Parse("", line)
		assert.Error(t, err, "zql: %q", line)
	}
}

// parseString is a helper for testing string parsing.  It wraps the
// given string in a simple zql query, parses it, and extracts the parsed
// string from inside the AST.
func parseString(in string) (string, error) {
	code := fmt.Sprintf("s = \"%s\"", in)
	tree, err := compiler.ParseProc(code)
	if err != nil {
		return "", err
	}
	if seq, ok := tree.(*ast.Sequential); ok {
		tree = seq.Procs[0]
	}
	filt, ok := tree.(*ast.Filter)
	if !ok {
		return "", fmt.Errorf("Expected Filter proc got %T", tree)
	}
	comp, ok := filt.Expr.(*ast.BinaryExpr)
	if !ok {
		return "", fmt.Errorf("Expected BinaryExpr got %T", filt.Expr)
	}
	p, ok := comp.RHS.(*zed.Primitive)
	if !ok {
		return "", fmt.Errorf("Expected Primitive got %T", filt.Expr)
	}
	return p.Text, nil
}

// Test handling of unicode escapes in the parser
func TestUnicode(t *testing.T) {
	result, err := parseString("Sacr\u00e9 bleu!")
	assert.NoError(t, err, "Parse of string succeeded")
	assert.Equal(t, result, "Sacré bleu!", "Unicode escape without brackets parsed correctly")

	result, err = parseString("I love \\u{1F32E}s")
	assert.NoError(t, err, "Parse of string succeeded")
	assert.Equal(t, result, "I love 🌮s", "Unicode escape with brackets parsed correctly")
}
