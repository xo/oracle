package types

import (
	"context"
	"database/sql"
)

type Blob struct {
	Raw
	lobBase
}

func (blob *Blob) Upload() error {
	return blob.uploadData(blob.bValue, 0, 0)
}

func (blob *Blob) SetValue(input any) error {
	var err error
	switch input := input.(type) {
	case Blob:
		*blob = input
		return nil
	case *Blob:
		*blob = *input
		return nil
	default:
		err = blob.Raw.SetValue(input)
		if err != nil {
			return err
		}
	}
	return blob.Upload()
}

// CreateBlob returns a BLOB that holds input, with a streamer from db. The
// value goes to the server when the BLOB is bound to a statement.
func CreateBlob(ctx context.Context, db *sql.DB, input any) (*Blob, error) {
	var err error
	ret := &Blob{}
	err = ret.createStreamer(ctx, db)
	if err != nil {
		return nil, err
	}
	return ret, ret.SetValue(input)
}

func (blob *Blob) Load() error {
	var err error
	blob.bValue, err = blob.load()
	return err
}

func (blob *Blob) Read(ctx context.Context) error {
	var err error
	blob.bValue, err = blob.ReadFromPos(ctx, 0)
	return err
}
func (blob *Blob) Scan(src any) error {
	return blob.SetValue(src)
}
