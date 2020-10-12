package multisource

import (
	"context"
	"io"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/filter"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/scanner"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zqd/api"
)

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
	SendSources(context.Context, *resolver.Context, SourceFilter, chan Source) error

	OpenSource(context.Context, Source) (ScannerCloser, error)

	SourceFromRequest(api.WorkerRequest) (Source, error)
}

// A Source is a closure sent by a MultiSource to provide scanning
// access to a single source. It may return a nil ScannerCloser, in the
// case that it represents a logically empty source.
//type Source func() (ScannerCloser, error)
type Source interface {
	ToWorkerRequest() api.WorkerRequest
	String() string // dev/test
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
