package ttc

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/types"

	"github.com/xo/oracle/internal/auth"
	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/tns"
)

type ConnectionState int

const (
	Closed ConnectionState = 0
	Opened ConnectionState = 1
)

type LogonMode int

const (
	NoNewPass   LogonMode = 0x1
	SysDba      LogonMode = 0x20
	SysOper     LogonMode = 0x40
	SysAsm      LogonMode = 0x00400000
	SysBackup   LogonMode = 0x01000000
	SysDg       LogonMode = 0x02000000
	SysKm       LogonMode = 0x04000000
	SysRac      LogonMode = 0x08000000
	UserAndPass LogonMode = 0x100
	WithNewPass LogonMode = 0x2
	PROXY       LogonMode = 0x400
)

// from GODROR
const wrapResultset = "--WRAP-RESULTSET--"

const createLob = "--CREATE-LOB-STREAM--"
const connRef = "--GET-CONNECTION-REF--"

// Querier is the QueryContext of sql.Conn.
type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// ///

type NLSData struct {
	Calender        string `db:"p_nls_calendar,,40,out"`
	Comp            string `db:"p_nls_comp,,40,out"`
	Language        string
	LengthSemantics string `db:"p_nls_length_semantics,,40,out"`
	NCharConvExcep  string `db:"p_nls_nchar_conv_excep,,40,out"`
	NCharConvImp    string
	DateLang        string `db:"p_nls_date_lang,,40,out"`
	Sort            string `db:"p_nls_sort,,40,out"`
	Currency        string `db:"p_nls_currency,,40,out"`
	DateFormat      string `db:"p_nls_date_format,,40,out"`
	TimeFormat      string
	IsoCurrency     string `db:"p_nls_iso_currency,,40,out"`
	NumericChars    string `db:"p_nls_numeric_chars,,40,out"`
	DualCurrency    string `db:"p_nls_dual_currency,,40,out"`
	UnionCurrency   string
	Timestamp       string `db:"p_nls_timestamp,,48,out"`
	TimestampTZ     string `db:"p_nls_timestamp_tz,,56,out"`
	TTimezoneFormat string
	NTimezoneFormat string
	Territory       string
	Charset         string
}
type Connection struct {
	State             ConnectionState
	LogonMode         LogonMode
	autoCommit        bool
	logger            *slog.Logger
	connOption        *config.ConnectionConfig
	session           *tns.Session
	tcpNego           *TCPNego
	dataNego          *DataTypeNego
	authObject        *AuthObject
	SessionProperties map[string]string
	dBVersion         *DBVersion
	sessionID         int
	serialID          int
	transactionID     []byte
	sStrConv          *charset.Converter
	nStrConv          *charset.Converter
	cStrConv          *charset.Converter
	NLSData           NLSData
	goTypeCoder       map[reflect.Type]codec.OracleParameterCoder
	oracleTypeCoder   map[uint16]codec.OracleParameterCoder
	nameTypeCoder     map[string]codec.OracleParameterCoder
	//typeDecoder       map[uint16]type_coder.OracleTypeDecoder
	cusTyp map[string]types.Object
	maxLen struct {
		varchar   int64
		nvarchar  int64
		raw       int64
		number    int64
		date      int64
		timestamp int64
	}
	bad                      bool
	dbTimeZone               *time.Location // equivalent to database timezone used for timestamp with local timezone
	dbServerTimeZone         *time.Location // equivalent to timezone of the server carry the database
	dbServerTimeZoneExplicit *time.Location
	temporaryLocs            []types.Locator
	token                    []byte
	tokenPrivateKey          []byte
	connectionCookie         *ConnectionCookie
	cookies                  *cookieJar
	newKey                   []byte
}

type ConnectionProperties struct {
	cusTyp map[string]types.Object
	maxLen struct {
		varchar   int
		nvarchar  int
		raw       int
		number    int
		date      int
		timestamp int
	}
	dbTimeZone               *time.Location // equivalent to database timezone used for timestamp with local timezone
	dbServerTimeZone         *time.Location // equivalent to timezone of the server carry the database
	dbServerTimeZoneExplicit *time.Location
}

func (conn *Connection) buildParameterCoderMap(drv *Registry) {
	conn.goTypeCoder = drv.goTypeCoder
	conn.nameTypeCoder = drv.nameTypeCoder
	conn.oracleTypeCoder = drv.oracleTypeCoder

}

// NLS return NLS properties of the connection.
// this function is left from v1. but v2 is using another method
func (conn *Connection) NLS() (*NLSData, error) {
	// we read from sys.nls_session_parameters ONCE
	cmdText := `
	BEGIN
		SELECT 
			MAX(CASE WHEN PARAMETER='NLS_CALENDAR' THEN VALUE END) AS NLS_CALENDAR,
			MAX(CASE WHEN PARAMETER='NLS_COMP' THEN VALUE END) AS NLS_COMP,
			MAX(CASE WHEN PARAMETER='NLS_LENGTH_SEMANTICS' THEN VALUE END) AS NLS_LENGTH_SEMANTICS,
			MAX(CASE WHEN PARAMETER='NLS_NCHAR_CONV_EXCP' THEN VALUE END) AS NLS_NCHAR_CONV_EXCP,
			MAX(CASE WHEN PARAMETER='NLS_DATE_LANGUAGE' THEN VALUE END) AS NLS_DATE_LANGUAGE,
			MAX(CASE WHEN PARAMETER='NLS_SORT' THEN VALUE END) AS NLS_SORT,
			MAX(CASE WHEN PARAMETER='NLS_CURRENCY' THEN VALUE END) AS NLS_CURRENCY,
			MAX(CASE WHEN PARAMETER='NLS_DATE_FORMAT' THEN VALUE END) AS NLS_DATE_FORMAT,
			MAX(CASE WHEN PARAMETER='NLS_ISO_CURRENCY' THEN VALUE END) AS NLS_ISO_CURRENCY,
			MAX(CASE WHEN PARAMETER='NLS_NUMERIC_CHARACTERS' THEN VALUE END) AS NLS_NUMERIC_CHARACTERS,
			MAX(CASE WHEN PARAMETER='NLS_DUAL_CURRENCY' THEN VALUE END) AS NLS_DUAL_CURRENCY,
			MAX(CASE WHEN PARAMETER='NLS_TIMESTAMP_FORMAT' THEN VALUE END) AS NLS_TIMESTAMP_FORMAT,
			MAX(CASE WHEN PARAMETER='NLS_TIMESTAMP_TZ_FORMAT' THEN VALUE END) AS NLS_TIMESTAMP_TZ_FORMAT
			into :p_nls_calendar, :p_nls_comp, :p_nls_length_semantics, :p_nls_nchar_conv_excep, 
				:p_nls_date_lang, :p_nls_sort, :p_nls_currency, :p_nls_date_format, :p_nls_iso_currency,
				:p_nls_numeric_chars, :p_nls_dual_currency, :p_nls_timestamp, :p_nls_timestamp_tz
		FROM
			sys.nls_session_parameters
		;
	END;`
	stmt := NewStmt(cmdText, conn)
	defer func(stmt *Stmt) {
		_ = stmt.Close()
	}(stmt)
	_, err := stmt.Exec([]driver.Value{&conn.NLSData})
	if err != nil {
		return nil, err
	}

	/*
		for _, par := range stmt.Pars {
			if par.Name == "p_nls_calendar" {
				conn.NLSData.Calender = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_comp" {
				conn.NLSData.Comp = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_length_semantics" {
				conn.NLSData.LengthSemantics = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_nchar_conv_excep" {
				conn.NLSData.NCharConvExcep = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_date_lang" {
				conn.NLSData.DateLang = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_sort" {
				conn.NLSData.Sort = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_currency" {
				conn.NLSData.Currency = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_date_format" {
				conn.NLSData.DateFormat = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_iso_currency" {
				conn.NLSData.IsoCurrency = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_numeric_chars" {
				conn.NLSData.NumericChars = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_dual_currency" {
				conn.NLSData.DualCurrency = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_timestamp" {
				conn.NLSData.Timestamp = conn.strConv.Decode(par.BValue)
			} else if par.Name == "p_nls_timestamp_tz" {
				conn.NLSData.TimestampTZ = conn.strConv.Decode(par.BValue)
			}
		}
	*/

	return &conn.NLSData, nil
}

// Prepare take a query string and create a stmt object
func (conn *Connection) Prepare(query string) (driver.Stmt, error) {
	conn.logger.Debug("prepare", "query", query)
	return NewStmt(query, conn), nil
}

// Ping test if connection is online
func (conn *Connection) Ping(ctx context.Context) error {
	conn.logger.Debug("ping")
	conn.session.ResetBuffer()
	done := conn.session.StartContext(ctx)
	defer conn.session.EndContext(done)
	return (&simpleObject{
		connection:  conn,
		operationID: 0x93,
		data:        nil,
	}).exec()
}

func (conn *Connection) MaxStringLength() int64 {
	if conn.maxLen.varchar == 0 {
		return 0x7FFF
	}
	return conn.maxLen.varchar
}
func (conn *Connection) MaxRawLength() int64 {
	if conn.maxLen.raw == 0 {
		return 0x7FFF
	}
	return conn.maxLen.raw
}
func (conn *Connection) getDefaultCharsetID() int {

	return conn.tcpNego.ServerCharset
}
func (conn *Connection) DefaultStringCoder() (*charset.Converter, error) {
	return conn.StringCoder(conn.getDefaultCharsetID(), 1)
}

func (conn *Connection) StringCoder(charsetID, charsetForm int) (*charset.Converter, error) {
	errStringCoder := fmt.Errorf("server requested charset id: %d which is not supported by the driver", charsetID)
	if charsetID == 0 {
		if charsetForm == 1 {
			if conn.cStrConv != nil {
				return conn.cStrConv, nil
			}
			if conn.sStrConv != nil {
				return conn.sStrConv, nil
			}
		} else {
			if conn.nStrConv != nil {
				return conn.nStrConv, nil
			}
		}
		return nil, errStringCoder
	}
	if conn.cStrConv != nil && charsetID == conn.cStrConv.LangID {
		return conn.cStrConv, nil
	}
	if conn.sStrConv != nil && charsetID == conn.sStrConv.LangID {
		if conn.cStrConv != nil {
			return conn.cStrConv, nil
		}
		return conn.sStrConv, nil
	}

	if conn.nStrConv != nil && charsetID == conn.nStrConv.LangID {
		return conn.nStrConv, nil
	}
	temp := charset.New(charsetID)
	if temp == nil {
		return nil, errStringCoder
	}
	return temp, nil
}

func (conn *Connection) ServerStringCoder() *charset.Converter {
	return conn.sStrConv
}
func (conn *Connection) ServerNStringCoder() *charset.Converter {
	return conn.nStrConv
}
func (conn *Connection) Logoff() error {
	conn.logger.Debug("logoff")
	session := conn.session
	session.ResetBuffer()
	session.PutTTCFunc(3, 9)
	err := session.Write()
	if err != nil {
		return err
	}
	return conn.read()
}

func (conn *Connection) read() error {
	loop := true
	session := conn.session
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		err = conn.ProcessTCCResponse(msg)
		if err != nil {
			return err
		}
		if msg == 4 || msg == 9 {
			loop = false
		}
	}
	return nil
}

// Open the connection = bring it online

// OpenWithContext open the connection with timeout context
func (conn *Connection) OpenWithContext(ctx context.Context) error {
	logger := conn.logger
	switch conn.connOption.DBAPrivilege {
	case config.SYSDBA:
		conn.LogonMode |= SysDba
	case config.SYSOPER:
		conn.LogonMode |= SysOper
	case config.SYSASM:
		conn.LogonMode |= SysAsm
	case config.SYSBACKUP:
		conn.LogonMode |= SysBackup
	case config.SYSDG:
		conn.LogonMode |= SysDg
	case config.SYSKM:
		conn.LogonMode |= SysKm
	case config.SYSRAC:
		conn.LogonMode |= SysRac
	default:
		conn.LogonMode = 0
	}
	conn.connOption.ResetServerIndex()
	conn.session = tns.NewSession(conn.connOption, conn.logger)
	// the packets of the logon carry the encrypted session key and password,
	// so the logger sees only their length until the logon ends (D14)
	conn.session.HidePackets(true)
	W := conn.connOption.Wallet
	if conn.connOption.SSL && W != nil {
		err := conn.session.LoadSSLData(W.Certificates, W.PrivateKeys, W.CertificateRequests)
		if err != nil {
			return err
		}
	}
	session := conn.session
	// start check for context
	done := session.StartContext(ctx)
	defer session.EndContext(done)
	err := session.Connect(ctx)
	if err != nil {
		return err
	}
	// advanced negotiation
	var ano *auth.AdvNego = nil
	if session.Context.ACFL0&1 != 0 && session.Context.ACFL0&4 == 0 && session.Context.ACFL1&8 == 0 {
		logger.Debug("advanced negotiation")
		ano, err = auth.NewAdvNego(session, conn.connOption)
		if err != nil {
			return err
		}
		err = ano.Write()
		if err != nil {
			return err
		}
		err = ano.Read()
		if err != nil {
			return err
		}
		err = ano.StartServices()
		if err != nil {
			return err
		}
	}
	conn.session.Context.SetAdvancedNegotiator(ano)
	// if fast login enabled this means new features are available
	if conn.session.Context.FastAuthEnabled {
		session.UseBigClrChunks = true
		session.ClrChunkSize = 0x7FFF
		session.TTCVersion = 24
		session.HasFSAPCapability = true
	}
	if conn.isFastLoginEnabled() {
		logger.Debug("fast authentication")
		session.ResetBuffer()
		cookie := lookupCookie(conn)
		if cookie != nil {
			conn.connectionCookie = cookie
			logger.Debug("fast authentication with a cookie")
			conn.tcpNego = &TCPNego{
				conn:                  conn,
				ServerFlags:           cookie.ServerFlags,
				ServerCharset:         cookie.ServerCharset,
				ServerNCharset:        cookie.ServerNCharset,
				OracleVersion:         cookie.OracleVersion,
				ProtocolServerString:  cookie.ProtocolServerString,
				ServerCompileTimeCaps: cookie.ServerCompileTimeCaps,
				ServerRuntimeCaps:     cookie.ServerRuntimeCaps,
			}
			if conn.sStrConv == nil {
				conn.sStrConv = charset.New(cookie.ServerCharset)
			}
			if conn.nStrConv == nil {
				conn.nStrConv = charset.New(cookie.ServerNCharset)
			}
			session.StrConv = conn.sStrConv
			session.PutBytes(0x22, 1, 0, 0)
			session.PutBytes([]byte("OracleClientGo\x00")...)
			cookie.writeTTICookie(session)
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dataNego.writeMessage()
		} else {
			logger.Debug("fast authentication with no cookie")
			session.PutBytes(0x22, 1, 0, 0)
			conn.tcpNego = &TCPNego{conn: conn, ServerFlags: 3, ServerCharset: 0x369, ServerNCharset: 0x7D0}
			conn.tcpNego.writeMessage()
			session.PutUint(conn.tcpNego.ServerCharset, 2, false, false)
			session.PutBytes(conn.tcpNego.ServerFlags)
			session.PutUint(conn.tcpNego.ServerNCharset, 2, false, false)
			session.PutBytes(session.TTCVersion)
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dataNego.writeMessage()
		}
	} else {
		err = conn.protocolNegotiation()
		if err != nil {
			return err
		}
		err = conn.dataTypeNegotiation()
		if err != nil {
			return err
		}

	}

	err = conn.doAuth()
	if errors.Is(err, tns.ErrConnReset) {
		if conn.isFastLoginEnabled() {
			err = conn.tcpNego.readMessage()
			if err != nil {
				return err
			}
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dbTimeZone, err = conn.dataNego.read()
			if err != nil {
				return err
			}
		}
		err = conn.read()
	}
	if conn.connectionCookie != nil && (errors.Is(err, driver.ErrBadConn) || errors.Is(err, io.EOF)) {
		// if cookie based remove cookie and reconnect
		logger.Info("reconnect without the fast login cookie", "error", err)
		deleteCookie(conn)
		conn.connectionCookie = nil
		if conn.session != nil {
			conn.session.Disconnect()
		}
		return conn.OpenWithContext(ctx)
	}
	if err != nil {
		return err
	}
	if ano != nil {
		externalAuth := len(conn.connOption.UserID) == 0 || len(conn.connOption.Password) == 0
		err = ano.SetKeyFolding(conn.newKey, externalAuth, conn.LogonMode&0x20 > 0 || conn.LogonMode&0x40 > 0)
		if err != nil {
			return err
		}
	}

	conn.State = Opened
	conn.session.ServerFlags = conn.tcpNego.ServerFlags
	if conn.connectionCookie == nil && conn.isFastLoginEnabled() && conn.tcpNego != nil {
		saveCookie(conn)
	}
	conn.dBVersion, err = ReadDBVersion(conn.session)
	if err != nil {
		return err
	}
	conn.session.SetConnected()
	conn.session.HidePackets(false)
	logger.Debug("connected", "version", conn.dBVersion.Text)
	sessionID, err := strconv.ParseUint(conn.SessionProperties["AUTH_SESSION_ID"], 10, 32)
	if err != nil {
		return err
	}
	conn.sessionID = int(sessionID)
	serialNum, err := strconv.ParseUint(conn.SessionProperties["AUTH_SERIAL_NUM"], 10, 32)
	if err != nil {
		return err
	}
	conn.serialID = int(serialNum)
	conn.connOption.InstanceName = conn.SessionProperties["AUTH_SC_INSTANCE_NAME"]
	conn.connOption.ServiceName = conn.SessionProperties["AUTH_SC_SERVICE_NAME"]
	conn.connOption.DomainName = conn.SessionProperties["AUTH_SC_DB_DOMAIN"]
	conn.connOption.DBName = conn.SessionProperties["AUTH_SC_DBUNIQUE_NAME"]
	if len(conn.NLSData.Language) == 0 {
		_, err = conn.NLS()
		if err != nil {
			logger.Debug("reading the NLS parameters", "error", err)
		}
	}
	conn.getDBServerTimeZone(ctx)
	return nil
}

func (conn *Connection) getDBServerTimeZone(ctx context.Context) {

	if conn.connOption.DatabaseInfo.Location != "" {
		loc, err := time.LoadLocation(conn.connOption.DatabaseInfo.Location)
		if err == nil {
			conn.dbServerTimeZone = loc
			return
		}
		conn.logger.Debug("loading the location", "error", err)
	}

	if conn.dbServerTimeZoneExplicit != nil {
		conn.dbServerTimeZone = conn.dbServerTimeZoneExplicit
		return
	}

	var current time.Time
	err := conn.QueryRowContext(ctx, "SELECT SYSTIMESTAMP FROM DUAL", nil).Scan(&current)
	if err != nil {
		conn.dbServerTimeZone = time.UTC
	}
	conn.dbServerTimeZone = current.Location()
}

// Begin is the form of BeginTx with no context, which database/sql does not
// call, because Connection implements driver.ConnBeginTx.
func (conn *Connection) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("beginning a transaction without a context: %w", tns.ErrNotSupported)
}

// BeginTx begins a transaction. Oracle has read committed, its default, and
// serializable, and a read only transaction. Any other isolation level fails
// with an error that wraps ErrNotSupported. A read only transaction is read
// committed, because Oracle has no read only serializable transaction.
func (conn *Connection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	var stmt string
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault, sql.LevelReadCommitted:
		if opts.ReadOnly {
			stmt = "SET TRANSACTION READ ONLY"
		}
	case sql.LevelSerializable:
		if opts.ReadOnly {
			return nil, fmt.Errorf("beginning a read only serializable transaction: %w", tns.ErrNotSupported)
		}
		stmt = "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE"
	default:
		return nil, fmt.Errorf("beginning a transaction with the isolation level %v: %w", sql.IsolationLevel(opts.Isolation), tns.ErrNotSupported)
	}
	conn.logger.Debug("begin")
	conn.autoCommit = false
	if stmt != "" {
		if _, err := conn.ExecContext(ctx, stmt, nil); err != nil {
			conn.autoCommit = true
			return nil, fmt.Errorf("beginning a transaction: %w", err)
		}
	}
	return &Transaction{conn: conn}, nil
}

// NewConnection create a new connection from databaseURL string or configuration
func NewConnection(cfg *config.ConnectionConfig) (*Connection, error) {
	var err error
	if cfg == nil {
		return nil, errors.New("database url or configuration is required")
	}
	var token, tokenPrivateKey []byte
	if len(cfg.TokenFile) > 0 {
		token, err = os.ReadFile(cfg.TokenFile)
		if err != nil {
			return nil, err
		}
	}
	if len(cfg.TokenPrivateKeyFile) > 0 {
		tokenPrivateKey, err = os.ReadFile(cfg.TokenPrivateKeyFile)
		if err != nil {
			return nil, err
		}
	}
	if len(token) > 0 && len(tokenPrivateKey) == 0 {
		return nil, errors.New("token private key file is required")
	}
	temp := new(config.ConnectionConfig)
	*temp = *cfg
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connection{
		State:           Closed,
		connOption:      temp,
		cStrConv:        charset.New(cfg.CharsetID),
		logger:          logger,
		autoCommit:      true,
		oracleTypeCoder: nil,
		cusTyp:          map[string]types.Object{},
		maxLen: struct {
			varchar   int64
			nvarchar  int64
			raw       int64
			number    int64
			date      int64
			timestamp int64
		}{varchar: 0x7FFF, nvarchar: 0x7FFF, raw: 0x7FFF, number: 0x16, date: 0xB, timestamp: 0xB},
		token:           token,
		tokenPrivateKey: tokenPrivateKey,
	}, nil
}

func (conn *Connection) freeTemporaryLobs() error {
	if len(conn.temporaryLocs) == 0 {
		return nil
	}
	conn.logger.Debug("free temporary LOBs", "count", len(conn.temporaryLocs))
	session := conn.session
	freeTemp := func(locators []types.Locator) {
		totalLen := 0
		for _, locator := range locators {
			totalLen += len(locator)
		}
		session.PutTTCFunc(0x11, 0x60)
		session.PutBytes(1)
		session.PutUint(totalLen, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0)
		session.PutUint(0x80111, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		for _, locator := range locators {
			session.PutBytes(locator...)
		}
	}
	start := 0
	end := 0
	session.ResetBuffer()
	for start < len(conn.temporaryLocs) {
		end = min(start+25000, len(conn.temporaryLocs))
		freeTemp(conn.temporaryLocs[start:end])
		start += end
	}
	session.PutTTCFunc(0x3, 0x93)
	err := session.Write()
	if err != nil {
		return err
	}
	err = conn.read()
	if err == nil {
		conn.temporaryLocs = nil
	}
	return err
}

// Close the connection by disconnect network session
func (conn *Connection) Close() (err error) {
	err = conn.freeTemporaryLobs()
	if err != nil {
		conn.logger.Debug("freeing temporary LOBs", "error", err)
	}
	logger := conn.logger
	logger.Debug("close")
	if conn.session != nil {
		err = conn.Logoff()
		if err != nil {
			logger.Debug("logoff", "error", err)
		}
		conn.session.Disconnect()
		conn.session = nil
	}
	conn.State = Closed
	conn.logger.Debug("closed")
	return
}

func (conn *Connection) isFastLoginEnabled() bool {
	return conn.session.Context.FastAuthEnabled && conn.connOption.FastLogin
}

// doAuth a login step that occur during open connection
func (conn *Connection) doAuth() error {
	conn.logger.Debug("authenticate")
	authObject := &AuthObject{
		conn:       conn,
		tcpNego:    conn.tcpNego,
		usePadding: false,
	}
	if !conn.isFastLoginEnabled() {
		conn.session.ResetBuffer()
	}
	if len(conn.connOption.UserID) > 0 && len(conn.connOption.Password) > 0 {
		conn.session.PutTTCFunc(0x3, 0x76)
		conn.session.PutBytes(1)
		conn.session.PutUint(len(conn.connOption.UserID), 4, true, true)
		conn.LogonMode = conn.LogonMode | NoNewPass
		conn.session.PutUint(int(conn.LogonMode), 4, true, true)
		conn.session.PutBytes(1, 1, 5, 1, 1)
		if len(conn.connOption.UserID) > 0 {
			conn.session.PutString(conn.connOption.UserID)
		}
		conn.session.PutKeyValString("AUTH_TERMINAL", conn.connOption.ClientInfo.HostName, 0)
		conn.session.PutKeyValString("AUTH_PROGRAM_NM", conn.connOption.ClientInfo.ProgramName, 0)
		conn.session.PutKeyValString("AUTH_MACHINE", conn.connOption.ClientInfo.HostName, 0)
		conn.session.PutKeyValString("AUTH_PID", fmt.Sprintf("%d", conn.connOption.ClientInfo.PID), 0)
		conn.session.PutKeyValString("AUTH_SID", conn.connOption.ClientInfo.OSUserName, 0)
		err := conn.session.Write()
		if err != nil {
			return err
		}

		if conn.isFastLoginEnabled() {
			if conn.connectionCookie == nil {
				err = conn.tcpNego.readMessage()
				if err != nil {
					return err
				}
				conn.tcpNego.ServerFlags |= 2
			}
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dbTimeZone, err = conn.dataNego.read()
			if err != nil {
				return err
			}
		}
		err = authObject.read()
		if err != nil {
			return err
		}
		conn.session.ResetBuffer()
	}
	// if proxyAuth ==> mode |= PROXY
	err := authObject.Write()
	if err != nil {
		return err
	}
	if (len(conn.connOption.UserID) == 0 || len(conn.connOption.Password) == 0) && conn.isFastLoginEnabled() {
		if conn.connectionCookie == nil {
			err = conn.tcpNego.readMessage()
			if err != nil {
				return err
			}
			conn.tcpNego.ServerFlags |= 2
		}
		conn.dataNego = buildTypeNego(conn.tcpNego, conn)
		conn.dbTimeZone, err = conn.dataNego.read()
		if err != nil {
			return err
		}
	}
	stop := false
	for !stop {
		msg, err := conn.session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 8:
			dictLen, err := conn.session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
			conn.SessionProperties = make(map[string]string, dictLen)
			for range dictLen {
				key, val, _, err := conn.session.ReadKeyVal()
				if err != nil {
					return err
				}
				conn.SessionProperties[string(key)] = string(val)
			}
		default:
			err = conn.ProcessTCCResponse(msg)
			if err != nil {
				return err
			}
			if msg == 4 || msg == 9 {
				stop = true
			}
		}
	}

	return nil
}

// loadNLSData get nls data for v2
func (conn *Connection) loadNLSData() error {
	_, err := conn.session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	_, err = conn.session.ReadByte()
	if err != nil {
		return err
	}
	length, err := conn.session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	_, err = conn.session.ReadByte()
	if err != nil {
		return err
	}
	for range length {
		nlsKey, nlsVal, nlsCode, err := conn.session.ReadKeyVal()
		if err != nil {
			return err
		}
		conn.NLSData.SaveNLSValue(string(nlsKey), string(nlsVal), nlsCode)
	}
	_, err = conn.session.ReadInt(4, true, true)
	return err
}

func (conn *Connection) getServerNetworkInformation(code uint8) error {
	session := conn.session
	if code == 0 {
		_, err := session.ReadByte()
		return err
	}
	switch code - 1 {
	case 1:
		// receive OCOSPID
		length, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadBytes(length)
		if err != nil {
			return err
		}
	case 3:
		// receive OCSESSRET session return values
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		length, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		// get nls data
		for range length {
			nlsKey, nlsVal, nlsCode, err := session.ReadKeyVal()
			if err != nil {
				return err
			}
			conn.NLSData.SaveNLSValue(string(nlsKey), string(nlsVal), nlsCode)
		}
		flag, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		sessionID, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		serialID, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		if flag&4 == 4 {
			conn.sessionID = sessionID
			conn.serialID = serialID
			// save session id and serial number to connection
		}
	case 4:
		err := conn.loadNLSData()
		if err != nil {
			return err
		}
	case 6:
		length, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		conn.transactionID, err = session.ReadCLR()
		if len(conn.transactionID) > length {
			conn.transactionID = conn.transactionID[:length]
		}
	case 7:
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadDLC()
		if err != nil {
			return err
		}
	case 8:
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
	}
	return nil
}

// SaveNLSValue a helper function that convert between nls key and code
func (nls *NLSData) SaveNLSValue(key, value string, code int) {
	key = strings.ToUpper(key)
	if len(key) > 0 {
		switch key {
		case "AUTH_NLS_LXCCURRENCY":
			code = 0
		case "AUTH_NLS_LXCISOCURR":
			code = 1
		case "AUTH_NLS_LXCNUMERICS":
			code = 2
		case "AUTH_NLS_LXCDATEFM":
			code = 7
		case "AUTH_NLS_LXCDATELANG":
			code = 8
		case "AUTH_NLS_LXCTERRITORY":
			code = 9
		case "SESSION_NLS_LXCCHARSET":
			code = 10
		case "AUTH_NLS_LXCSORT":
			code = 11
		case "AUTH_NLS_LXCCALENDAR":
			code = 12
		case "AUTH_NLS_LXLAN":
			code = 16
		case "AL8KW_NLSCOMP":
			code = 50
		case "AUTH_NLS_LXCUNIONCUR":
			code = 52
		case "AUTH_NLS_LXCTIMEFM":
			code = 57
		case "AUTH_NLS_LXCSTMPFM":
			code = 58
		case "AUTH_NLS_LXCTTZNFM":
			code = 59
		case "AUTH_NLS_LXCSTZNFM":
			code = 60
		case "SESSION_NLS_LXCNLSLENSEM":
			code = 61
		case "SESSION_NLS_LXCNCHAREXCP":
			code = 62
		case "SESSION_NLS_LXCNCHARIMP":
			code = 63
		}
	}
	switch code {
	case 0:
		nls.Currency = value
	case 1:
		nls.IsoCurrency = value
	case 2:
		nls.NumericChars = value
	case 7:
		nls.DateFormat = value
	case 8:
		nls.DateLang = value
	case 9:
		nls.Territory = value
	case 10:
		nls.Charset = value
	case 11:
		nls.Sort = value
	case 12:
		nls.Calender = value
	case 16:
		nls.Language = value
	case 50:
		nls.Comp = value
	case 52:
		nls.UnionCurrency = value
	case 57:
		nls.TimeFormat = value
	case 58:
		nls.Timestamp = value
	case 59:
		nls.TTimezoneFormat = value
	case 60:
		nls.NTimezoneFormat = value
	case 61:
		nls.LengthSemantics = value
	case 62:
		nls.NCharConvExcep = value
	case 63:
		nls.NCharConvImp = value
	}
}

func (conn *Connection) Exec(text string, args ...driver.Value) (driver.Result, error) {
	stmt := NewStmt(text, conn)
	defer func() {
		_ = stmt.Close()
	}()
	return stmt.Exec(args)
}

func (conn *Connection) CheckNamedValue(nv *driver.NamedValue) error {
	// database/sql converts a driver.Valuer, such as sql.Null[T], to its
	// value, except the few that the driver binds itself
	if _, ok := nv.Value.(driver.Valuer); ok && !ownValue(nv.Value) {
		return driver.ErrSkip
	}
	if namedBasic(nv.Value) {
		return driver.ErrSkip
	}

	return nil
}

func (conn *Connection) QueryRowContext(ctx context.Context, query string, args []driver.NamedValue) *DataSet {
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		dataSet := &DataSet{}
		dataSet.currentResultSet().lastErr = err
		return dataSet
	}
	dataSet, ok := rows.(*DataSet)
	if !ok {
		dataSet.currentResultSet().lastErr = fmt.Errorf("expected DataSet, got %T", rows)
		return dataSet
	}
	dataSet.Advance()
	return dataSet
}

func WrapRefCursor(ctx context.Context, q Querier, cursor *RefCursor) (*sql.Rows, error) {
	rows, err := cursor.Query()
	if err != nil {
		return nil, err
	}
	return q.QueryContext(ctx, wrapResultset, rows)
}

func (conn *Connection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == createLob {
		if len(args) != 1 {
			return nil, fmt.Errorf("CreateLob: expected 1 arguments, got %d", len(args))
		}
		if temp, ok := args[0].Value.(*types.LobStreamer); ok {
			*temp = &LobStream{
				conn: conn,
			}
		} else {
			return nil, fmt.Errorf("CreateLob: unsupported data type %T", args[0].Value)
		}
		return nil, nil
	}
	if query == connRef {
		if len(args) != 1 {
			return nil, fmt.Errorf("GetConnectionRef: expected 1 arguments, got %d", len(args))
		}
		tempVal := reflect.ValueOf(args[0].Value)
		tempVal.Elem().Set(reflect.ValueOf(conn))
		return nil, nil
	}
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	result, err := stmt.ExecContext(ctx, args)
	if err != nil {
		_ = stmt.Close()
		return nil, err
	}
	err = stmt.Close()
	return result, err
}

func (conn *Connection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query == wrapResultset {
		return args[0].Value.(driver.Rows), nil
	}
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		_ = stmt.Close()
	}
	return rows, err
}

func (conn *Connection) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if conn.State != Opened || !conn.session.Connected {
		// database/sql discards the connection and opens another. go-ora
		// opened this one again here, printed the result to standard
		// output, and returned ErrBadConn all the same
		conn.logger.Info("prepare on a broken connection")
		return nil, driver.ErrBadConn
	}
	conn.logger.Debug("prepare", "query", query)
	done := conn.session.StartContext(ctx)
	defer conn.session.EndContext(done)
	return NewStmt(query, conn), nil
}

func (conn *Connection) ProcessTCCResponse(msgCode uint8) error {
	session := conn.session
	var err error
	switch msgCode {
	case 4:
		conn.session.Summary, err = tns.NewSummary(session)
		if err != nil {
			return err
		}
		conn.logger.Debug("summary", "code", session.Summary.RetCode, "message", string(session.Summary.ErrorMessage))
		if conn.session.HasError() {
			return conn.session.ServerError()
		}
	case 8:
		size, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		for x := 0; x < size; x++ {
			_, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
		}
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		size, err = session.ReadInt(2, true, true)
		for x := 0; x < size; x++ {
			_, val, num, err := session.ReadKeyVal()
			if err != nil {
				return err
			}
			if num == 163 {
				session.TimeZone = val
			}
		}
		if session.TTCVersion >= 4 {
			// get queryID
			size, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if size > 0 {
				bty, err := session.ReadBytes(size)
				if err != nil {
					return err
				}
				if len(bty) >= 8 {
					queryID := binary.LittleEndian.Uint64(bty[size-8:])
					_, _ = os.Stderr.WriteString(fmt.Sprintln("query ID: ", queryID))
				}
			}
		}
		if session.TTCVersion >= 7 {
			length, err := session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			for range length {
				_, err = session.ReadInt(8, true, true)
				if err != nil {
					return err
				}
			}
		}
	case 9:
		if session.HasEOSCapability {
			temp, err := session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if session.Summary != nil {
				session.Summary.EndOfCallStatus = temp
			}
		}
		if session.HasFSAPCapability {
			if session.Summary == nil {
				session.Summary = new(tns.SummaryObject)
			}
			session.Summary.EndToEndECIDSequence, err = session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
		}
	case 15:
		warning, err := tns.NewWarningObject(session)
		if err != nil {
			return err
		}
		if warning != nil {
			_, _ = os.Stderr.WriteString(fmt.Sprintln(warning))
		}
	case 23:
		opCode, err := session.ReadByte()
		if err != nil {
			return err
		}
		err = conn.getServerNetworkInformation(opCode)
		if err != nil {
			return err
		}
	case 28:
		conn.logger.Debug("fast negotiation refused")
		err = conn.protocolNegotiation()
		if err != nil {
			return err
		}
		err = conn.dataTypeNegotiation()
		if err != nil {
			return err
		}
	default:
		return errors.New(fmt.Sprintf("TTC error: received code %d during response reading", msgCode))
	}
	return nil
	// cancel loop if = 4 or 9
}

func (conn *Connection) setBad() {
	conn.bad = true
}

// ResetSession decides responsible for resetting a connection. Part of a keepConnOnRollback condition to decide if to keep a transaction after rollback.
func (conn *Connection) ResetSession(_ context.Context) error {
	if conn.bad {
		return driver.ErrBadConn
	}
	return nil
}

// IsValid validates if a connection has to be discarded. Part of a keepConnOnRollback condition to decide if to keep a transaction after rollback.
func (conn *Connection) IsValid() bool {
	// Connection is valid if it's not marked as bad and is in opened state
	return !conn.bad && conn.State == Opened
}

func (conn *Connection) dataTypeNegotiation() error {
	var err error
	conn.logger.Debug("data type negotiation")
	conn.dataNego = buildTypeNego(conn.tcpNego, conn)
	err = conn.dataNego.write()
	if err != nil {
		return err
	}
	conn.dbTimeZone, err = conn.dataNego.read()
	return err
}

func (conn *Connection) protocolNegotiation() error {
	var err error
	conn.logger.Debug("protocol negotiation")
	conn.tcpNego = &TCPNego{conn: conn}
	err = conn.tcpNego.write()
	if err != nil {
		return err
	}
	err = conn.tcpNego.readMessage()
	if err != nil {
		return err
	}
	conn.logger.Debug("charsets", "server", conn.tcpNego.ServerCharset, "national", conn.tcpNego.ServerNCharset)
	conn.tcpNego.ServerFlags |= 2
	return nil
}

func (conn *Connection) appendTemporaryLoc(loc types.Locator) {
	for _, locator := range conn.temporaryLocs {
		if bytes.Equal(loc, locator) {
			return
		}
	}
	conn.temporaryLocs = append(conn.temporaryLocs, loc)
}

func (conn *Connection) Session() tns.SessionReadWriter {
	return conn.session
}

func (conn *Connection) NewLobStreamer() types.LobStreamer {
	return &LobStream{conn: conn}
}

func (conn *Connection) ParameterCoder(input any) (codec.OracleParameterCoder, error) {
	switch input := input.(type) {
	case reflect.Type:
		if coder, ok := conn.goTypeCoder[input]; ok {
			return coder.Copy(), nil
		}
		if input != nil && input.Kind() != reflect.Interface {
			if coder := builtinCoder(reflect.Zero(input).Interface()); coder != nil {
				coder.Init()
				return coder, nil
			}
		}

		return nil, fmt.Errorf("no parameter coder registered for go type %s", input.String())
	case string:
		if coder, ok := conn.nameTypeCoder[strings.ToUpper(input)]; ok {
			return coder.Copy(), nil
		}
		return nil, fmt.Errorf("no parameter coder registered for name %s", input)
	case int, int16, int32, int64:
		index := reflect.ValueOf(input).Int()
		if coder, ok := conn.oracleTypeCoder[uint16(index)]; ok {
			return coder.Copy(), nil
		}

		return nil, fmt.Errorf("no parameter coder registered for oracle type %d", index)
	case uint, uint16, uint32, uint64:
		index := reflect.ValueOf(input).Uint()
		if coder, ok := conn.oracleTypeCoder[uint16(index)]; ok {
			return coder.Copy(), nil
		}

		return nil, fmt.Errorf("no parameter coder registered for oracle type %d", index)
	default:
		return nil, fmt.Errorf("accepted values for ParameterCoder: reflec.Type, int/uint or string")
	}
}

func (conn *Connection) SendTimeAsUTC() bool {
	return conn.dataNego.serverTZVersion > 0 && conn.dataNego.clientTZVersion == conn.dataNego.serverTZVersion
}

func (conn *Connection) DBTimeZone() *time.Location {
	return conn.dbTimeZone
}

func (conn *Connection) DBServerTimeZone() *time.Location {
	return conn.dbServerTimeZone
}

func (conn *Connection) TTCVersion() uint8 {
	return conn.session.TTCVersion
}
