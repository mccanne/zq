package zngio

import (
	"encoding/binary"
	"io"

	"github.com/brimsec/zq/zcode"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/pierrec/lz4/v4"
)

type Writer struct {
	ow *offsetWriter
	cw *compressionWriter

	encoder          *resolver.Encoder
	buffer           []byte
	streamRecords    int
	streamRecordsMax int
}

func NewWriter(w io.Writer, flags zio.WriterFlags) *Writer {
	ow := &offsetWriter{w: w}
	var cw *compressionWriter
	if flags.ZngCompress {
		cw = newCompressionWriter(ow)
	}
	return &Writer{
		ow:               ow,
		cw:               cw,
		encoder:          resolver.NewEncoder(),
		buffer:           make([]byte, 0, 128),
		streamRecordsMax: flags.StreamRecordsMax,
	}
}

func (w *Writer) write(p []byte) error {
	if w.cw != nil {
		_, err := w.cw.Write(p)
		return err
	}
	_, err := w.ow.Write(p)
	return err
}

func (w *Writer) Position() int64 {
	return w.ow.off
}

func (w *Writer) EndStream() error {
	if w.cw != nil {
		if err := w.cw.Flush(); err != nil {
			return err
		}
	}
	w.encoder.Reset()
	w.streamRecords = 0
	_, err := w.ow.Write([]byte{zng.CtrlEOS})
	return err
}

func (w *Writer) Write(r *zng.Record) error {
	// First send any typedefs for unsent types.
	typ := w.encoder.Lookup(r.Type)
	if typ == nil {
		var b []byte
		var err error
		b, typ, err = w.encoder.Encode(w.buffer[:0], r.Type)
		if err != nil {
			return err
		}
		w.buffer = b
		err = w.write(b)
		if err != nil {
			return err
		}
	}
	dst := w.buffer[:0]
	id := typ.ID()
	// encode id as uvarint7
	if id < 0x40 {
		dst = append(dst, byte(id&0x3f))
	} else {
		dst = append(dst, byte(0x40|(id&0x3f)))
		dst = zcode.AppendUvarint(dst, uint64(id>>6))
	}
	dst = zcode.AppendUvarint(dst, uint64(len(r.Raw)))
	dst = append(dst, r.Raw...)
	w.buffer = dst
	if err := w.write(dst); err != nil {
		return err
	}
	w.streamRecords++
	if w.streamRecordsMax > 0 && w.streamRecords >= w.streamRecordsMax {
		if err := w.EndStream(); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) WriteControl(b []byte) error {
	dst := w.buffer[:0]
	//XXX 0xff for now.  need to pass through control codes?
	dst = append(dst, 0xff)
	dst = zcode.AppendUvarint(dst, uint64(len(b)))
	dst = append(dst, b...)
	return w.write(dst)
}

func (w *Writer) Flush() error {
	if w.streamRecords > 0 {
		return w.EndStream()
	}
	if w.cw != nil {
		if err := w.cw.Flush(); err != nil {
			return err
		}
	}
	return nil
}

type offsetWriter struct {
	w   io.Writer
	off int64
}

func (o *offsetWriter) Write(b []byte) (int, error) {
	n, err := o.w.Write(b)
	o.off += int64(n)
	return n, err
}

type compressionWriter struct {
	w    io.Writer
	ubuf []byte
	zbuf []byte
}

func newCompressionWriter(w io.Writer) *compressionWriter {
	return &compressionWriter{w: w}
}

func (c *compressionWriter) Flush() error {
	if len(c.ubuf) == 0 {
		return nil
	}
	// len(c.ubuf)-1 guarantees compression will either be effective or fail.
	if cap(c.zbuf) < len(c.ubuf)-1 {
		c.zbuf = make([]byte, len(c.ubuf)-1)
	}
	zbuf := c.zbuf[:len(c.ubuf)-1]
	zlen, err := lz4.CompressBlock(c.ubuf, zbuf, nil)
	switch {
	case err != nil:
		return err
	case zlen > 0:
		// Compression was effective.
		header := make([]byte, 0, 1+3*binary.MaxVarintLen64)
		header = append(header, zng.CtrlCompressed)
		header = zcode.AppendUvarint(header, uint64(zng.CompressionFormatLZ4))
		header = zcode.AppendUvarint(header, uint64(len(c.ubuf)))
		header = zcode.AppendUvarint(header, uint64(zlen))
		if _, err := c.w.Write(header); err != nil {
			return err
		}
		if _, err := c.w.Write(zbuf[:zlen]); err != nil {
			return err
		}
	case zlen == 0:
		// Compression wasn't effective, so just write uncompressed data.
		if _, err := c.w.Write(c.ubuf); err != nil {
			return err
		}
	default:
		panic("negative size")
	}
	c.ubuf = c.ubuf[:0]
	return nil
}

func (c *compressionWriter) Write(p []byte) (int, error) {
	const blockMaxSize = 16 * 1024
	if len(c.ubuf)+len(p) > blockMaxSize {
		if err := c.Flush(); err != nil {
			return 0, err
		}
	}
	c.ubuf = append(c.ubuf, p...)
	return len(p), nil
}
