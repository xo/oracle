package types

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xo/oracle/internal/config"
)

type Lob interface {
	Locator() Locator
	Free() error
	Length() (int64, error)
	Read(ctx context.Context) error
	// Load reads the whole value with no context of its own. The driver
	// calls it inside a call whose context it watches already, and while it
	// fetches rows, where database/sql gives it no context.
	Load() error
	ReadMode() config.LobReadMode
	sql.Scanner
}

var errEmptyLocator = errors.New("calling Lob function: on nil or non allocated object")
var errNilStreamer = errors.New("calling Lob function: nil streamer")

type lobBase struct {
	stream LobStreamer
}

func (l *lobBase) createStreamer(ctx context.Context, db *sql.DB) error {
	var err error
	if l.stream == nil {
		_, err = db.ExecContext(ctx, "--CREATE-LOB-STREAM--", &l.stream)
	}
	return err
}
func (l *lobBase) Streamer() LobStreamer {
	return l.stream
}
func (l *lobBase) SetStreamer(input LobStreamer) {
	l.stream = input
}
func (l *lobBase) Locator() Locator {
	if l.IsNil() {
		return nil
	}
	return l.stream.Locator()
}
func (l *lobBase) IsNil() bool {
	return l.stream == nil || l.stream.Locator() == nil
}
func (l *lobBase) IsInline() bool {
	if l.IsNil() {
		return true
	}
	return l.stream.Locator().IsInline()
}
func (l *lobBase) IsDataUploaded() bool {
	return l.stream != nil && l.stream.Locator() != nil
}

func (lob *lobBase) uploadData(data []byte, charsetID, charsetForm int) error {
	if len(data) == 0 {
		return nil
	}
	var err error
	if !lob.IsDataUploaded() && lob.stream != nil {
		// the upload runs inside the call that binds the LOB, which watches
		// the context of that call

		_, err = lob.stream.CreateTemporaryLocator(charsetID, charsetForm)
		if err != nil {
			return err
		}
		err = lob.stream.Write(data)
	}
	return err
}

func (l *lobBase) Free() error {
	if !l.IsInline() {
		err := l.stream.FreeTemporaryLocator()
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *lobBase) Length() (int64, error) {
	if !l.IsInline() {
		return l.stream.Size()
	}
	return 0, nil
}

func (l *lobBase) ReadMode() config.LobReadMode {
	if !l.IsInline() {
		return l.stream.LobReadMode()
	}
	return config.LobReadModeNone
}

func (l *lobBase) ReadFromPos(ctx context.Context, pos int64) ([]byte, error) {
	return l.ReadBytesFromPos(ctx, pos, 0)
}

func (l *lobBase) ReadBytesFromPos(ctx context.Context, pos, count int64) ([]byte, error) {
	if !l.IsInline() {
		done := l.stream.StartContext(ctx)
		defer l.stream.EndContext(done)
		return l.stream.Read(pos, count)
	}
	return nil, nil
}

// load reads the whole value and watches no context.
func (l *lobBase) load() ([]byte, error) {
	if l.IsInline() {
		return nil, nil
	}
	return l.stream.Read(0, 0)
}
