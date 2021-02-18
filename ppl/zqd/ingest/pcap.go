package ingest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/pkg/fs"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/zqd/pcapanalyzer"
	"github.com/brimsec/zq/ppl/zqd/pcapstorage"
	"github.com/brimsec/zq/ppl/zqd/storage"
	"github.com/brimsec/zq/ppl/zqd/storage/archivestore"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/detector"
	"github.com/brimsec/zq/zio/ndjsonio"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng/resolver"
)

//go:generate go run ../../../zio/ndjsonio/typegenerator -o ./suricata.go -package ingest -var suricataTC ./suricata-types.json

type PcapOp interface {
	Status() api.PcapPostStatus
	// Err returns the an error if an error occurred while the ingest process was
	// running. If the process is still running Err will wait for the process to
	// complete before returning.
	Err() error
	Warn() <-chan string
	Done() <-chan struct{}
	Snap() <-chan struct{}
}

var suricataTransform = compiler.MustParseProc("rename ts=timestamp")

type ClearableStore interface {
	storage.Storage
	Clear(ctx context.Context) error
}

// NewPcapOp kicks of the process for ingesting a pcap file into a space.
func NewPcapOp(ctx context.Context, store storage.Storage, pcapstore *pcapstorage.Store, pcap string, suricata, zeek pcapanalyzer.Launcher) (PcapOp, error) {
	pcapuri, err := iosrc.ParseURI(pcap)
	if err != nil {
		return nil, err
	}
	if suricata == nil && zeek == nil {
		return nil, fmt.Errorf("must provide at least one launcher")
	}
	logstore, ok := store.(ClearableStore)
	if ok {
		return newFilePcapOp(ctx, pcapstore, logstore, pcapuri, suricata, zeek)
	}
	return newArchivePcapOp(ctx, store.(*archivestore.Storage), pcapstore, pcapuri, suricata, zeek)
}

type legacyPcapOp struct {
	pcapstore  *pcapstorage.Store
	store      ClearableStore
	snapshots  int32
	pcapuri    iosrc.URI
	logdir     string
	done, snap chan struct{}
	err        error
	warn       chan string

	slauncher, zlauncher pcapanalyzer.Launcher

	// stats
	startTime    nano.Ts
	pcapSize     int64
	pcapReadSize int64
}

func newFilePcapOp(ctx context.Context, pcapstore *pcapstorage.Store, store ClearableStore, pcapuri iosrc.URI, slauncher, zlauncher pcapanalyzer.Launcher) (*legacyPcapOp, error) {
	if slauncher == nil && zlauncher == nil {
		return nil, fmt.Errorf("must provide at least one launcher")
	}
	info, err := iosrc.Stat(ctx, pcapuri)
	if err != nil {
		return nil, err
	}
	warn := make(chan string)
	go func() {
		err = pcapstore.Update(ctx, pcapuri, warn)
		close(warn)
	}()
	var warnings []string
	for w := range warn {
		warnings = append(warnings, w)
	}
	if err != nil {
		return nil, err
	}
	logdir, err := ioutil.TempDir("", "zqd-pcap-ingest-")
	if err != nil {
		return nil, err
	}
	p := &legacyPcapOp{
		startTime: nano.Now(),
		pcapSize:  info.Size(),
		pcapstore: pcapstore,
		store:     store,
		pcapuri:   pcapuri,
		logdir:    logdir,
		done:      make(chan struct{}),
		snap:      make(chan struct{}),
		warn:      make(chan string),
		slauncher: slauncher,
		zlauncher: zlauncher,
	}
	go func() {
		for _, w := range warnings {
			p.warn <- w
		}
		p.err = p.run(ctx)
		close(p.done)
		close(p.snap)
	}()
	return p, nil
}

func (p *legacyPcapOp) run(ctx context.Context) error {
	var sErr, zErr error
	var wg sync.WaitGroup
	slurpDone := make(chan struct{})
	if p.slauncher != nil {
		wg.Add(1)
		go func() {
			sErr = p.runSuricata(ctx)
			wg.Done()
		}()
	}
	if p.zlauncher != nil {
		wg.Add(1)
		go func() {
			zErr = p.runZeek(ctx)
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		close(slurpDone)
	}()

	abort := func() {
		os.RemoveAll(p.logdir)
		// Don't want to use passed context here because a cancelled context
		// would cause storage not to be cleared.
		p.pcapstore.Delete(context.Background())
		p.store.Clear(context.Background())
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	start := time.Now()
	next := time.Second
outer:
	for {
		select {
		case <-slurpDone:
			break outer
		case t := <-ticker.C:
			if t.After(start.Add(next)) {
				if err := p.createSnapshot(ctx); err != nil {
					abort()
					return err
				}
				select {
				case p.snap <- struct{}{}:
				default:
				}
				next = 2 * next
			}
		}
	}
	if sErr != nil {
		abort()
		return sErr
	}
	if zErr != nil {
		abort()
		return zErr
	}

	if err := p.createSnapshot(ctx); err != nil {
		abort()
		return err
	}
	if err := os.RemoveAll(p.logdir); err != nil {
		abort()
		return err
	}
	return nil
}

func (p *legacyPcapOp) runZeek(ctx context.Context) error {
	pcapfile, err := iosrc.NewReader(ctx, p.pcapuri)
	if err != nil {
		return err
	}
	defer pcapfile.Close()
	r := io.TeeReader(pcapfile, p)
	zproc, err := p.zlauncher(ctx, bufio.NewReader(r), p.logdir)
	if err != nil {
		return err
	}
	return zproc.Wait()
}

func (p *legacyPcapOp) runSuricata(ctx context.Context) error {
	pcapfile, err := iosrc.NewReader(ctx, p.pcapuri)
	if err != nil {
		return err
	}
	defer pcapfile.Close()
	sproc, err := p.slauncher(ctx, bufio.NewReader(pcapfile), p.logdir)
	if err != nil {
		return err
	}
	if err = sproc.Wait(); err != nil {
		return err
	}
	return p.convertSuricataLog(ctx)
}

func (p *legacyPcapOp) Status() api.PcapPostStatus {
	return api.PcapPostStatus{
		Type:          "PcapPostStatus",
		StartTime:     p.startTime,
		UpdateTime:    nano.Now(),
		PcapSize:      p.pcapSize,
		PcapReadSize:  atomic.LoadInt64(&p.pcapReadSize),
		SnapshotCount: int(atomic.LoadInt32(&p.snapshots)),
	}
}

// Err returns the an error if an error occurred while the ingest process was
// running. If the process is still running Err will wait for the process to
// complete before returning.
func (p *legacyPcapOp) Err() error {
	<-p.done
	return p.err
}

// Done returns a chan that emits when the ingest process is complete.
func (p *legacyPcapOp) Done() <-chan struct{} {
	return p.done
}

// Warn returns a chan that emits warnings.
func (p *legacyPcapOp) Warn() <-chan string {
	return p.warn
}

func (p *legacyPcapOp) SnapshotCount() int {
	return int(atomic.LoadInt32(&p.snapshots))
}

// Snap returns a chan that emits every time a snapshot is made. It
// should no longer be read from after Done() has emitted.
func (p *legacyPcapOp) Snap() <-chan struct{} {
	return p.snap
}

func (p *legacyPcapOp) zeekFiles() []string {
	files, err := filepath.Glob(filepath.Join(p.logdir, "*.log"))
	// Per filepath.Glob documentation the only possible error would be due to
	// an invalid glob pattern. Ok to panic.
	if err != nil {
		panic(err)
	}
	return files
}

func (p *legacyPcapOp) suricataFiles() []string {
	path := filepath.Join(p.logdir, "eve.zng")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	return []string{path}
}

func (p *legacyPcapOp) createSnapshot(ctx context.Context) error {
	files := append(p.zeekFiles(), p.suricataFiles()...)
	if len(files) == 0 {
		return nil
	}
	// convert logs into sorted zng
	zctx := resolver.NewContext()
	readers, err := detector.OpenFiles(ctx, zctx, files...)
	if err != nil {
		return err
	}
	defer zbuf.CloseReaders(readers)
	reader, err := zbuf.MergeReadersByTsAsReader(ctx, readers, p.store.NativeOrder())
	if err != nil {
		return err
	}
	if err := p.store.Write(ctx, zctx, reader); err != nil {
		return err
	}
	atomic.AddInt32(&p.snapshots, 1)
	return nil
}

func (p *legacyPcapOp) convertSuricataLog(ctx context.Context) error {
	zctx := resolver.NewContext()
	path := filepath.Join(p.logdir, "eve.json")
	zr, err := detector.OpenFile(zctx, path, zio.ReaderOpts{JSON: ndjsonio.ReaderOpts{TypeConfig: suricataTC, Warnings: p.warn}})
	if err != nil {
		return err
	}
	defer zr.Close()
	return fs.ReplaceFile(filepath.Join(p.logdir, "eve.zng"), os.FileMode(0666), func(w io.Writer) error {
		zw := zngio.NewWriter(zio.NopCloser(w), zngio.WriterOpts{})
		program := &ast.Program{Entry: suricataTransform}
		return driver.Copy(ctx, zw, program, zctx, zr, driver.Config{})
	})
}

func (p *legacyPcapOp) Write(b []byte) (int, error) {
	n := len(b)
	atomic.AddInt64(&p.pcapReadSize, int64(n))
	return n, nil
}
