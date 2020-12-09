package microindex

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/pkg/bufwriter"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/zngio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
)

// Writer implements the zbuf.Writer interface. A Writer creates a microindex,
// comprising the base zng file along with its related B-tree sections,
// as zng records are consumed.
//
// The keyFields argument to NewWriter provides a list of the key names, ordered by
// precedence, that will serve as the keys into the index.  The input
// records may or may not have all the key fields.  If a key field is
// missing, it appears as a null value in the index.  Nulls are sorted
// before all non-null values.  All key fields must have the same type.
// The Writer may detect an error if a key field changes type but does not
// check that every key has the same type; it is up to the caller to guarantee
// this type consistency.  For example, the caller should create a separate
// microindex for fields that have a common name but different types.
//
// The keys in the input zng stream must be previously sorted consistent
// with the precedence order of the keyFields.
//
// As the zng file data is written, a B-tree index is computed as a
// constant B-tree to make key lookups efficient.  The B-tree sections
// are written to temporary files and at close, they are merged into
// a single-file microindex.
//
// If a Writer is created but Closed without ever writing records to it, then
// the index is created with no keys and an "empty" microindex trailer.  This is
// useful for knowing when something has been indexed but no keys were present.
// If a Writer is created then an error is enountered (for example, the type of
// key changes), then you generally want to abort and cleanup by calling Abort()
// instead of Close().
type Writer struct {
	uri         iosrc.URI
	keyFields   []field.Static
	zctx        *resolver.Context
	writer      *indexWriter
	cutter      *expr.Cutter
	tmpdir      string
	frameThresh int
	keyType     *zng.TypeRecord
	iow         io.WriteCloser
	childField  string
	nlevel      int
	order       zbuf.Order
}

type indexWriter struct {
	base       *Writer
	parent     *indexWriter
	name       string
	buffer     *bufwriter.Writer
	zng        *zngio.Writer
	frameStart int64
	frameEnd   int64
	frameKey   *zng.Record
}

// NewWriter returns a Writer ready to write a microindex or it returns
// an error.  The microindex is written to the URL provided in the path argument
// while temporary file are written locally.  Calls to Write must
// provide keys in increasing lexicographic order.  Duplicate keys are not
// allowed but will not be detected.  Close() or Abort() must be called when
// done writing.
func NewWriter(zctx *resolver.Context, path string, options ...Option) (*Writer, error) {
	return NewWriterWithContext(context.Background(), zctx, path, options...)
}

func NewWriterWithContext(ctx context.Context, zctx *resolver.Context, path string, options ...Option) (*Writer, error) {
	w := &Writer{zctx: zctx}
	for _, opt := range options {
		opt.apply(w)
	}
	if w.keyFields == nil {
		w.keyFields = []field.Static{field.New("key")}
	}
	if w.frameThresh == 0 {
		w.frameThresh = frameThresh
	}
	if w.frameThresh > FrameMaxSize {
		return nil, fmt.Errorf("frame threshold too large (%d)", w.frameThresh)
	}
	var err error
	w.uri, err = iosrc.ParseURI(path)
	if err != nil {
		return nil, err
	}
	w.iow, err = iosrc.NewWriter(ctx, w.uri)
	if err != nil {
		return nil, err
	}
	w.tmpdir, err = ioutil.TempDir("", "microindex-*")
	if err != nil {
		return nil, err
	}
	fields, resolvers := compiler.CompileAssignments(w.keyFields, w.keyFields)
	w.cutter, err = expr.NewCutter(zctx, fields, resolvers)
	return w, err
}

type Option interface {
	apply(*Writer)
}

type optionFunc func(*Writer)

func (f optionFunc) apply(w *Writer) { f(w) }

func KeyFields(keys ...field.Static) Option {
	return optionFunc(func(w *Writer) {
		w.keyFields = keys
	})
}

func Keys(keys ...string) Option {
	return optionFunc(func(w *Writer) {
		for _, k := range keys {
			w.keyFields = append(w.keyFields, field.Dotted(k))
		}
	})
}

func FrameThresh(frameThresh int) Option {
	return optionFunc(func(w *Writer) {
		w.frameThresh = frameThresh
	})
}

func Order(order zbuf.Order) Option {
	return optionFunc(func(w *Writer) {
		w.order = order
	})
}

func (w *Writer) Write(rec *zng.Record) error {
	if w.writer == nil {
		var err error
		w.writer, err = newIndexWriter(w, w.iow, "")
		if err != nil {
			return err
		}
		keys, err := w.cutter.Apply(rec)
		if err != nil {
			return err
		}
		w.keyType = keys.Type
		w.childField = uniqChildField(w.zctx, keys)
	}
	return w.writer.write(rec)
}

// Abort closes this writer, deleting any and all objects and/or files associated
// with it.
func (w *Writer) Abort() error {
	// Delete the temp files comprising the index hierarchy.
	defer os.RemoveAll(w.tmpdir)
	err := w.closeTree()
	if closeErr := w.iow.Close(); err == nil {
		err = closeErr
	}
	// Ignore context here in the event that context is the reson for the abort.
	if rmErr := iosrc.Remove(context.Background(), w.uri); err == nil {
		err = rmErr
	}
	return err
}

func (w *Writer) Close() error {
	// No matter what, delete the temp files comprising the index hierarchy.
	defer os.RemoveAll(w.tmpdir)
	// First, close the parent if it exists (which will recursively close
	// all the parents to the root) while leaving the base layer open.
	if err := w.closeTree(); err != nil {
		w.iow.Close()
		return err
	}
	if w.writer == nil {
		// If the writer hasn't been created because no records were
		// encountered, then the base layer writer was never created.
		// In this case, bypass the base layer, write an empty trailer
		// directly to the output, and close.
		err := w.writeEmptyTrailer()
		if err2 := w.iow.Close(); err == nil {
			err = err2
		}
		return err
	}
	// Otherwise, close the frame of the base layer so we can copy the hierarchy
	// to the base.  Note that sum of the sizes of the parents is much smaller
	// than the base so this will go fast compared to the entire indexing job.
	if err := w.writer.closeFrame(); err != nil {
		return err
	}
	// The hierarchy is now flushed and closed.  Assemble the file into
	// a single microindex and remove the temporary btree files.
	if err := w.finalize(); err != nil {
		w.writer.buffer.Close()
		return err
	}
	// Finally, close the base layer.
	return w.writer.buffer.Close()
}

func (w *Writer) closeTree() error {
	if w.writer == nil {
		return nil
	}
	var err error
	for p := w.writer.parent; p != nil; p = p.parent {
		if closeErr := p.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (w *Writer) finalize() error {
	// First, collect up parent linkage into a slice so we can traverse
	// top down...
	base := w.writer
	var layers []*indexWriter
	for p := base.parent; p != nil; p = p.parent {
		layers = append(layers, p)
	}
	// Now, copy each non-base file in top-down order to the base-layer object.
	var sizes []int64
	sizes = append(sizes, base.frameStart)
	for k := len(layers) - 1; k >= 0; k-- {
		// Copy the files in the reverse order so the root comes first.
		// This will avoid backward seeks while looking up keys in the tree
		// (except for the one backward seek to the base layer).
		layer := layers[k]
		size := layer.frameStart
		sizes = append(sizes, size)
		f, err := os.Open(layer.name)
		if err != nil {
			return err
		}
		n, err := io.Copy(base.buffer, f)
		if err != nil {
			f.Close()
			return err
		}
		if n != size {
			f.Close()
			return fmt.Errorf("internal microindex error: index file size (%d) does not equal zng size (%d)", n, size)
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return writeTrailer(base.zng, w.zctx, w.childField, w.frameThresh, sizes, w.keyType, w.order)
}

func (w *Writer) writeEmptyTrailer() error {
	var cols []zng.Column
	for _, key := range w.keyFields {
		cols = append(cols, zng.Column{key.String(), zng.TypeNull})
	}
	typ, err := w.zctx.LookupTypeRecord(cols)
	if err != nil {
		return err
	}
	zw := zngio.NewWriter(w.iow, zngio.WriterOpts{})
	return writeTrailer(zw, w.zctx, "", w.frameThresh, nil, typ, w.order)
}

func writeTrailer(w *zngio.Writer, zctx *resolver.Context, childField string, frameThresh int, sizes []int64, keyType *zng.TypeRecord, order zbuf.Order) error {
	// Finally, write the size records as the trailer of the microindex.
	rec, err := newTrailerRecord(zctx, childField, frameThresh, sizes, keyType, order)
	if err != nil {
		return err
	}
	if err := w.Write(rec); err != nil {
		return err
	}
	return w.EndStream()
}

func newIndexWriter(base *Writer, w io.WriteCloser, name string) (*indexWriter, error) {
	base.nlevel++
	if base.nlevel >= MaxLevels {
		return nil, ErrTooManyLevels
	}
	writer := bufwriter.New(w)
	return &indexWriter{
		base:     base,
		buffer:   writer,
		name:     name,
		zng:      zngio.NewWriter(writer, zngio.WriterOpts{}),
		frameEnd: int64(base.frameThresh),
	}, nil
}

func (w *indexWriter) newParent() (*indexWriter, error) {
	file, err := ioutil.TempFile(w.base.tmpdir, "")
	if err != nil {
		return nil, err
	}
	return newIndexWriter(w.base, file, file.Name())
}

func (w *indexWriter) Close() error {
	// Make sure to pass up framekeys to parent trees, even though frames aren't
	// full.
	if err := w.closeFrame(); err != nil {
		return err
	}
	return w.buffer.Close()
}

func (w *indexWriter) write(rec *zng.Record) error {
	offset := w.zng.Position()
	if offset >= w.frameEnd && w.frameKey != nil {
		w.frameEnd = offset + int64(w.base.frameThresh)
		// the frame in place is already big enough... flush it and
		// start going on the next
		if err := w.endFrame(); err != nil {
			return err
		}
	}
	if w.frameKey == nil {
		// When we start a new frame, we want to create a key entry
		// in the parent for the current key but we don't want to write
		// it until we know this frame will be big enough to add it
		// (or until we know it's the last frame in the file).
		// So we build the frame key record from the current record
		// here ahead of its use and save it in the frameKey variable.
		key, err := w.base.cutter.Apply(rec)
		if err != nil {
			return err
		}
		// If the key isn't here flag an error.  All keys must be
		// present to build a proper index.
		// XXX We also need to check that they are in order.
		if key == nil {
			return fmt.Errorf("no key field present in record of type: %s", rec.Type)
		}
		w.frameKey = key
	}
	return w.zng.Write(rec)
}

func (w *indexWriter) endFrame() error {
	if err := w.addToParentIndex(w.frameKey, w.frameStart); err != nil {
		return err
	}
	if err := w.closeFrame(); err != nil {
		return err
	}
	return nil
}

func (w *indexWriter) closeFrame() error {
	if err := w.zng.EndStream(); err != nil {
		return err
	}
	w.frameStart = w.zng.Position()
	w.frameKey = nil
	return nil
}

func (w *indexWriter) addToParentIndex(key *zng.Record, offset int64) error {
	if w.parent == nil {
		var err error
		w.parent, err = w.newParent()
		if err != nil {
			return err
		}
	}
	return w.parent.writeIndexRecord(key, offset)
}

func (w *indexWriter) writeIndexRecord(keys *zng.Record, offset int64) error {
	col := []zng.Column{{w.base.childField, zng.TypeInt64}}
	val := zng.EncodeInt(offset)
	rec, err := w.base.zctx.AddColumns(keys, col, []zng.Value{{zng.TypeInt64, val}})
	if err != nil {
		return err
	}
	return w.write(rec)
}
