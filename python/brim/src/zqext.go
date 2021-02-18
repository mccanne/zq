package main

import "C"

import (
	"context"
	"errors"

	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/emitter"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zng/resolver"
)

// result converts an error into response structure expected
// by the Python calling code. cgo does not support exporting
// a function that returns a struct, hence the multiple return
// values.
// If C.CString is used to allocate a C char* string, the Python
// side code will free it.
func result(err error) (*C.char, bool) {
	if err != nil {
		return C.CString(err.Error()), false
	}
	return nil, true
}

// ErrorTest is only used to verify that errors are successfully passed
// between the Go & Python realms.
//
//export ErrorTest
func ErrorTest() (*C.char, bool) {
	return result(errors.New("error test"))
}

//export ZqlFileEval
func ZqlFileEval(inquery, inpath, informat, outpath, outformat string) (*C.char, bool) {
	return result(doZqlFileEval(inquery, inpath, informat, outpath, outformat))
}

func doZqlFileEval(inquery, inpath, informat, outpath, outformat string) (err error) {
	if inpath == "-" {
		inpath = "/dev/stdin"
	}
	if outpath == "-" {
		outpath = "/dev/stdout"
	}
	query, err := compiler.ParseProgram(inquery)
	if err != nil {
		return err
	}

	zctx := resolver.NewContext()
	rc, err := detector.OpenFile(zctx, inpath, zio.ReaderOpts{
		Format: informat,
	})
	if err != nil {
		return err
	}
	defer rc.Close()

	w, err := emitter.NewFile(context.Background(), outpath, zio.WriterOpts{
		Format: outformat,
	})
	if err != nil {
		return err
	}
	defer func() {
		closeErr := w.Close()
		if err == nil {
			err = closeErr
		}
	}()

	d := driver.NewCLI(w)
	return driver.Run(context.Background(), d, query, zctx, rc, driver.Config{})
}

func main() {}
