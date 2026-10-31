package auth

import (
	"encoding/binary"

	"github.com/xo/oracle/internal/auth/ntlmssp"
	"github.com/xo/oracle/internal/config"
)

// ntlm is the NTSAuthenticator of the driver, which a connection uses when
// its configuration names none.
type ntlm struct{}

func (ntlm) NewNegotiateMessage(domain, machine string) ([]byte, error) {
	return ntlmssp.NewNegotiateMessage(domain, machine)
}

func (ntlm) ProcessChallenge(chaMsgData []byte, user, password string) ([]byte, error) {
	return ntlmssp.ProcessChallenge(chaMsgData, user, password)
}

func createNTSNegoPacket(nts config.NTSAuthenticator, domain, machine string) ([]byte, error) {
	packetData := []byte{
		0, 1, 0, 7, 0, 0, 0, 0, 0, 4, 0, 5, 2, 0, 0, 0,
		0, 4, 0, 4, 0, 0, 0, 9, 0, 4, 0, 4, 0, 0, 0, 2,
		0, 20, 0, 1, 2, 0, 0, 0, 4, 0, 0, 0, 2, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 1, 0, 0, 0, 0,
		0, 4, 0, 1, 55, 0, 0, 0, 0, 55, 0, 1,
	}
	ret := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x99, 0x0B, 0x20, 0x02, 0x00, 0x00, 0x01, 0x00}
	ret = append(ret, packetData...)
	sspiOffset := len(ret)
	negoData, err := nts.NewNegotiateMessage(domain, machine)
	if err != nil {
		return nil, err
	}
	ret = append(ret, negoData...)
	temp := make([]byte, 2)
	binary.BigEndian.PutUint16(temp, uint16(len(ret)))
	for x := range 2 {
		ret[x+4] = temp[x]
	}
	temp2 := make([]byte, 4)
	ntsDataLen := len(ret) - sspiOffset
	binary.LittleEndian.PutUint32(temp2, uint32(ntsDataLen))
	binary.BigEndian.PutUint16(temp, uint16(ntsDataLen))
	for x := range 4 {
		ret[sspiOffset-8+x] = temp2[x]
	}
	for x := range 2 {
		ret[sspiOffset-4+x] = temp[x]
	}
	return ret, nil
}

func createNTSAuthPacket(nts config.NTSAuthenticator, chaMsgData []byte, user, password string) ([]byte, error) {
	packetData := []byte{
		0, 1, 0, 2, 0, 0, 0, 0, 0, 4,
		0, 1, 55, 0, 0, 0, 0, 55, 0, 1,
	}
	ret := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x90, 0x0, 0x0, 0x0, 0x0, 0x0, 0x01, 0x0}
	ret = append(ret, packetData...)
	sspiOffset := len(ret)
	authData, err := nts.ProcessChallenge(chaMsgData, user, password)
	if err != nil {
		return nil, err
	}
	ret = append(ret, authData...)
	temp := make([]byte, 2)
	binary.BigEndian.PutUint16(temp, uint16(len(ret)))
	for x := range 2 {
		ret[x+4] = temp[x]
	}
	temp2 := make([]byte, 4)
	ntsDataLen := len(ret) - sspiOffset
	binary.LittleEndian.PutUint32(temp2, uint32(ntsDataLen))
	binary.BigEndian.PutUint16(temp, uint16(ntsDataLen))
	for x := range 4 {
		ret[sspiOffset-8+x] = temp2[x]
	}
	for x := range 2 {
		ret[sspiOffset-4+x] = temp[x]
	}
	return ret, nil
}
