package tns

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
)

type RefusePacket struct {
	Packet
	Err          *OracleError
	SystemReason uint8
	UserReason   uint8
	message      string
}

func (pck *RefusePacket) bytes() []byte {
	output := pck.Packet.bytes()
	output[8] = pck.SystemReason
	output[9] = pck.UserReason
	data := []byte(pck.message)
	binary.BigEndian.PutUint16(output[10:], uint16(len(data)))
	output = append(output, data...)
	return output
}

func newRefusePacketFromData(packetData []byte) *RefusePacket {
	if len(packetData) < 12 {
		return nil
	}
	dataLen := binary.BigEndian.Uint16(packetData[10:])
	var message string
	if uint16(len(packetData)) >= 12+dataLen {
		message = string(packetData[12 : 12+dataLen])
	}

	return &RefusePacket{
		dataOffset:   12,
		length:       uint32(binary.BigEndian.Uint16(packetData)),
		packetType:   PacketType(packetData[4]),
		flag:         0,
		SystemReason: packetData[9],
		UserReason:   packetData[8],
		message:      message,
	}
}

var errExtractRegexp = regexp.MustCompile(`\(\s*ERR\s*=\s*([0-9]+)\s*\)`)

var errorExtractRegexp = regexp.MustCompile(`\(\s*ERROR\s*=([A-Z0-9=()]+)`)

var codeExtractRegexp = regexp.MustCompile(`CODE\s*=\s*([0-9]+)`)

func (pck *RefusePacket) extractErrCode() {
	var err error
	pck.Err = NewOracleError(12564)
	if len(pck.message) == 0 {
		return
	}

	msg := strings.ToUpper(pck.message)
	matches := errExtractRegexp.FindStringSubmatch(msg)
	if len(matches) != 2 {
		return
	}

	strErrCode := matches[1]
	errCode, err := strconv.ParseInt(strErrCode, 10, 32)
	if err == nil {
		pck.Err = NewOracleError(int(errCode))
		return
	}

	matches = errorExtractRegexp.FindStringSubmatch(msg)
	if len(matches) != 2 {
		return
	}
	codeStr := matches[1]

	matches = codeExtractRegexp.FindStringSubmatch(codeStr)
	if len(matches) != 2 {
		return
	}

	strErrCode = matches[1]
	errCode, err = strconv.ParseInt(strErrCode, 10, 32)
	if err == nil {
		pck.Err = NewOracleError(int(errCode))
	}
}
