package types

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xo/oracle/internal/configurations"
)

type Lob interface {
	GetLocator() Locator
	Free() error
	GetLength() (int64, error)
	Read(ctx context.Context) error
	// Load reads the whole value with no context of its own. The driver
	// calls it inside a call whose context it watches already, and while it
	// fetches rows, where database/sql gives it no context.
	Load() error
	GetReadMode() configurations.LobReadMode
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
func (l *lobBase) GetStreamer() LobStreamer {
	return l.stream
}
func (l *lobBase) SetStreamer(input LobStreamer) {
	l.stream = input
}
func (l *lobBase) GetLocator() Locator {
	if l.IsNil() {
		return nil
	}
	return l.stream.GetLocator()
}
func (l *lobBase) IsNil() bool {
	return l.stream == nil || l.stream.GetLocator() == nil
}
func (l *lobBase) IsQuasi() bool {
	if l.IsNil() {
		return true
	}
	return l.stream.GetLocator().IsQuasi()
}
func (l *lobBase) IsDataUploaded() bool {
	return l.stream != nil && l.stream.GetLocator() != nil
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
	if !l.IsQuasi() {
		err := l.stream.FreeTemporaryLocator()
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *lobBase) GetLength() (int64, error) {
	if !l.IsQuasi() {
		return l.stream.GetSize()
	}
	return 0, nil
}

func (l *lobBase) GetReadMode() configurations.LobReadMode {
	if !l.IsQuasi() {
		return l.stream.GetLobReadMode()
	}
	return configurations.LobReadMode_NONE
}

func (l *lobBase) ReadFromPos(ctx context.Context, pos int64) ([]byte, error) {
	return l.ReadBytesFromPos(ctx, pos, 0)
}

func (l *lobBase) ReadBytesFromPos(ctx context.Context, pos, count int64) ([]byte, error) {
	if !l.IsQuasi() {
		done := l.stream.StartContext(ctx)
		defer l.stream.EndContext(done)
		return l.stream.Read(pos, count)
	}
	return nil, nil
}

// load reads the whole value and watches no context.
func (l *lobBase) load() ([]byte, error) {
	if l.IsQuasi() {
		return nil, nil
	}
	return l.stream.Read(0, 0)
}
