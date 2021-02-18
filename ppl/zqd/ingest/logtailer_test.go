package ingest

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/suite"
)

var sortTs = compiler.MustParseProc("sort ts")

const expected = `#0:record[ts:time]
0:[0;]
0:[1;]
0:[2;]
0:[3;]
0:[4;]
0:[5;]
0:[6;]
0:[7;]
0:[8;]
0:[9;]
0:[10;]
0:[11;]
0:[12;]
0:[13;]
0:[14;]
0:[15;]
0:[16;]
0:[17;]
0:[18;]
0:[19;]
`

type logTailerTSuite struct {
	suite.Suite
	dir  string
	zctx *resolver.Context
	dr   *logTailer
}

func TestLogTailer(t *testing.T) {
	suite.Run(t, new(logTailerTSuite))
}

func (s *logTailerTSuite) SetupTest() {
	dir, err := ioutil.TempDir("", "TestLogTailer")
	s.Require().NoError(err)
	s.dir = dir
	s.T().Cleanup(func() { os.RemoveAll(s.dir) })
	s.zctx = resolver.NewContext()
	s.dr, err = newLogTailer(s.zctx, s.dir, zio.ReaderOpts{Format: "tzng"})
	s.Require().NoError(err)
}

func (s *logTailerTSuite) TestCreatedFiles() {
	result, errCh := s.read()
	f1 := s.createFile("test1.tzng")
	f2 := s.createFile("test2.tzng")
	s.write(f1, f2)
	s.Require().NoError(<-errCh)
	s.Equal(expected, <-result)
}

func (s *logTailerTSuite) TestIgnoreDir() {
	result, errCh := s.read()
	f1 := s.createFile("test1.tzng")
	f2 := s.createFile("test2.tzng")
	err := os.Mkdir(filepath.Join(s.dir, "testdir"), 0755)
	s.Require().NoError(err)
	s.write(f1, f2)
	s.Require().NoError(<-errCh)
	s.Equal(expected, <-result)
}

func (s *logTailerTSuite) TestExistingFiles() {
	f1 := s.createFile("test1.tzng")
	f2 := s.createFile("test2.tzng")
	result, errCh := s.read()
	s.write(f1, f2)
	s.Require().NoError(<-errCh)
	s.Equal(expected, <-result)
}

func (s *logTailerTSuite) TestInvalidFile() {
	_, errCh := s.read()
	f1 := s.createFile("test1.tzng")
	_, err := f1.WriteString("#0:record[ts:time]\n")
	s.Require().NoError(err)
	_, err = f1.WriteString("this is an invalid line\n")
	s.Require().NoError(err)
	s.Require().NoError(f1.Sync())
	s.EqualError(<-errCh, "line 2: bad format")
	s.NoError(s.dr.Stop())
}

func (s *logTailerTSuite) TestEmptyFile() {
	result, errCh := s.read()
	f1 := s.createFile("test1.tzng")
	_ = s.createFile("test2.tzng")
	s.write(f1)
	s.Require().NoError(<-errCh)
	s.Equal(expected, <-result)
}

func (s *logTailerTSuite) createFile(name string) *os.File {
	f, err := os.Create(filepath.Join(s.dir, name))
	s.Require().NoError(err)
	// Call sync to ensure fs events are sent in a timely matter.
	s.Require().NoError(f.Sync())
	return f
}

func (s *logTailerTSuite) read() (<-chan string, <-chan error) {
	result := make(chan string)
	errCh := make(chan error)
	buf := bytes.NewBuffer(nil)
	w := tzngio.NewWriter(zio.NopCloser(buf))
	go func() {
		program := &ast.Program{Entry: sortTs}
		err := driver.Copy(context.Background(), w, program, s.zctx, s.dr, driver.Config{})
		if err != nil {
			close(result)
			errCh <- err
		} else {
			close(errCh)
			result <- buf.String()
		}
	}()
	return result, errCh
}

func (s *logTailerTSuite) write(files ...*os.File) {
	for _, f := range files {
		_, err := f.WriteString("#0:record[ts:time]\n")
		s.Require().NoError(err)
	}
	for i := 0; i < 20; {
		for _, f := range files {
			_, err := f.WriteString(fmt.Sprintf("0:[%d;]\n", i))
			s.Require().NoError(err)
			i++
		}
	}
	// Need to sync here as on windows the fsnotify event is not triggered
	// unless this is done. Presumably this happens in cases when not enough
	// data has been written so the system has not flushed the file buffer to disk.
	for _, f := range files {
		s.Require().NoError(f.Sync())
	}
	s.Require().NoError(s.dr.Stop())
}
