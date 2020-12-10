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
	Warnings          chan string
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

func compile(ctx context.Context, program ast.Proc, zctx *resolver.Context, readers []zbuf.Reader, cfg Config) (*muxOutput, error) {
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.Span.Dur == 0 {
		cfg.Span = nano.MaxSpan
	}
	if cfg.Warnings == nil {
		cfg.Warnings = make(chan string, 5)
	}

	filterExpr, program := compiler.Optimize(zctx, program, field.Dotted(cfg.ReaderSortKey), cfg.ReaderSortReverse)
	procs := make([]proc.Interface, 0, len(readers))
	scanners := make([]zbuf.Scanner, 0, len(readers))
	for _, r := range readers {
		sn, err := zbuf.NewScanner(ctx, r, filterExpr, cfg.Span)
		if err != nil {
			return nil, err
		}
		if stringer, ok := r.(fmt.Stringer); ok {
			sn = &namedScanner{sn, stringer.String()}
		}
		scanners = append(scanners, sn)
		procs = append(procs, &scannerProc{sn})
	}

	pctx := &proc.Context{
		Context:     ctx,
		TypeContext: zctx,
		Logger:      cfg.Logger,
		Warnings:    cfg.Warnings,
	}
	leaves, err := compiler.Compile(cfg.Custom, program, pctx, procs)
	if err != nil {
		return nil, err
	}
	return newMuxOutput(pctx, leaves, zbuf.MultiStats(scanners)), nil
}

type MultiConfig struct {
	Custom      compiler.Hook
	Distributed bool // true if remote request specified worker count
	Order       zbuf.Order
	Logger      *zap.Logger
	Parallelism int
	Span        nano.Span
	StatsTick   <-chan time.Time
	Warnings    chan string
}

func compileMulti(ctx context.Context, program ast.Proc, zctx *resolver.Context, msrc MultiSource, mcfg MultiConfig) (*muxOutput, error) {
	if mcfg.Logger == nil {
		mcfg.Logger = zap.NewNop()
	}
	if mcfg.Span.Dur == 0 {
		mcfg.Span = nano.MaxSpan
	}
	if mcfg.Warnings == nil {
		mcfg.Warnings = make(chan string, 5)
	}

	if mcfg.Parallelism == 0 {
		mcfg.Parallelism = runtime.GOMAXPROCS(0)
	}

	sortKey, sortReversed := msrc.OrderInfo()
	filterExpr, program := compiler.Optimize(zctx, program, sortKey, sortReversed)

	var isParallel bool
	if mcfg.Parallelism > 1 {
		program, isParallel = compiler.Parallelize(program, mcfg.Parallelism, sortKey, sortReversed)
	}
	if !isParallel {
		mcfg.Parallelism = 1
	}

	pctx := &proc.Context{
		Context:     ctx,
		TypeContext: zctx,
		Logger:      mcfg.Logger,
		Warnings:    mcfg.Warnings,
	}
	sources, pgroup, err := createParallelGroup(pctx, filterExpr, msrc, mcfg)
	if err != nil {
		return nil, err
	}
	leaves, err := compiler.Compile(mcfg.Custom, program, pctx, sources)
	if err != nil {
		return nil, err
	}
	return newMuxOutput(pctx, leaves, pgroup), nil
}
