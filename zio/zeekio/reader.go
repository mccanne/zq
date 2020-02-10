package zeekio

import (
	"bytes"
	"fmt"
	"io"

	"github.com/brimsec/zq/pkg/peeker"
	"github.com/brimsec/zq/pkg/skim"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
)

const (
	ReadSize    = 64 * 1024
	MaxLineSize = 50 * 1024 * 1024
)

type Reader struct {
	scanner *skim.Scanner
	peeker  *peeker.Reader
	parser  *Parser
	zctx    *resolver.Context
}

func NewReader(reader io.Reader, zctx *resolver.Context) *Reader {
	// XXX LookupTypeAlias() needs to be fallible in case this alias
	// already exists.  Which means that NewReader() also needs to
	// be fallible, ugh.
	_ = zctx.LookupTypeAlias("zenum", zng.TypeString)

	buffer := make([]byte, ReadSize)
	return &Reader{
		scanner: skim.NewScanner(reader, buffer, MaxLineSize),
		parser:  NewParser(zctx),
	}
}

func (r *Reader) Read() (*zng.Record, error) {
again:
	line, err := r.scanner.ScanLine()
	if line == nil {
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", r.scanner.Stats.Lines, err)
		}
		return nil, nil
	}
	// remove newline
	line = bytes.TrimSpace(line)
	if line[0] == '#' {

		if err := r.parser.ParseDirective(line); err != nil {
			return nil, err
		}
		goto again
	}
	return r.parser.ParseValue(line)
}
