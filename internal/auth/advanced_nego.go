package auth

import (
	"encoding/binary"
	"errors"
	"net"

	"github.com/xo/oracle/internal/auth/security"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
)

const version = uint32(0x17000000)

type AdvNego struct {
	version      uint32
	authKerberos bool
	authNTS      bool
	comm         *AdvancedNegoComm
	clientInfo   *config.ClientInfo
	negoInfo     *config.AdvNegoServiceInfo
	serviceList  []negoService
	sessionKey   []byte
	iv           []byte
	oldIV        []byte
	cryptAlgo    security.OracleNetworkEncryption
	hashAlgo     security.OracleNetworkDataIntegrity
	keyFolding   bool
}

func NewAdvNego(session *tns.Session, config *config.ConnectionConfig) (*AdvNego, error) {
	output := &AdvNego{
		version:     version,
		comm:        &AdvancedNegoComm{session: session},
		clientInfo:  &config.ClientInfo,
		negoInfo:    &config.AdvNegoServiceInfo,
		serviceList: make([]negoService, 5),
	}
	var err error
	output.serviceList[1], err = newAuthService(output, output.negoInfo)
	if err != nil {
		return nil, err
	}
	output.serviceList[2], err = newEncryptService(output, output.negoInfo)
	if err != nil {
		return nil, err
	}
	output.serviceList[3], err = newDataIntegrityService(output, output.negoInfo)
	if err != nil {
		return nil, err
	}
	output.serviceList[4], err = newSupervisorService(output)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (nego *AdvNego) readHeader() (length, version, servCount int, err error) {
	var num int64
	num, err = nego.comm.session.ReadInt64(4, false, true)
	if err != nil {
		return
	}
	if num != 0xDEADBEEF {
		err = errors.New("advanced negotiation error: during receive header")
		return
	}
	length, err = nego.comm.session.ReadInt(2, false, true)
	if err != nil {
		return
	}
	version, err = nego.comm.session.ReadInt(4, false, true)
	if err != nil {
		return
	}
	servCount, err = nego.comm.session.ReadInt(2, false, true)
	if err != nil {
		return
	}
	var errFlags int
	errFlags, err = nego.comm.session.ReadInt(1, false, true)
	if err != nil {
		return
	}
	if errFlags != 0 {
		err = tns.NewOracleError(errFlags)
	}
	return
}

func (nego *AdvNego) writeHeader(length, servCount int) {
	nego.comm.session.PutInt(uint64(0xDEADBEEF), 4, true, false)
	nego.comm.session.PutInt(length, 2, true, false)
	nego.comm.session.PutInt(nego.version, 4, true, false)
	nego.comm.session.PutInt(servCount, 2, true, false)
	var errFlags = uint8(0)
	nego.comm.session.PutBytes(errFlags)
}

func (nego *AdvNego) readServiceHeader() (serviceType, serviceSubPacket int, err error) {
	serviceType, err = nego.comm.session.ReadInt(2, false, true)
	if err != nil {
		return
	}
	serviceSubPacket, err = nego.comm.session.ReadInt(2, false, true)
	if err != nil {
		return
	}
	var errFlags int
	errFlags, err = nego.comm.session.ReadInt(4, false, true)
	if err != nil {
		return
	}
	if errFlags != 0 {
		err = tns.NewOracleError(errFlags)
	}
	return
}

func (nego *AdvNego) IsNTSAuth() bool {
	return nego.authNTS
}

func (nego *AdvNego) IsKerberosAuth() bool {
	return nego.authKerberos
}

func (nego *AdvNego) Read() error {
	_, version, count, err := nego.readHeader()
	if err != nil {
		return err
	}
	nego.version = uint32(version)
	for range count {
		serviceType, subPacket, err := nego.readServiceHeader()
		if err != nil {
			return err
		}
		err = nego.serviceList[serviceType].readServiceData(subPacket)
		if err != nil {
			return err
		}
		err = nego.serviceList[serviceType].validateResponse()
		if err != nil {
			return err
		}
	}
	nego.authKerberos = false
	nego.authNTS = false
	if authServ, ok := nego.serviceList[1].(*authService); ok {
		if authServ.active {
			if authServ.serviceName == "KERBEROS5" {
				nego.authKerberos = true
			} else if authServ.serviceName == "NTS" {
				nego.authNTS = true
			}
		}
	}
	size := 0
	numService := 0
	if dataServ, ok := nego.serviceList[3].(*dataIntegrityService); ok {
		if len(dataServ.publicKey) > 0 {
			size = size + 12 + len(dataServ.publicKey)
			numService++
		}
	}
	if nego.authKerberos {
		size += 37
		numService++
	}
	if nego.authNTS {
		size += 130
		numService++
	}
	if numService == 0 {
		return nil
	}
	nego.comm.session.ResetBuffer()
	nego.writeHeader(size+13, numService)
	if dataServ, ok := nego.serviceList[3].(*dataIntegrityService); ok {
		if len(dataServ.publicKey) > 0 {
			dataServ.writeHeader(1)
			nego.comm.writeBytes(dataServ.publicKey)
		}
	}
	if nego.authKerberos {
		auth := nego.negoInfo.Kerberos
		if auth == nil {
			return errors.New("kerberos authentication needs Config.Kerberos")
		}

		if authServ, ok := nego.serviceList[1].(*authService); ok {
			authServ.writeHeader(4)
			nego.comm.writeVersion(authServ.getVersion())
			nego.comm.writeUB4(9)
			nego.comm.writeUB4(2)
			nego.comm.writeUB1(1)
			err = nego.comm.session.Write()
			if err != nil {
				return err
			}
			return nego.kerberosHandshake(auth, authServ)
		}
	}
	if nego.authNTS {
		var nts config.NTSAuthenticator = ntlm{}
		if nego.negoInfo.NTS != nil {
			nts = nego.negoInfo.NTS
		}
		ntsPacket, err := createNTSNegoPacket(nts, nego.clientInfo.DomainName, nego.clientInfo.HostName)
		if err != nil {
			return err
		}
		nego.comm.session.ResetBuffer()
		nego.comm.session.PutBytes(ntsPacket...)
		err = nego.comm.session.Write()
		if err != nil {
			return err
		}
		ntsHeader, err := nego.comm.session.ReadBytes(33)
		if err != nil {
			return err
		}
		sizeOffset := len(ntsHeader) - 8
		chaSize := binary.LittleEndian.Uint32(ntsHeader[sizeOffset : sizeOffset+4])
		chaData, err := nego.comm.session.ReadBytes(int(chaSize))
		if err != nil {
			return err
		}
		ntsPacket, err = createNTSAuthPacket(nts, chaData, nego.clientInfo.OSUserName,
			nego.clientInfo.OSPassword)
		if err != nil {
			return err
		}
		nego.comm.session.ResetBuffer()
		nego.comm.session.PutBytes(ntsPacket...)
		err = nego.comm.session.Write()
		if err != nil {
			return err
		}
		return nil
	}
	return nego.comm.session.Write()
}

func (nego *AdvNego) Write() error {
	nego.comm.session.ResetBuffer()
	size := 0
	for i := 1; i < 5; i++ {
		size = size + 8 + nego.serviceList[i].getServiceDataLength()
	}
	// size += 13
	nego.writeHeader(13+size, 4)
	err := nego.serviceList[4].writeServiceData()
	if err != nil {
		return err
	}
	err = nego.serviceList[1].writeServiceData()
	if err != nil {
		return err
	}
	err = nego.serviceList[2].writeServiceData()
	if err != nil {
		return err
	}
	err = nego.serviceList[3].writeServiceData()
	if err != nil {
		return err
	}
	return nego.comm.session.Write()
}

func (nego *AdvNego) StartServices() error {
	err := nego.serviceList[3].activateAlgorithm()
	if err != nil {
		return err
	}
	err = nego.serviceList[2].activateAlgorithm()
	if err != nil {
		return err
	}
	err = nego.serviceList[1].activateAlgorithm()
	if err != nil {
		return err
	}
	err = nego.serviceList[4].activateAlgorithm()
	if err != nil {
		return err
	}
	return nil
}

func (nego *AdvNego) IsNewVersion(serviceType int) bool {
	if serviceType < len(nego.serviceList) {
		return nego.serviceList[serviceType].isNewVersion()
	}

	return isNew(nego.version)
}

func (nego *AdvNego) kerberosHandshake(kerberos config.KerberosAuthenticator, authServ *authService) error {
	_, _, count, err := nego.readHeader()
	if err != nil {
		return err
	}
	for i := 0; i < count; i++ {
		_, _, err = nego.readServiceHeader()
		if err != nil {
			return err
		}
	}
	serviceName, err := nego.comm.readString()
	if err != nil {
		return err
	}
	serverHostName, err := nego.comm.readString()
	if err != nil {
		return err
	}
	if len(serviceName) == 0 {
		return errors.New("kerberos negotiation error: Service Name not received")
	}
	if len(serverHostName) == 0 {
		return errors.New("kerberos negotiation error: Server hostname not received")
	}
	ticketData, err := kerberos.Authenticate(serverHostName, serviceName)
	if err != nil {
		return err
	}
	// get host ip address
	localAddress, err := getHostIPAddress()
	if err != nil {
		return err
	}
	// if address is ipv6 then num1 = 24 otherwise = 2
	num1 := 2
	if len(localAddress) > 4 {
		num1 = 24
	}
	nego.comm.session.ResetBuffer()
	// send ano header(length of ticket + 43 + length of address, 1 , 0)
	nego.writeHeader(len(ticketData)+43+len(localAddress), 1)
	// send header(4)
	authServ.writeHeader(4)
	nego.comm.writeUB2(num1)
	// send ub4 = length of address
	nego.comm.writeUB4(len(localAddress))
	// send bytes address bytes
	nego.comm.writeBytes(localAddress)
	// send bytes ticket
	nego.comm.writeBytes(ticketData)
	// write
	err = nego.comm.session.Write()
	if err != nil {
		return err
	}
	// read ano header
	_, _, count, err = nego.readHeader()
	if err != nil {
		return err
	}
	for index := 0; index < count; index++ {
		_, _, err := nego.readServiceHeader()
		if err != nil {
			return err
		}
	}
	// get packet header (2)
	_, err = nego.comm.readPacketHeader(2)
	if err != nil {
		return err
	}
	// num2 = get ub1
	_, err = nego.comm.session.ReadByte()
	if err != nil {
		return err
	}
	// receive byte array
	_, err = nego.comm.readBytes()
	if err != nil {
		return err
	}
	// send ano header (25,1, 0)
	nego.comm.session.ResetBuffer()
	nego.writeHeader(25, 1)
	// as.send header(1)
	authServ.writeHeader(1)
	// send packet header(0, 1)
	nego.comm.writePacketHeader(0, 1)
	// write
	return nego.comm.session.Write()
}

func getHostIPAddress() (net.IP, error) {
	adders, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	for _, address := range adders {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.To4(), nil
			}
			if ipnet.IP.To16() != nil {
				return ipnet.IP.To16(), nil
			}
		}
	}
	return nil, errors.New("advanced negotiation error: during get local ip address")
}

func (nego *AdvNego) WriteDataBuffer(data []byte) ([]byte, error) {
	if nego.hashAlgo != nil {
		hashData := nego.hashAlgo.Compute(data)
		data = append(data, hashData...)
	}
	var err error
	if nego.cryptAlgo != nil {
		nego.comm.session.LogPacket("write packet, decrypted", data)
		data, err = nego.cryptAlgo.Encrypt(data)
		if err != nil {
			return nil, err
		}
	}
	if nego.hashAlgo != nil || nego.cryptAlgo != nil {
		foldingKey := uint8(0)
		if nego.keyFolding {
			foldingKey = uint8(1)
		}
		data = append(data, foldingKey)
	}
	return data, nil
}

func (nego *AdvNego) ReadDataBuffer(data []byte) ([]byte, error) {
	var err error
	if nego.cryptAlgo != nil || nego.hashAlgo != nil {
		data = data[:len(data)-1]
	}
	if nego.cryptAlgo != nil {
		data, err = nego.cryptAlgo.Decrypt(data)
		if err != nil {
			return nil, err
		}
		nego.comm.session.LogPacket("read packet, decrypted", data)
	}
	if nego.hashAlgo != nil {
		data, err = nego.hashAlgo.Validate(data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (nego *AdvNego) SetKeyFolding(key []byte, isExternalAuth, isSys bool) error {
	nego.keyFolding = false
	if isExternalAuth {
		if nego.IsNTSAuth() && (!isSys) {
			return nil
		}
	}
	if len(key) == 0 || len(nego.sessionKey) == 0 || len(nego.iv) == 0 {
		return nil
	}

	length := min(len(key), len(nego.sessionKey))
	for i := 0; i < length; i++ {
		nego.sessionKey[i] ^= key[i]
	}
	if nego.IsNewVersion(EncryptionServiceID) || nego.IsNewVersion(DataIntegrityServiceID) {
		length = min(len(key), len(nego.iv))
		for i := 0; i < length; i++ {
			nego.iv[i] ^= key[i]
		}
	}
	if nego.cryptAlgo != nil || nego.hashAlgo != nil {
		err := nego.StartServices()
		if err != nil {
			return err
		}
		nego.keyFolding = true
	}
	return nil
}

func (nego *AdvNego) Reset() error {
	var err error
	if nego.hashAlgo != nil {
		err = nego.hashAlgo.Init()
		if err != nil {
			return err
		}
	}
	if nego.cryptAlgo != nil {
		err = nego.cryptAlgo.Reset()
		if err != nil {
			return err
		}
	}
	return nil
}
