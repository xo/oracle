package ttc

import (
	"errors"
	"fmt"

	"github.com/xo/oracle/internal/tns"
)

type DBVersion struct {
	Info            string
	Text            string
	Number          uint16
	MajorVersion    int
	MinorVersion    int
	PatchsetVersion int
	// isDb10gR20OrHigher bool
	// isDb11gR10OrHigher bool
}

// ReadDBVersion write a request to get database version the read
// database version from network session
func ReadDBVersion(session *tns.Session) (*DBVersion, error) {
	session.ResetBuffer()
	session.PutTTCFunc(0x3, 0x3B)
	session.PutBytes(1)
	session.PutUint(0x100, 2, true, true)
	session.PutBytes(1, 1)
	if session.TTCVersion >= 11 {
		session.PutUint(1, 4, true, true)
	}
	err := session.Write()
	if err != nil {
		return nil, err
	}
	msg, err := session.ReadByte()
	if err != nil {
		return nil, err
	}
	if msg != 8 {
		return nil, errors.New(fmt.Sprintf("message code error: received code %d and expected code is 8", msg))
	}
	length, err := session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	info, err := session.ReadString(int(length))
	if err != nil {
		return nil, err
	}
	number, err := session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	version := (number>>24&0xFF)*1000 + (number>>20&0xF)*100 + (number>>12&0xF)*10 + (number >> 8 & 0xF)
	text := fmt.Sprintf("%d.%d.%d.%d.%d", number>>24&0xFF, number>>20&0xF,
		number>>12&0xF, number>>8&0xF, number&0xFF)

	ret := &DBVersion{
		Info:            info,
		Text:            text,
		Number:          uint16(version),
		MajorVersion:    int(number >> 24 & 0xFF),
		MinorVersion:    int(number >> 20 & 0xF),
		PatchsetVersion: int(number >> 8 & 0xF),
	}
	return ret, nil
}
