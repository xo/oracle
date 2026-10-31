package ttc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

// The flags of the image of an object, and of the XML in an XMLType, as
// python-oracledb names them.
const (
	objectDegenerate = 0x10
	objectNoPrefix   = 0x04

	xmlLOB       = 0x000001
	xmlString    = 0x000004
	xmlSkipNext4 = 0x100000
)

// xmlParameter reads an XMLType column. The server sends it as the image of
// an object, whose flag word says whether the XML follows as text or as the
// locator of a CLOB. go-ora read no XMLType in v3, and in v2 it compared the
// flag word for equality, which missed the 0x8014 of Autonomous Database,
// sijms/go-ora#771.
type xmlParameter struct {
	codec.BasicParameter
}

func (param *xmlParameter) Copy() codec.OracleParameterCoder {
	ret := *param
	return &ret
}

func (param *xmlParameter) Init() {
	param.SetDefault()
	param.DataType = types.XMLType
}

func (param *xmlParameter) Encode(any, codec.Conn) error {
	return fmt.Errorf("binding an XMLType: bind its text, and convert it with XMLTYPE(:1): %w", tns.ErrNotSupported)
}

func (param *xmlParameter) Write(tns.SessionWriter) error {
	return fmt.Errorf("binding an XMLType: %w", tns.ErrNotSupported)
}

func (param *xmlParameter) SetLobStreamer(types.LobStreamer) {}

// Read reads the object that holds the image, as ObjectParameter does.
func (param *xmlParameter) Read(session tns.SessionReader) error {
	if _, err := session.ReadDLC(); err != nil { // the type ID
		return err
	}
	if _, err := session.ReadBytes(3); err != nil {
		return err
	}
	size, err := session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	if _, err := session.ReadBytes(2); err != nil { // 0x1 0x1
		return err
	}
	if size == 0 {
		// a NULL, which has no image
		param.BValue = nil
		return nil
	}
	param.BValue, err = param.BasicRead(session)
	return err
}

func (param *xmlParameter) Decode(conn codec.Conn) (any, error) {
	if len(param.BValue) == 0 {
		return nil, nil
	}
	payload, lob, err := parseXMLImage(param.BValue)
	if err != nil {
		return nil, err
	}
	if lob {
		return xmlClob(payload, conn)
	}
	conv, err := conn.StringCoder(0, 1)
	if err != nil {
		return nil, err
	}
	return conv.Decode(payload), nil
}

// parseXMLImage returns the XML of the image of an XMLType: its text, or the
// locator of the CLOB that holds it when lob is true. It tests each bit of
// the flag word, as python-oracledb does.
func parseXMLImage(image []byte) (payload []byte, lob bool, err error) {
	errShort := errors.New("reading an XMLType: the image is too short")
	if len(image) < 3 {
		return nil, false, errShort
	}
	flags, pos := image[0], 2 // the flags and the version
	length := func() (int, bool) {
		if pos >= len(image) {
			return 0, false
		}
		if image[pos] != 0xFE {
			pos++
			return int(image[pos-1]), true
		}
		if pos+5 > len(image) {
			return 0, false
		}
		n := int(binary.BigEndian.Uint32(image[pos+1:]))
		pos += 5
		return n, true
	}
	if _, ok := length(); !ok {
		return nil, false, errShort
	}
	if flags&objectDegenerate != 0 {
		return nil, false, fmt.Errorf("reading an XMLType: a degenerate image: %w", tns.ErrNotSupported)
	}
	if flags&objectNoPrefix == 0 {
		n, ok := length()
		if !ok {
			return nil, false, errShort
		}
		pos += n
	}
	pos++ // the version of the XML
	if pos+4 > len(image) {
		return nil, false, errShort
	}
	xmlFlags := binary.BigEndian.Uint32(image[pos:])
	pos += 4
	if xmlFlags&xmlSkipNext4 != 0 {
		pos += 4
	}
	if pos > len(image) {
		return nil, false, errShort
	}
	switch {
	case xmlFlags&xmlString != 0:
		return image[pos:], false, nil
	case xmlFlags&xmlLOB != 0:
		return image[pos:], true, nil
	}
	return nil, false, fmt.Errorf("reading an XMLType: the flags %#x name neither text nor a CLOB: %w", xmlFlags, tns.ErrNotSupported)
}

// xmlClob returns the CLOB of an XMLType whose locator is locator, which 11g
// sends for an XMLType that a table stores as a CLOB. It reads nothing: a
// round trip in the middle of a fetch loses the rest of the fetch, so
// decodePrim loads it after the fetch, as it loads a CLOB column.
func xmlClob(locator []byte, conn codec.Conn) (any, error) {
	streamer := conn.NewLobStreamer()
	streamer.SetLocator(locator)
	clob := &codec.ClobParameter{}
	clob.Init()
	clob.BValue = locator
	clob.SetLobStreamer(streamer)
	return clob.Decode(conn)
}
