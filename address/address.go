package address

import (
	"context"
	"io"
	"time"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/filter"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/proc/compiler"
	"github.com/brimsec/zq/scanner"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/segmentio/ksuid"
	"go.uber.org/zap"
)

type SpanInfo interface {
	GetSpan() nano.Span
	GetChunks() []Chunk
	// SourceOpener provides scanning access to a single source.
	// It works for locals files or S3 (not for accessing zqd/worker).
	// It may return a nil ScannerCloser, in the
	// case that it represents a logically empty source.
	SourceOpener() (ScannerCloser, error)
}

type Chunk interface {
	GetId() ksuid.KSUID
	GetFirst() nano.Ts
	GetLast() nano.Ts
	GetDataFileKind() string
	GetRecordCount() int
}

// A MultiSource is a set of one or more ZNG record sources, which could be
// a zar archive, a local directory, a collection of remote objects, etc.
type MultiSource interface {
	// OrderInfo reports if the same order exists on the data in any
	// single source and on the sources reported via SendSources.
	OrderInfo() (field string, reversed bool)

	// SendSources sends SourceOpeners to the given channel. If the
	// MultiSource declares an ordering via its OrderInfo method, the
	// SourceOpeners must be sent in the same order.
	// The MultiSource should return a nil error when all of its sources
	// have been sent; it must not close the provided channel.
	// SendSources is called from a single goroutine, but the SourceOpeners
	// it generates will be used from potentially many goroutines. For best
	// performance, SendSources should perform quick filtering that performs
	// little or no i/o, and let the returned ScannerCloser perform more intensive
	// filtering (e.g., reading a micro-index to check for filter matching).
	SendSources(context.Context, *resolver.Context, SourceFilter, chan SpanInfo) error
	GetAltPaths() []string
}

type ScannerCloser interface {
	scanner.Scanner
	io.Closer
}

type SourceFilter struct {
	Filter     filter.Filter
	FilterExpr ast.BooleanExpr
	Span       nano.Span
}

type MultiConfig struct {
	Custom      compiler.Hook
	Logger      *zap.Logger
	Parallelism int
	Span        nano.Span
	StatsTick   <-chan time.Time
	Warnings    chan string
}
