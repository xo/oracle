package types

import (
	"context"
	"errors"
	"io"

	"github.com/xo/oracle/internal/charset"
)

// DefaultPieceSize is the size in bytes of the piece that a LobReader asks
// the server for when NewReaderSize sets none (D22).
const DefaultPieceSize = 1 << 20

// LobReader reads the value of a LOB in pieces, one round trip each, and
// watches its context on each. It reads the text of a CLOB as UTF-8. A LOB of
// a row can be read until its rows close (D22).
type LobReader struct {
	ctx    context.Context
	stream LobStreamer
	// conv decodes the text of a CLOB, and is nil for bytes. utf16 is true
	// when the server sends the text of the CLOB as UTF-16, where an offset
	// counts code units of 2 bytes.
	conv  *charset.Converter
	utf16 bool
	// count is the units that one piece asks for, and offset the units read.
	count  int64
	offset int64
	// buf holds what a piece brought and Read has not returned. carry holds
	// the first half of a surrogate pair that ended a piece.
	buf   []byte
	carry []byte
	err   error
}

// newLobReader returns a reader of stream in pieces of size bytes. A LOB
// with no locator holds its value already, which value returns.
func newLobReader(ctx context.Context, stream LobStreamer, size int, conv *charset.Converter, utf16 bool, value func() ([]byte, error)) *LobReader {
	if size <= 0 {
		size = DefaultPieceSize
	}
	r := &LobReader{ctx: ctx, stream: stream, conv: conv, utf16: utf16, count: int64(size)}
	if utf16 {
		r.count = max(int64(size/2), 2)
	}
	if stream == nil || stream.Locator() == nil || stream.Locator().IsInline() {
		if value != nil {
			r.buf, r.err = value()
		}
		if r.err == nil {
			r.err = io.EOF
		}
	}
	return r
}

// Read reads the next bytes of the value into p.
func (r *LobReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.fill()
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// fill reads the next piece into buf, or sets err.
func (r *LobReader) fill() {
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return
	}
	end := r.stream.StartContext(r.ctx)
	data, err := r.stream.Read(r.offset, r.count)
	r.stream.EndContext(end)
	switch {
	case err != nil && r.ctx.Err() != nil:
		r.err = r.ctx.Err()
		return
	case err != nil:
		r.err = err
		return
	case len(data) == 0:
		r.err = io.EOF
		if len(r.carry) != 0 {
			r.buf = []byte(r.conv.Decode(r.carry))
			r.carry = nil
		}
		return
	}
	switch {
	case r.conv == nil:
		r.offset += int64(len(data))
		r.buf = append(r.buf[:0], data...)
	case r.utf16:
		r.offset += int64(len(data) / 2)
		data = append(r.carry, data...)
		r.carry = nil
		// a piece can end with the first half of a pair, which the next
		// piece completes
		if n := len(data); n >= 2 && isHighSurrogate(data[n-2:], r.conv.LangID) {
			r.carry, data = append([]byte(nil), data[n-2:]...), data[:n-2]
		}
		r.buf = []byte(r.conv.Decode(data))
	default:
		r.offset += int64(len(data))
		r.buf = []byte(r.conv.Decode(data))
	}
}

// isHighSurrogate reports whether unit, a UTF-16 code unit in the byte order
// of the charset langID, is the first half of a surrogate pair.
func isHighSurrogate(unit []byte, langID int) bool {
	u := uint16(unit[0])<<8 | uint16(unit[1])
	if langID == 2002 {
		u = uint16(unit[1])<<8 | uint16(unit[0])
	}
	return u >= 0xD800 && u <= 0xDBFF
}

// NewReader returns a reader of the value of the BLOB, in pieces of
// DefaultPieceSize bytes, that watches ctx.
func (blob *Blob) NewReader(ctx context.Context) *LobReader {
	return blob.NewReaderSize(ctx, DefaultPieceSize)
}

// NewReaderSize is NewReader with pieces of size bytes.
func (blob *Blob) NewReaderSize(ctx context.Context, size int) *LobReader {
	return newLobReader(ctx, blob.stream, size, nil, false, func() ([]byte, error) {
		return blob.bValue, nil
	})
}

// NewReader returns a reader of the text of the CLOB as UTF-8, in pieces of
// about DefaultPieceSize bytes, that watches ctx.
func (clob *Clob) NewReader(ctx context.Context) *LobReader {
	return clob.NewReaderSize(ctx, DefaultPieceSize)
}

// NewReaderSize is NewReader with pieces of about size bytes. The server
// counts a piece in characters, which can each be more than one byte.
func (clob *Clob) NewReaderSize(ctx context.Context, size int) *LobReader {
	utf16 := false
	if !clob.IsNil() {
		utf16 = clob.Locator().IsVarWidthChar()
	}
	return newLobReader(ctx, clob.stream, size, clob.Conv, utf16, func() ([]byte, error) {
		v, err := clob.Value()
		s, _ := v.(string)
		return []byte(s), err
	})
}

// NewReader returns a reader of the file, in pieces of DefaultPieceSize
// bytes, that watches ctx. The file must be open.
func (file *BFile) NewReader(ctx context.Context) *LobReader {
	return file.NewReaderSize(ctx, DefaultPieceSize)
}

// NewReaderSize is NewReader with pieces of size bytes.
func (file *BFile) NewReaderSize(ctx context.Context, size int) *LobReader {
	if !file.isOpened {
		return &LobReader{err: errors.New("reading a BFILE: the file is not open")}
	}
	return newLobReader(ctx, file.stream, size, nil, false, nil)
}
