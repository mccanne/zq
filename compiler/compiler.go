package compiler

import (
	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/zql"
)

// ParseProc() is an entry point for use from external go code,
// mostly just a wrapper around Parse() that casts the return value.
//func ParseProc(query string, opts ...zql.Option) (ast.Proc, error) {
func ParseProc(query string) (ast.Proc, error) {
	parsed, err := zql.Parse("", []byte(query), zql.Entrypoint("start"))
	if err != nil {
		return nil, err
	}
	return ast.UnpackProc(nil, parsed)
}

func ParseExpression(expr string) (ast.Expression, error) {
	parsed, err := zql.Parse("", []byte(expr), zql.Entrypoint("Expr"))
	if err != nil {
		return nil, err
	}
	return ast.UnpackExpression(nil, parsed)
}

func ParseProgram(z string) (*ast.Program, error) {
	parsed, err := zql.Parse("", []byte(z), zql.Entrypoint("Program"))
	if err != nil {
		proc, nerr := ParseProc(z)
		if nerr != nil {
			return nil, err
		}
		return &ast.Program{Entry: proc}, nil
	}
	return ast.UnpackProgram(nil, parsed)
}

func ParseToObject(expr, entry string) (interface{}, error) {
	return zql.Parse("", []byte(expr), zql.Entrypoint(entry))
}

// MustParseProc is functionally the same as ParseProc but panics if an error
// is encountered.
func MustParseProc(query string) ast.Proc {
	proc, err := ParseProc(query)
	if err != nil {
		panic(err)
	}
	return proc
}

func MustParseProgram(query string) *ast.Program {
	p, err := ParseProgram(query)
	if err != nil {
		if proc, err := ParseProc(query); err == nil {
			return &ast.Program{Entry: proc}
		}
		panic(err)
	}
	return p
}
