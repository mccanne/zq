package driver

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/zqd/worker"
	"github.com/brimsec/zq/proc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng/resolver"
	"go.uber.org/zap"
)

// XXX ReaderSortKey should be a field.Static.  Issue #1467.
type Config struct {
	Custom            compiler.Hook
	Logger            *zap.Logger
	ReaderSortKey     string
	ReaderSortReverse bool
	Span              nano.Span
	StatsTick         <-chan time.Time
}

type scannerProc struct {
	zbuf.Scanner
}

func (s *scannerProc) Done() {}

type namedScanner struct {
	zbuf.Scanner
	name string
}

func (n *namedScanner) Pull() (zbuf.Batch, error) {
	b, err := n.Scanner.Pull()
	if err != nil {
		err = fmt.Errorf("%s: %w", n.name, err)
	}
	return b, err
}

func compile(ctx context.Context, p *ast.Program, zctx *resolver.Context, readers []zbuf.Reader, cfg Config) (*muxOutput, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Span.Dur == 0 {
		cfg.Span = nano.MaxSpan
	}
	pctx := &proc.Context{
		Context:     ctx,
		TypeContext: zctx,
		Logger:      cfg.Logger,
		Warnings:    make(chan string, 5),
	}
	program := compiler.NewProgram(p, pctx)
	if err := program.Optimize(field.Dotted(cfg.ReaderSortKey), cfg.ReaderSortReverse); err != nil {
		return nil, err
	}
	procs := make([]proc.Interface, 0, len(readers))
	scanners := make([]zbuf.Scanner, 0, len(readers))
	for _, r := range readers {
		sn, err := zbuf.NewScanner(ctx, r, program.Filter(), cfg.Span)
		if err != nil {
			return nil, err
		}
		if stringer, ok := r.(fmt.Stringer); ok {
			sn = &namedScanner{sn, stringer.String()}
		}
		scanners = append(scanners, sn)
		procs = append(procs, &scannerProc{sn})
	}

	if err := program.Compile(cfg.Custom, procs); err != nil {
		return nil, err
	}
	return newMuxOutput(pctx, program.Outputs(), zbuf.MultiStats(scanners)), nil
}

type MultiConfig struct {
	Custom      compiler.Hook
	Distributed bool // true if remote request specified worker count
	Order       zbuf.Order
	Logger      *zap.Logger
	Parallelism int
	Span        nano.Span
	StatsTick   <-chan time.Time
	Worker      worker.WorkerConfig
}

func compileMulti(ctx context.Context, p *ast.Program, zctx *resolver.Context, msrc MultiSource, mcfg MultiConfig) (*muxOutput, error) {
	if mcfg.Logger == nil {
		mcfg.Logger = zap.NewNop()
	}
	if mcfg.Span.Dur == 0 {
		mcfg.Span = nano.MaxSpan
	}
	if mcfg.Parallelism == 0 {
		mcfg.Parallelism = runtime.GOMAXPROCS(0)
	}

	sortKey, sortReversed := msrc.OrderInfo()
	pctx := &proc.Context{
		Context:     ctx,
		TypeContext: zctx,
		Logger:      mcfg.Logger,
		Warnings:    make(chan string, 5),
	}
	program := compiler.NewProgram(p, pctx)
	if err := program.Optimize(sortKey, sortReversed); err != nil {
		return nil, err
	}
	if !program.IsParallelizable(sortKey, sortReversed) {
		mcfg.Parallelism = 1
	}
	sources, pgroup, err := createParallelGroup(pctx, program.Filter(), msrc, mcfg)
	if err != nil {
		return nil, err
	}
	if len(sources) > 1 {
		program.Parallelize(len(sources), sortKey, sortReversed)
	}
	if err := program.Compile(mcfg.Custom, sources); err != nil {
		return nil, err
	}
	return newMuxOutput(pctx, program.Outputs(), pgroup), nil
}
