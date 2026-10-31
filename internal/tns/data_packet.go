package tns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
)

type DataPacket struct {
	Packet

	dataFlag uint16
	buffer   []byte
}

func (pck *DataPacket) bytes() []byte {
	output := pck.Packet.bytes()
	binary.BigEndian.PutUint16(output[8:], pck.dataFlag)
	ret := bytes.NewBuffer(output)
	if len(pck.buffer) > 0 {
		ret.Write(pck.buffer)
	}
	return ret.Bytes()
}

func newDataPacket(initialData []byte, sessionCtx *SessionContext, mu *sync.Mutex, dataFlag uint16) (*DataPacket, error) {
	var err error
	mu.Lock()
	defer mu.Unlock()
	if sessionCtx.nego != nil {
		initialData, err = sessionCtx.nego.WriteDataBuffer(initialData)
		if err != nil {
			return nil, err
		}
	}

	return &DataPacket{
		sessionCtx: sessionCtx,
		dataOffset: 0xA,
		length:     uint32(len(initialData)) + 0xA,
		packetType: DATA,
		flag:       0,
		dataFlag:   dataFlag,
		buffer:     initialData,
	}, nil
}

func newDataPacketFromData(packetData []byte, sessionCtx *SessionContext, mu *sync.Mutex) (*DataPacket, error) {
	mu.Lock()
	defer mu.Unlock()
	if len(packetData) < 0xA || PacketType(packetData[4]) != DATA {
		return nil, errors.New("not data packet")
	}
	pck := &DataPacket{
		sessionCtx: sessionCtx,
		dataOffset: 0xA,
		// length:     binary.BigEndian.Uint16(packetData),
		packetType: PacketType(packetData[4]),
		flag:       packetData[5],
		dataFlag:   binary.BigEndian.Uint16(packetData[8:]),
		buffer:     packetData[10:],
	}
	if sessionCtx.handshakeComplete && sessionCtx.Version >= 315 {
		pck.length = binary.BigEndian.Uint32(packetData)
	} else {
		pck.length = uint32(binary.BigEndian.Uint16(packetData))
	}
	if len(pck.buffer) > 1 && sessionCtx.nego != nil {
		var err error
		pck.buffer, err = sessionCtx.nego.ReadDataBuffer(pck.buffer)
		if err != nil {
			return nil, err
		}
	}
	return pck, nil
}
