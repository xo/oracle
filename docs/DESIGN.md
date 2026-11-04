# Driver design

This document is the design of the rewritten driver. Ken accepted it on
2026-10-31, and D8 to D16 in [PLAN.md](PLAN.md) record it. The design
follows the drivers of [`xo/dbimp`](https://github.com/xo/dbimp) and
[`xo/cassandra`](https://github.com/xo/cassandra), so that an `xo` driver looks
the same whatever database it reaches. Where this document cites a decision of
another repository, it names the repository, as in "dbimp D8".

The sources, read on 2026-10-31:

- `dbimp/AGENTS.md`, `dbimp/docs/DESIGN.md`, and dbimp D6, D8, D25, D27, D35 and
  D109.
- `cassandra/docs/DESIGN.md`, which is the nearest example of a repository that
  holds one driver.
- The code of this repository at `cb68b12`, which is the `v3` code of
  `sijms/go-ora` at upstream commit `62f8a42`.
- The Go 1.27.2 source of `database/sql/driver`.

## Goals

1. Keep the contract of dbimp D8 with `database/sql`. Implement the context form
   of every interface, stop work when the context ends, return a NULL as nil,
   keep the column order, and return every error wrapped with `%w`.
2. Hold no state at the package level. Everything that belongs to one database
   belongs to one `Connector` (dbimp hard rule 2).
3. Use the idioms of Go 1.27, and require Go 1.27.1 or newer, as dbimp D2 and
   dbimp D25 do. Depend on the standard library and on
   `github.com/cockroachdb/apd/v3` alone (D16).
4. Keep the wire protocol code. TNS (the Oracle network layer), TTC (the Oracle
   two task layer), the authentication and the encryption are the hard part, and
   they work. The rewrite changes the shape around them, and fixes them only
   where a fault is known.
5. Run every unit test with no server. Gate the integration tests on
   `ORACLE_DSN`, which a test reads from `dbrun` (D6).

## What changes, and why

The table lists the faults of the current shape. Each row names the part of the
design that removes it.

| Today | Fault | Design |
| --- | --- | --- |
| One package variable, `oracleDriver`, holds the session parameters, the registered types, the coders, the TLS configuration and the string converters of every database, and one store holds the fast login cookies of the process | Two databases in one process share them. A cookie of the first names the charset of the second when both have one service name, which is sijms/go-ora#692 | Connector |
| `AddSessionParam(db, ...)`, `RegisterType(db, ...)` and `AddParameterCoder(db, ...)` reach the driver through a `*sql.DB` | The call changes the driver of every `*sql.DB`, not only the one that the caller names | Connector |
| `StartContext` starts a goroutine for each call, and the goroutine breaks the connection when the context ends | A goroutine for each round trip, a race on `breakConn`, and a rollback that runs under a context that already ended, which is sijms/go-ora#767 | Context |
| `Transaction` stores its context | Against dbimp hard rule 4 | Transactions |
| The library calls `context.Background()` or `context.TODO()` in 20 places | A caller cannot stop that work | Context |
| The DSN takes any scheme, keys with spaces, and a host that must have a port | Against dbimp D27 and dbimp D35 | DSN |
| The `trace` package writes files | A package holds no configuration of its own, by dbimp hard rule 2 | Logging |
| The errors are made with `errors.New` and `fmt.Errorf` with no `%w` | A caller cannot test an error with `errors.Is` | Errors |
| `converters` is 79,775 lines, almost all one generated table | A build of a program that imports the driver is 23.6 MB | Charsets |
| The package is `go_ora` | Not an idiomatic name | Packages |

## Packages

The root package is `oracle`, at `github.com/xo/oracle`. Its name is the name
that it registers with `database/sql`, and the name of the scheme of its DSN, as
dbimp hard rule 1 asks. A consumer imports it as `_ "github.com/xo/oracle"` or
calls it as `oracle.NewConnector`.

Everything that a caller does not use goes under `internal/`, so that it can
change without a new major version. `go-ora` made every package public.

Step 3 moved each package of `go-ora` that a caller does not use under
`internal/`, and changed nothing else. The 30 recorded exchanges passed before
and after the move, so the driver sent the same packets. Later steps renamed,
split and deleted packages, and the table holds the layout now. The third
column says where each one was.

| Path | Holds | In `go-ora` |
| --- | --- | --- |
| `/` | The driver, the connector, the DSN, the errors, and the value types that a caller passes or scans, as aliases of `internal/types` and `internal/ttc` | the root package |
| `aq/` | Advanced Queuing | `aq` |
| `integration/` | The integration tests of `go-ora`, which need a server | `TestIssues` |
| `internal/ttc/` | The connection, statements, rows, transactions, LOBs, cursors, user defined types, the negotiation of data types, O5LOGON, the fast login cookies, and the registry of types | the root package, which step 4b moved |
| `internal/config/` | The configuration of the protocol code, and wallets | `configurations` |
| `internal/tns/` | Packets, the session, TLS, the redirect, out of band breaks, and the logging of packets | `network` |
| `internal/auth/` | The advanced negotiation of Oracle Net: encryption, checksums, Kerberos and NTS | `advanced_nego` |
| `internal/auth/security/` | DES and RC4, and the padding that O5LOGON uses | `network/security` |
| `internal/auth/ntlmssp/` | NTLM, with the MD4 hash that it needs | `advanced_nego/ntlmssp`, `network/security/md4` |
| `internal/codec/` | The encoders and decoders of each Oracle type | `parameter_coder` |
| `internal/codec/oson/` | OSON, the binary JSON of Oracle | `types/oson` |
| `internal/types/` | The Oracle values, such as NUMBER, dates, intervals, LOBs and JSON | `types` |
| `internal/charset/` | The 236 charsets, as an embedded blob | `converters` |
| `internal/tnstest/` | The recorder and the player of TNS exchanges | none |
| `internal/repotest/` | The tests of the repository setup | none |

`internal/utils`, `internal/lazyinit` and `internal/trace` are gone. O5LOGON
is in `internal/ttc/auth_object.go`, and the wallets in `internal/config`.
Both can move to `internal/auth/` later.

## The root package

The layout is the one of `cassandra` and of every dbimp driver. Each file holds
one job, so that a person or an agent finds a behavior by its file name.

| File | Holds |
| --- | --- |
| `driver.go` | `Driver`, `Open`, `OpenConnector`, and the `init` that calls `sql.Register("oracle", ...)` |
| `connector.go` | `Connector`, `NewConnector`, `Connect` and `Close` |
| `conn.go` | `conn` and every method that `database/sql` calls on it |
| `stmt.go` | `stmt` |
| `rows.go` | `rows`, with `NextRow`, `ScanColumn` and the column type methods |
| `tx.go` | `tx` |
| `dsn.go` | `ParseDSN` and `Config.DSN` |
| `options.go` | `Option`, `WithOptions`, and the option of each setting |
| `errors.go` | `Error`, the sentinel errors, `*ServerError`, `*BatchError` and `RowError` |
| `values.go` | The Oracle types that Go lacks, such as `IntervalYM`, `RowID`, `Vector` and `JSON`, the LOB types, and `Object` |
| `output.go` | The reader of `DBMS_OUTPUT` (D19) |

`values.go` names the types of `internal/ttc` and `internal/types` that a
caller uses, such as `Out`, `RefCursor` and `Clob`, as aliases.
`conn`, `stmt`, `rows` and `tx` implement the interfaces of `database/sql`,
and each holds the `ttc.Conn`, `ttc.Stmt`, `ttc.Rows` or `ttc.Tx` that runs
the protocol, and passes each call to it. Those were `Connection`, `Stmt`,
`DataSet` and `Transaction` of go-ora, and live in `conn.go`, `stmt.go`,
`rows.go` and `tx.go` of `internal/ttc`.

## Connector

```go
// Connector opens connections to one Oracle database. It holds everything
// that belongs to that database: the configuration, the registered types,
// the session parameters and the TLS configuration.
type Connector struct {
	cfg Config // a copy, so that the caller can change theirs

	mu    sync.Mutex
	types map[string]objectType
}

// NewConnector returns a Connector for cfg. It does not connect.
func NewConnector(cfg Config, opts ...ConnectorOption) (*Connector, error)
```

A caller that needs more than the DSN can say builds a `Config`, or parses the
DSN with `ParseDSN` and changes the result, and calls
`sql.OpenDB(oracle.NewConnector(cfg))`. These replace the functions that take a
`*sql.DB`:

| Today | Design |
| --- | --- |
| `AddSessionParam(db, k, v)` | `WithSessionParam(k, v)`, a `ConnectorOption`, run after each logon |
| `RegisterType(db, name, array, v)` | Kept, with its `*sql.DB`. It reaches the registry of that database alone through `db.Driver()`. See the note below |
| `AddParameterCoder(db, t, id, c)` | Removed until a caller needs it. A user defined type covers the case that issues name |
| `SetStringConverter(db, ...)` | Removed. The server names the charset at logon, and the connection keeps it |
| `WithTLSConfig(cfg)` | `Config.TLS`, a `*tls.Config` that the connector clones for each dial, which fixes sijms/go-ora#757 |
| `RegisterConnConfig(cfg)` | Removed |

Step 4 built this, and step 4b finished it. `Config.connectionConfig` writes
the configuration of the protocol code directly. Before the parser of
`go-ora` went, a test compared the two configurations, field by field, for 18
DSNs, and they were equal. The one difference was a fault of the old path: it
wrote the key `CONNSTR` and the parser read `connStr`, so a descriptor failed.

`RegisterType` keeps its `*sql.DB`, and `WithType` does not exist. A
`ConnectorOption` acts when the connector connects, and `RegisterType` reads
the attributes of the type from the database, so the type has to exist
before the first connection. A program, and many tests in `integration`,
create the type through the same `*sql.DB` and register it after. Under
`WithType` the first connection fails, and the type can never be created. The
registry belongs to one connector, so `RegisterType` changes no other
database, which is what D9 asks.

`Driver` is the value that `init` registers, and it holds nothing. Each
`Connector` returns a `Driver` that points back at it, so a function that
takes a `*sql.DB` reaches the state of that database through `db.Driver()`.

`sql.Open` and `OpenConnector` never connect. The first network traffic is the
first `Connect`, as in `cassandra`.

The charset and the national charset belong to each connection. The server
sends them at logon, and `conn` keeps them. A fast login cookie names them
too, so that the next logon is shorter, and each `Connector` keeps its own
cookies. A cookie of one database never reaches another. That fixes
sijms/go-ora#692.

The authenticators of Kerberos and NTS are `Config.Kerberos` and `Config.NTS`.
`go-ora` held each in a variable of the process, which `SetNTSAuth` and
`SetKerberosAuth` set. With them gone, step 9 left no state in the process.

## Context

The context comes first, is named `ctx`, and is never stored in a struct. The
library never calls `context.Background()` or `context.TODO()`.

Each call takes its context. `Session.StartContext` watches it with
`context.AfterFunc`, and returns the function that ends the watch, which
`EndContext` calls. When the context ends first, the watch sends the break of
Oracle Net, by out of band data where the server supports it and by a marker
packet where it does not. The server answers with ORA-01013, the call reads to
the reset, and the call returns the error of the context. A call that ends
with no error returns its result, even when its context ends at the same
moment. There is no goroutine for each call: `context.AfterFunc` runs a
function only when the context ends. `go-ora` started two for each statement,
one that watched the context and one that ran the call.

The deadline of the context is not the deadline of the `net.Conn`. A read that
fails at a deadline leaves the stream in the middle of a packet, and the
connection is lost. A break leaves it usable, and the tests show that on 11g,
21c and 26ai. After a break, the next read has a deadline of 10 seconds, so a
server that does not answer the break fails the read, and the connection is
broken. `EndContext` waits for a break that started, so that the next call
never meets it halfway. The flag of the break is read under the lock of the
session, which was a data race in `go-ora`.

A break can reach the server after the call that it broke has ended. The
server then answers it at the start of the next call, with a marker and
ORA-01013. The session marks a break pending until a marker answers it, and
`ResetBuffer`, which starts each call, first reads and drops that answer.
go-ora read it as the reply of the next call, which failed with ORA-01013, or
which waited for good. `TestLobReadDeadline` holds it. The session marks the
break pending before it sends the break, because the reader can read the
answer before the write returns. It marked the break after the write, and on
a fast machine the next call then waited 10 seconds for an answer that had
come already, and the connection went bad. `TestBreakAnsweredAtOnce` holds
it.

A LOB that the driver reads while it fetches rows, or uploads while it binds a
statement, runs inside a call that is watched already, or under `Next`, where
`database/sql` gives no context. It reads with `Lob.Load`, which watches
nothing, so one call never has two watches. `CreateBlob`, `CreateClob` and
`CreateBFile` take the context first, and the LOB types keep none.

A `BFile` of a row keeps its locator in its streamer, which `IsInit` reads, and
`Exists` runs on a file that is not open. go-ora read neither, so a `BFile`
of a row did not open and a missing file did not answer. `CreateBFile` writes
the names in the character set of the database, where go-ora wrote them in
AL16UTF16. `TestBFile` holds the cases of python-oracledb.

`Next` loads the LOB values of a row as it returns the row, and not of every
row of a fetch, so the driver holds the values of one row (sijms/go-ora#664).
Under `database/sql`, which scans through `ScanColumn`, a CLOB or a BLOB is
loaded there instead, and only for a destination that is not an
`oracle.Clob` or an `oracle.Blob`. That destination gets the locator, which
`NewReader` reads in pieces (D22). A NULL LOB scans as nil, and a LOB whose
locator holds its value scans as the value with no round trip. Both went to
the destination as the LOB itself after D22, until `TestBatchBlob` found it.
A locator of a row shares a guard with its `ttc.Rows`: a flag that `Close`
sets, after which each operation of its `LobStream` fails with
`ErrRowsClosed`, and a lock that each operation holds while it uses the
connection. `Close` takes the lock, so a close from another goroutine, which
`database/sql` runs when the context of a query ends, waits for a LOB call
in flight. It wrote the close of the cursor in the middle of the call
before.

A fetch that brings LOB or LONG values inside it, as `lob_fetch=inline`
does, asks for one row first. Each later fetch asks for the rows that fill
4 MiB with values the size of the last ones. A prefetch that the caller sets
is used as it is.

`Driver.Open` and `conn.Begin`, the forms with no context, fail with an
error that wraps `ErrNotSupported`. `database/sql` calls neither, as in dbimp.

When the break fails, or the read after it fails, the connection is broken. The
next call returns `driver.ErrBadConn`, and `ResetSession` and `IsValid` say so,
so that `database/sql` drops it.

Logon, the TNS connect, and the TLS handshake take the context of `Connect`.

## Transactions

`database/sql` calls `Commit` and `Rollback` with no context. So `tx` keeps no
context, and the two calls run with the read and write deadline of the
connection, not with the context of `BeginTx`. A context that ends while a
transaction is open makes `database/sql` call `Rollback`. The rollback then
runs to the end, because nothing cancels it, and that fixes sijms/go-ora#767.
`TestCancelledTransactionDoesNotCommit` fails on the code before step 5 and
passes after it.
The isolation levels that Oracle has, read committed and serializable, are
accepted. A read only transaction is `SET TRANSACTION READ ONLY`. Any other
level fails with an error that wraps `ErrNotSupported`.

The server commits an open transaction when a session logs off. So `Close`
rolls back first when the end of the last call said that a transaction is
open, or when a `BeginTx` has not ended, as python-oracledb does. go-ora
committed it. `TestRollbackAtClose` holds it.

## Advanced Queuing

The `aq` package is that of go-ora. `CreateQueue` takes an `Execer`, which
needs `ExecContext` alone, so that a `*sql.Conn` is one. go-ora asked for
`Exec` and `Prepare`, which a `*sql.Conn` lacks, and a queue of a `*sql.DB`
held a connection that the pool can give to another caller. A dequeue
that finds no message returns none and no error, as python-oracledb does,
where go-ora returned ORA-25228 and an empty message. The server breaks a
dequeue of an array that finds no message and sends ORA-25228 after the
reset, which the queue reads, where go-ora returned the reset as a timeout.
A message writes its expiration as a signed number and its shard as four
bytes, and an array asks for the ID of each message, as python-oracledb
writes them, where go-ora wrote -1, the shard and the flags so that the
server broke a dequeue of an array. A dequeue takes the delivery mode of its
options, which go-ora took from the message that it had not read yet, and
an array returns the messages that came back, each with its own ID.
`TestAQRaw`, `TestAQBulk`, `TestAQObject` and `TestAQJSON` hold the cases of
python-oracledb, which had no test here before.

## DSN

The DSN is a standard URL whose scheme is `oracle`, parsed with `net/url`, by
dbimp D27 and dbimp D35. Another scheme is an error. This is a new package, so
the DSN keeps no form and no key name of `go-ora` (D11).

```
oracle://user:password@host:port/service/dbname?key=value
```

- The user and the password are the user information of the URL.
- The host is required. The port is 1521 when the URL names none.
- The first segment of the path is the service name. The second segment, when
  there is one, is the database name, which goes in the connect data as
  `INSTANCE_NAME`. A path with more than two segments is an error.
- Each segment is read from `URL.EscapedPath` and unescaped on its own. A
  service name that holds a `/` is written as `%2F` by the code that builds
  the URL, which is `dburl` or the caller. There is no key that names the
  service in place of the path.
- A key that the driver does not know is an error. A key that appears twice is
  an error, except `server`, which names one more address each time.
- Every key is lower case, with no space. Where Oracle Net names the setting,
  the key is that name in lower case, such as `connect_timeout` and
  `retry_count`.

The TLS keys are the ones of `pgx` and `lib/pq`, which `usql` users know.

| Key | Values | Default |
| --- | --- | --- |
| `sslmode` | `disable`, `require`, `verify-ca`, `verify-full` | `disable` |
| `sslrootcert` | A file of PEM certificates that sign the certificate of the server | the roots of the system |
| `sslcert`, `sslkey` | The certificate and the key of the client, for mutual TLS | none |
| `sslservername` | The name to check the certificate against, in place of the host | the host of the URL |
| `wallet` | A folder that holds `cwallet.sso` or `ewallet.p12` | none |
| `wallet_password` | The password of `ewallet.p12` | none |

`require` encrypts and checks nothing. `verify-ca` checks the chain.
`verify-full` checks the chain and the name. After a listener redirect, the
name stays the one of the URL, which fixes sijms/go-ora#765. `allow` and
`prefer` of `pgx` are not accepted, because an Oracle listener speaks TCP or
TCPS on a port and never both, so there is no plain connection to fall back to.

The other keys:

| Key | Holds | `go-ora` read it as |
| --- | --- | --- |
| `sid` | The SID, in place of a service name | `SID` |
| `descriptor` | A TNS descriptor that replaces the host, the port and the path | `CONNSTR` |
| `server` | One more address, `host:port` | `SERVER` |
| `timeout` | The read and write deadline of a round trip, a Go duration such as `30s` | `TIMEOUT` |
| `connect_timeout` | The limit on the TCP connect, TLS and logon, a Go duration | `CONNECT TIMEOUT` |
| `prefetch` | The rows that one fetch asks for | `PREFETCH_ROWS` |
| `lob_fetch` | `inline` or `stream` | `LOB FETCH` |
| `lob_read` | `auto` or `explicit` | `LOB READ` |
| `auth` | `password`, `os`, `kerberos` or `token` | `AUTH TYPE` |
| `privilege` | `sysdba`, `sysoper` and the other administrative privileges | `DBA PRIVILEGE` |
| `token`, `token_key` | The files of a token and its private key | `TOKEN FILE`, `TOKEN PRIVATE KEY FILE` |
| `encryption`, `checksum` | `accepted`, `rejected`, `requested` or `required` | `ENCRYPTION`, `DATA INTEGRITY` |
| `language`, `territory`, `charset` | The NLS settings of the client | `LANGUAGE`, `TERRITORY`, `CHARSET` |
| `program`, `proxy_user` | What the session reports | `PROGRAM`, `PROXY CLIENT NAME` |
| `unix_socket` | A Unix socket in place of TCP | `UNIX SOCKET` |
| `alias`, `tns_admin` | An alias of `tnsnames.ora` and the folder of the file, which is `$TNS_ADMIN` when the key is absent (D20) | none |

A user named `SYS` gets no privilege by its name. The DSN names it with
`privilege=sysdba`.

`tnsnames.ora` follows the rules that python-oracledb tests. An entry can be
a descriptor or an Easy Connect string, `[tcp://]host[:port][/service]`,
which becomes a descriptor. An entry that comes later, in the file or in a
file that `IFILE` includes, replaces an earlier one. Text before an entry
that is no entry is skipped, and an `IFILE` that includes a file that
includes it is an error. A descriptor is read by walking its parentheses, and
each `ADDRESS` keeps its `HOST`, `PORT` and `PROTOCOL` and ignores its other
keys, where go-ora matched an address with a regular expression and dropped
one with `HTTPS_PROXY` or `SEND_BUF_SIZE`. `TestLookupAliasCases` and
`TestDescriptors` in `internal/config` hold the cases of python-oracledb.

## Options

A statement takes options as dbimp D109 sets them out. An option comes from the
DSN, then from the context through `WithOptions`, then from an argument of the
type `Option`, and a later one wins. `CheckNamedValue` takes an `Option` out of
the arguments. The common options are `WithTimeout`, the time that the server
gives one statement, and `WithReadonly`. The options of this driver are
`WithPrefetch`, `WithLobFetch`, `WithBatchErrors` and `WithRowCounts` (D34).
dbimp holds the machinery in its root
package. This module cannot import it (hard rule 2), so it holds a copy of the
part it needs, in `internal/ttc/options.go`, and `options.go` names it.

Step 9 built it. `WithTimeout` ends the call of a statement through the break
of Oracle Net, which leaves the connection usable, and bounds the call and
not the fetches of the rows after it. `WithPrefetch` and `WithLobFetch` are
the keys `prefetch` and `lob_fetch` for one statement. A statement keeps its
mode of LOBs and sets it on the connection before each of its round trips,
so a statement after it gets the mode of the DSN. Oracle has no setting for
one statement for `WithReadonly`, `WithParameter` or `WithDatabase`, so each
fails with `ErrNotSupported`, and a value that the DSN refuses fails
with `ErrInvalidValue`, as dbimp D109 asks. `TestOptions` holds them.

## Values

A NULL is nil. A value has the Go type that fits its Oracle type, and `rows`
implements `driver.RowsColumnScanner`, so that `Scan` fills the destination of
the caller with no copy through `any`.

`ColumnTypeDatabaseTypeName` gives the name of the data dictionary, such as
`VARCHAR2`, `NCHAR`, `FLOAT` or `TIMESTAMP WITH TIME ZONE`, and the name of
its type for an object. `go-ora` gave none. `TestColumnType` holds it.

`ColumnTypeLength` gives the declared length of a string in characters, and
of a `RAW` in bytes. So a caller can tell a `RAW(16)`, which holds a GUID,
from another `RAW`. A `LONG`, a `LONG RAW`, a LOB and a `BFILE` give
`math.MaxInt64`, as `database/sql` asks for a type with no limit. A type of
fixed length, such as `NUMBER` or `DATE`, gives none. The length comes from
the name of the type, because the define of an inline LOB changes the data
type of the column. `go-ora` gave a length for a string only.
`TestColumnTypeLength` holds it.

| Oracle | Go, for a scan into `*any` | Also scans into |
| --- | --- | --- |
| `VARCHAR2`, `CHAR`, `NVARCHAR2`, `NCHAR`, `LONG`, `CLOB`, `NCLOB` | `string` | `[]byte`, `Clob` |
| `NUMBER` with scale 0 and precision 18 or less | `int64` | the numeric types, `string` |
| `NUMBER` with any other precision or scale | `*apd.Decimal`, which holds every digit | `apd.Decimal`, `string`, `float64`, `int64` |
| `BINARY_FLOAT`, `BINARY_DOUBLE` | `float64` | `float32` |
| `DATE`, `TIMESTAMP` | `time.Time` in UTC, because the type holds no zone | |
| `TIMESTAMP WITH TIME ZONE` | `time.Time` in its zone | |
| `TIMESTAMP WITH LOCAL TIME ZONE` | `time.Time` at the same instant, in the time zone of the database | |
| `INTERVAL YEAR TO MONTH` | `IntervalYM` | `string` |
| `INTERVAL DAY TO SECOND` | `time.Duration` | `string` |
| `RAW`, `LONG RAW`, `BLOB` | `[]byte` | `Blob`, and `uuid.UUID` for a `RAW(16)` |
| `BOOLEAN` | `bool` | `sql.Null[bool]` |
| `JSON` | `JSON`, which holds the document | `string`, any type that `encoding/json/v2` decodes |
| `VECTOR` | the elements: `[]float32`, `[]float64`, `[]int8`, `[]byte` of `BINARY`, or `SparseVector` | `Vector` |
| `ROWID`, `UROWID` | `string` | `RowID` |
| `REF CURSOR` | `*sql.Rows` through `driver.Rows` | |
| An object or a collection | the type that `RegisterType` registered | |

Step 6 built the rows of `NUMBER`, `BINARY_FLOAT`, the intervals and
`RowsColumnScanner`, step 7 built `JSON`, and `values_test.go` checks each
against a server.

A `time.Time` with a fraction of a second binds as a `TIMESTAMP`, and one with
none as a `DATE`. `go-ora` bound every one as a `DATE`, which dropped the
fraction. A `DATE` bind keeps the index of a `DATE` column that it meets,
which a `TIMESTAMP` bind converts, and a `TIMESTAMP` that goes into a
`DATE` column loses its fraction, as Oracle truncates it. The zone of a
`time.Time` is dropped, and the server reads its wall clock in the zone of
the session. `TimestampTZ` binds with its zone.

`go-ora` gave a `DATE` and a `TIMESTAMP` the zone of the operating system of
the server, which a query of `SYSTIMESTAMP` found at each logon. That zone is a
fixed offset, so a date in winter read with the offset of summer
(sijms/go-ora#672). They are in UTC now, and the logon runs no query: the
query of the NLS settings, which nothing read, is gone too (sijms/go-ora#611).
A `TIMESTAMP WITH LOCAL TIME ZONE` is the wall clock of the zone of the
database, which the negotiation of data types sends as an offset or as a
region, in the form of a `TIMESTAMP WITH TIME ZONE`. `go-ora` read a region as
an offset of more than 100 hours (sijms/go-ora#655). A region that the time
zone database of Go lacks makes a read of such a value fail with an error.
`TestLocalTimeZone` holds it, on 11g with `+00:00`, `-05:00` and
`Europe/London`.

A `bool` binds as a `BOOLEAN` on 23ai and later, and in PL/SQL from 12.1,
as python-oracledb binds one. In SQL before 23ai, which has no `BOOLEAN`, it
binds as the `NUMBER` 1 or 0. `go-ora` bound no `bool`. A `BOOLEAN` bind has
the size 4, and a NULL one is `0xFD 1`, as python-oracledb writes them. With
the size 1, 18c never answered a block with a `BOOLEAN` IN and a `BOOLEAN`
OUT. A NULL `BOOLEAN` OUT of PL/SQL before 23ai is a byte of 0 with the
length -1, which reads as NULL.

A `time.Time` OUT binds as a `TIMESTAMP`, so that it takes the fraction of a
`TIMESTAMP`. A NULL `RAW`, `LONG RAW` or `BLOB` reads as NULL, where go-ora
returned a nil `[]byte` in an `any`, which `sql.Null[[]byte]` took as valid.
An OUT string sends its size in bytes and no count of characters, as
python-oracledb does. With the count that go-ora sent, an OUT of PL/SQL held
32,767 bytes divided by the bytes of a character, 8,191 on AL32UTF8. 11g
holds one to that size in any case. `TestVarTypes` holds a value and a NULL
of each type, OUT and IN OUT, for the types of `test_2000_var.py`.

A `uuid.UUID` of the standard library binds as a `RAW(16)`, which is how
Oracle stores a UUID, and so do a `*uuid.UUID` and a `sql.Null[uuid.UUID]`.
The driver took one, a `[16]byte`, for a PL/SQL array, which the server
refused with ORA-01484. The `Value` method of a `sql.Null[uuid.UUID]` makes
its text, so `types.OwnNull` reads it. An OUT of one takes 16 bytes. A
`RAW(16)` scans into `*any` as a `[]byte` (D37). `TestUUIDs` holds it.

A string longer than the longest `VARCHAR2` binds as a `LONG`. 11g and 18c
convert one to the UTF-16 of a `CLOB` one packet at a time, and stored U+FFFD
for a character that the end of a packet split. 19.33, 21c, 23ai and 26ai
store it whole, and 19.3, which CI leaves out (D17), is not tested. So before
19c a string that long with a character outside ASCII binds as a temporary
`CLOB`, which a `CLOB` column stores whole. A `LONG` column of 18c stores it
whole too, and one of 11g refuses it with ORA-22835, where it garbled it. A
bind in the national charset fixed the `CLOB` too, and garbled a `LONG` on
11g. `TestStrings` and `TestLongColumn` hold it.

A `[]byte` longer than the longest `RAW` binds as a `LONG RAW`. SQL takes a
`LONG` or a `LONG RAW` bind only as a value to insert or set, and refuses one
in an expression or in the `SELECT` of a `MERGE` with ORA-01461. A caller
binds `CreateClob` or `CreateBlob` there (D24). PL/SQL takes a `VARCHAR2` or a
`RAW` of up to 32,767 bytes on every release, and no `LONG` past that, so in a
PL/SQL block a string or a `[]byte` binds as a `VARCHAR2` or a `RAW` up to
32,767 bytes, and as a temporary `CLOB` or `BLOB` past that, as
python-oracledb binds it. An IN OUT value that long binds so too, and comes
back as a string or a `[]byte`. An OUT with a larger size stays a `VARCHAR2`
or a `RAW`, as a `LONG` column selects into one and not into a `CLOB`, so a
longer CLOB comes back through an `oracle.Clob`. The driver used the limit of
SQL in PL/SQL, so a value over 4,000 bytes on 11g and 32,767 bytes on 18c and
later failed with ORA-01483. `TestPLSQLLongValues` holds it.

A value of the Go type that `RegisterType` names binds as its user defined
type. A collection of a basic type, such as a `TABLE OF VARCHAR2`, binds
through a named Go element type, such as `type Name string`, or through
`Object`, which names the type at the bind (D25). `Object` was
`go_ora.Object`. A named value of a basic kind, or of `time.Time` or
`[]byte`, inside an object or a collection binds as that kind, where
`database/sql` converts one only at the top of a statement.

`RegisterType` takes a record and a collection of a PL/SQL package as
`PKG.TYPE`, from `ALL_PLSQL_TYPES` and its attributes from
`ALL_PLSQL_TYPE_ATTRS` (D33). A `BOOLEAN`, a `PLS_INTEGER` and a
`BINARY_INTEGER` attribute are a length of 4 and four bytes, big endian, in
the image. An index-by table has the flag 0x10 in its image, and the key of
each element before it. A slice binds with the keys from 1, and a
`map[int32]T` with its own keys. On a read, a map keeps the keys, and a slice
takes the elements in their order. The bind of every object sends the version
of its type, 1, as python-oracledb sends it. go-ora sent 0, which a SQL type
takes and a type of a package fails with ORA-00600 [kocpnsh014].
`TestPLSQLRecords` holds the cases of python-oracledb. `RegisterType` takes
the record of the row of a table as `TABLE%ROWTYPE`, alone or as the element
of a collection of a package. It is in no view, so its type OID comes from
`DBMS_PICKLER.GET_TYPE_SHAPE` and its attributes from `ALL_TAB_COLS`, as
python-oracledb reads them. A precision in the name of a type, as
`TIMESTAMP(6)`, is dropped. `TestRowTypeRecords` holds it.

A slice binds to PL/SQL as a host array, as an index-by table that PL/SQL
reads with the keys from 1. A `[]bool` is refused with `ErrNotSupported`,
because the server fails on a host array of `BOOLEAN` with an internal error,
ORA-06544 on 26ai and ORA-00600 on 18c. A collection of a package binds one
(D33). A host array with holes fails with ORA-06513, and a collection of the
package reads one. `TestPLSQLArrays` and `TestPLSQLBooleanArrays` hold the
cases of python-oracledb.

The cursors that a PL/SQL block returns with `DBMS_SQL.RETURN_RESULT` read as
the result sets of its rows, through `Rows.NextResultSet`.
`TestImplicitResults` holds the cases of python-oracledb.

The values of a call are decoded while its reply is read, when no LOB can
load. After the call, the driver loads each CLOB and BLOB of an OUT value
and of the elements of a collection. An object with a LOB attribute is
decoded as a pending object, which holds its struct and the fields still to
fill, and these are filled after the reply, for an OUT value after the call
and for a row in `Next`, at any depth of objects. A LOB attribute fills an
`oracle.Clob`, an `oracle.Blob`, a `string` or a `[]byte` field. In the image
of an object, a LOB attribute is its locator with the length of the image,
and a NULL is `0xFF`, as python-oracledb writes them. go-ora wrote a length of
4 bytes first, which the server refused with ORA-03101. An object with a LOB
attribute panicked on a read, and an OUT collection of CLOBs gave the
locators as text. `TestBasicCollections` and `TestObjectLobAttributes` in
`integration` hold it.

The type of a column decides the Go type, and the value never does, so every
row of a column has the same type. `COUNT(*)` is a `NUMBER` with no precision,
so it is an `*apd.Decimal`, and it scans into an `*int64`.

`ScanColumn` stores a value as dbimp `Assign` does. A `*any` receives it as it
is, and an `*apd.Decimal` receives a decimal. Every other pair goes to
`sql.ConvertAssign`, and a decimal goes to it as text.

`database/sql` passes a decimal between two packages as its parts, a form, a
sign, a coefficient and an exponent, through the methods `Decompose` and
`Compose`. It does not export the interfaces that hold them. A value with
`Decompose` binds as an `*apd.Decimal`, as an argument, as an attribute of an
object and as the destination of an OUT. A destination with `Compose`
receives a `NUMBER`, an integer, a `BINARY_DOUBLE` or the text of a number
through it. The driver takes `Compose` before `Scan`, as `sql.ConvertAssign`
does. go-ora knew neither method, so a decimal with the two methods and no
`Scan` or `Value` failed both ways. `TestDecimalArg`,
`TestAssignDecimal` and `TestNumberDecimalParts` hold it.

An interval is an `IntervalYM` or a `time.Duration`, and both bind as an
argument. `go-ora` read an interval as a `time.Time`, so `+2021-10` came back
as 2021-09-30, and it wrote the nanoseconds of a day to second interval as
microseconds and read them back multiplied by 1000. The same fault sent the
time zone of the client at logon with 30 days added.

A decimal is an `*apd.Decimal` from `github.com/cockroachdb/apd/v3`, a new one
for each value, as in dbimp D33 (D16). `NUMBER` holds 38 digits, which neither
`float64` nor `int64` holds.

A decimal binds as a `NUMBER`. `database/sql` turned one into its text, which
the server read with the `NLS_NUMERIC_CHARACTERS` of the session, so 24.5
failed where the decimal point is a comma. Each number that binds as a
`NUMBER` goes through its decimal text, as python-oracledb writes it: a byte of
exponent and up to 20 digits of base 100, from 1e-130 to below 1e126. A value
past that range, an infinity and a NaN are errors. go-ora read the `E` of the
text of a decimal as a digit, let the exponent byte overflow, and sent
infinity and NaN, and the server returned another number, or on 11g bytes of
its own memory. It read the NUMBER of 1e-130, `80 02`, as 0, because a NUMBER
that starts with `80` is 0 only when it is that one byte. `TestEncodeDecimal`,
`TestNumberBounds` and `TestDecimalBind` hold it.

A batch binds one type for each argument. Each row takes the type of its own
value, where go-ora kept the type of the row before, so a decimal went as text
labeled `NUMBER` after a row of an `int`, and the server stored NULL. A NULL
takes the type of the other rows, and a batch of NULLs alone the type of its
NULLs. Short and long strings in one argument bind as `LONG`, short and long
`[]byte` as `LONG RAW`, a `DATE` and a `TIMESTAMP` as `TIMESTAMP`, and any
other mix is an error. A statement that runs again sends the definition of
its binds again when the type of one changed, where go-ora did so only for a
few pairs of types, and a batch of NUMBER after a batch of NULLs went as
text. A batch stops at the first row that fails, as python-oracledb runs
one, unless the caller asks for `WithBatchErrors`, which the section Errors
describes. go-ora asked for the errors of every batch, option `0x80000`, and
the server went on past the row, so rows after an ORA-00001 were stored and
the caller got the error alone. `TestBatches` holds the cases of
python-oracledb.

An object takes an attribute of every type that `ALL_TYPE_ATTRS` names:
`CHAR`, `NCHAR`, `INTEGER`, `SMALLINT`, `REAL`, `DOUBLE PRECISION`, `FLOAT`,
`BINARY_FLOAT`, `BINARY_DOUBLE`, `TIMESTAMP WITH TZ`, `TIMESTAMP WITH LOCAL TZ`,
the intervals, `BOOLEAN`, the LOBs and `XMLTYPE`. go-ora knew a few of the
names, and keyed the last one as `TIMESTAMP WITH LOCAL TIME ZONE`, which the
dictionary never writes. A `BINARY_FLOAT` attribute encodes as a binary float,
where go-ora sent a NUMBER under its type. A `TIMESTAMP WITH LOCAL TIME ZONE`
attribute is 11 bytes in the zone of the database, where go-ora wrote the
13 bytes of a zone. An `XMLType` attribute reads as the bytes of its image.
An object that is an OUT alone sends a NULL, where go-ora sent the zero value
of its destination, which 11g and 19c refused for its NULL `XMLType`.
`RegisterType` of a collection whose element is a registered collection marks
a copy of that coder as an element, where go-ora marked the registered coder
itself, so a bind of the inner collection hung. `TestObjectAllTypes`,
`TestNestedCollections` and `TestObjectXMLType` hold the cases of
python-oracledb, on 11g, 18c, 19c and 26ai.

A logical UROWID, which an index organized table has, reads as a `*` and the
base 64 of its bytes after the first, as python-oracledb writes it. go-ora
left the first byte zero and wrote each group of four characters over the
last, so the server refused the text with ORA-01410. A NULL ROWID or UROWID
reads as NULL, where go-ora read the ROWID of zeros, `AAAAAAAAAAAAAAAAAA`.
`TestRowIDs` holds the cases of python-oracledb.

A nullable value is `sql.Null[T]`, and the driver defines no nullable type of
its own. `NullTimeStampTZ` is gone, and `sql.Null[TimestampTZ]` takes its
place. The driver handles every nullable type through two interfaces, with no
case for each one:

- An argument that is a `driver.Valuer` goes through its `Value` method. A
  value type of the driver unwraps one with `types.ValueOf`, which a field of
  a user defined type needs too.
- A destination that is an `sql.Scanner` takes the plain value through `Scan`,
  with `types.ScanExternal`.

Three nullable types are the exception, because `sql.Null[T].Value` converts a
`time.Duration` to an `int64` and refuses an `IntervalYM` and a `TimestampTZ`.
`types.OwnNull` reads `sql.Null[time.Duration]`, `sql.Null[IntervalYM]` and
`sql.Null[TimestampTZ]` itself.

A NULL binds with the type of its value, which an OUT argument needs, for
`sql.NullX` and for `sql.Null[T]` of the common types. A NULL of any other
type binds as a NULL `VARCHAR2`.

A `string` binds as `VARCHAR2`, and an `NVarChar` as `NVARCHAR2`, in the
national character set (D31). The string coder of an `NVarChar` sends the
form of that character set, for each element of a `[]NVarChar` too, which a
PL/SQL array of `NVARCHAR2` needs and which an array of `VARCHAR2` fails with
PLS-00418. `sql.Null[NVarChar]` is a value that the driver binds itself, so
that `database/sql` does not make a `string` of it. `TestNChar` holds the
cases of python-oracledb.
A NULL `sql.Null[bool]` or `sql.NullBool` binds as a NULL
`BOOLEAN`, which a PL/SQL `BOOLEAN` takes. `database/sql` made a NULL of no
type of it, which PL/SQL refused with ORA-06550.

JSON and OSON (the binary JSON of Oracle) are read and written with
`encoding/json/v2` and `encoding/json/jsontext`.

## Errors

A sentinel error is a constant of the type `Error`, as in dbimp D6:
`ErrNotSupported`, `ErrScheme`, `ErrUnknownKey`, `ErrRepeatedKey`,
`ErrInvalidValue`, `ErrConnectorClosed`, `ErrAuthentication` and
`ErrCompilation`.

An error from the server is a `*ServerError`, with the `ORA-` code, the
message and the position in the statement. `Error` returns the message of the
server alone, with no line feed at its end, and `ErrPos` the position. go-ora
added a line with the position, which usql printed, and which held 0 for an
error with no position. Its `Is` method makes ORA-01017 and ORA-01005, for no
password, match `ErrAuthentication`, and ORA-24344 match `ErrCompilation`. An
account that is locked, ORA-28000, or whose password expired, ORA-28001, does
not match, since a new password does not help. This document said that
ORA-28000 matched, as dbimp D197 asks, which the code never did. ORA-28041
matched until usql W66, but the driver raises it itself for logon data of the
server that it cannot read. A statement that creates a unit that does not
compile returns ORA-24344, and a warning of the server goes to the `Logger`
(D32). A message is lower case, starts with a gerund and names what failed,
such as `reading the accept packet: %w`.

A `*ServerError` and an `Error` have the method `AuthenticationRefused()
bool`, which usql asks an error for to decide that the server refused the
credential, and so prompts for a password (usql W66). It is true where
`errors.Is(err, ErrAuthentication)` is: ORA-01017 and ORA-01005, and
`ErrAuthentication` itself, so that a new password can succeed. It is false
for a lack of a permission, ORA-01031 or ORA-01045, and for ORA-28000,
ORA-28001 and ORA-28041. usql declares the interface, so the driver imports
nothing of it. `TestAuthenticationRefused` and `TestConnectRefused` hold it.

A batch of DML with `WithBatchErrors` sends the execute option 0x80000, and
the server runs the rows that it can. It answers with a break, a reset, and
the return parameters, which hold the count of each row, then ORA-24381 with
the error and the index of each row that failed. The statement reads that
reply after the reset as its own, so that `WithRowCounts` gets the counts,
and returns a `*BatchError` (D34). The driver asked for the counts of every
batch already, and dropped them. `TestBatchErrors` holds the cases of
python-oracledb.

`driver.ErrBadConn` is returned only when the statement did not reach the
server, so that `database/sql` runs it again only when that is safe.

An error that ends the session marks the connection bad: ORA-00028 of a
killed session, ORA-01012, ORA-03113 and the other codes that python-oracledb
reads as a closed connection. The statement returns the error of the server.
`database/sql` then drops the connection from the pool, and a statement, a
ping or a transaction on a `sql.Conn` that holds it returns
`driver.ErrBadConn`. go-ora marked the connection bad and sent the next
statement on it all the same, which the server answered with ORA-01012.
`TestKilledSession` holds it.

## Logging

The driver writes no file and holds no logger of its own. A caller that needs
to see the packets sets `Config.Logger`, a `*slog.Logger`. The connection
writes each packet it sends and receives to it at `slog.LevelDebug`, as a hex
dump that the handler formats only when it writes the record, and each event
of the connection and each statement and bind at the same level. A break and
a bad connection go at `slog.LevelInfo`. A nil logger, the default, logs
nothing. A password and a key never reach the logger (D14).

The packets of a logon carry the encrypted session key and password, from
which a password can be found offline. So `tns.Session` withholds the bytes of
each packet from the start of a connection until the logon ends, and logs
only its length. The network encryption logs each packet decrypted under the
same rule. The keys of the Diffie Hellman exchange, which `go-ora` traced, are
not logged. Step 9 did this, and deleted `internal/trace` and the trace file
of the configuration. `TestLogger` holds it.

## Charsets

Step 8 built this on 2026-10-31. The tables of the 236 Oracle charsets were a
generated Go file of 11 MB, a `switch` of literals. They are now
`internal/charset/charsets.bin`, a blob of 2.0 MB that `go:embed` holds: the
tables as varints, one deflate stream for each charset, behind an index. A
charset is decoded the first time it is used, which takes about 1.4 ms for
ZHS16GBK, one of the largest. A test dumped every table of the generated code,
and every charset that the blob gives was equal to its dump, byte for byte,
before the generated file went.

A program that imports the driver and opens a database built to 22.6 MB, or
19.3 MB stripped, before, and to 12.5 MB, or 9.2 MB stripped, after.

`Converter.Clone` did not copy the `leading` table, so a clone of an EUC
charset, such as 0x35C or 0x35F, decoded wrongly. It copies it now, and
`TestCloneKeepsLeading` holds it.

`internal/converters` is split. The charsets are `internal/charset`, with
`charset.Converter`, `charset.New` and `charset.MaxBytesPerChar`. The NUMBER
and float code of `converters` had no user outside its own tests, because
`internal/types` holds its own, so it is gone with its tests and its
generator.

## Go 1.27 idioms

The code uses these, where each fits:

- `any`, `min`, `max`, a `for` range over an integer, and `new(expr)`.
- The `slices`, `maps` and `iter` packages, in place of a loop that copies.
- `sync.OnceValue` and `sync.OnceValues`, in place of a flag and a mutex.
- `context.AfterFunc`, for a break.
- `errors.Join`, where two errors both reach the caller, as for a statement
  that failed and a cursor that did not close.
- `encoding/json/v2`, `slog`, and `sql.Null[T]`.
- `driver.RowsColumnScanner`.
- In tests, `t.Context()`, `b.Loop()`, and `testing/synctest` for deadlines.
- `reflect.TypeFor[T]()`, in place of `reflect.TypeOf((*T)(nil)).Elem()`.

The `go fix` of Go 1.27 does most of the mechanical part. The rest is review.

## The text of a statement

`parseSQL` in `internal/ttc` reads the text of a statement once, as the
parser of python-oracledb does. It finds three things:

- The kind of the statement, from its first word. `DECLARE` and `BEGIN` are
  PL/SQL, `SELECT` and `WITH` a query, `INSERT`, `UPDATE`, `DELETE` and
  `MERGE` DML. `CALL` is PL/SQL too, as python-oracledb reads it.
- Whether DML has `RETURNING` or `RETURN`, then `INTO`.
- The names of the binds. A bind is a colon outside a comment, a string, a
  q-string or a quoted name, then a name. Spaces can come before the name. A
  name in quotes keeps its case and holds any character, and any other name
  is in upper case. A colon after a string is a part of a JSON constant, as
  in `json_object('k':v)`, and is not a bind. DDL has no binds, so the `:new`
  and `:old` of a trigger are not binds.

A string, a quoted name or a q-string with no end is an error, which a bind
by name returns. The server reports it for any other statement.

The driver uses the names only for a bind by name. An argument matches a
name in quotes in its case, and any other name in any case. go-ora matched a
name in quotes in any case. SQL takes a bind for each
place a name appears, and PL/SQL one for each name. `CALL` takes one for
each place, as SQL does, but the IO vector of its response holds one
direction for each name. The driver gives the direction to the first place
of the name. 26ai counts the names in the IO vector and 11g counts the
binds, so the driver does not use the count.

go-ora removed the comments and the strings and took each colon and word
that was left. It took the words of a q-string and the colon of a JSON
constant, and it took no quoted name and no name with a letter outside
ASCII. A bind by name of such a statement failed. It read `CALL` as neither
SQL nor PL/SQL, so a `CALL` with an OUT failed with ORA-03146, and the
server then closed the connection with ORA-03138. `TestParseNames` and
`TestParseKind` hold the cases of python-oracledb, and `TestBindNames` and
`TestCallBindNames` in `integration` run them on the server.

## Statements that run again

A prepared statement keeps its cursor on the server, and runs it again with no
parse. A run that fails returns the cursor with the error, and the next run
parses again into that cursor. A parse sends the text with its pointer, as
python-oracledb sends it. go-ora sent the pointer only for a new cursor, so
the second run of a statement that failed got ORA-03120. A query with a LOB of
the inline mode runs, then defines its columns, and then each later run
executes again. go-ora left the execute out after the define, so each later
run was a fetch with the binds, which failed with ORA-01002 once a bind
changed type. `TestCursorOther` holds the cases of python-oracledb. The server
keeps the types in which the cursor fetched its columns. When a view or a
table under the cursor changes the type of a column, the server parses again
and sends a new describe with the rows. It converts a new type to the old type
of the fetch when it can, and the rows come in the old type: a CLOB that was a
string or a LONG comes as a LONG, a BLOB that was a RAW comes as a LONG RAW,
and a string that was a CLOB of the inline mode comes as a LONG. The driver
reads each column so, as python-oracledb does. When the server cannot convert,
it fails the execute with ORA-00932 or ORA-01007, and the driver closes the
cursor and runs the query once more, with a parse. go-ora kept the flags of
LOB and LONG columns from the first run and read the new rows in their new
types, so 14 of 18 changes failed. `TestTypeChanges` in `integration` holds
the cases of python-oracledb.

The driver sends the binds of a statement again when one grows or changes
type. For DML RETURNING it sends the option of PL/SQL binds every time, as
python-oracledb does, where go-ora left it out then, and the server refused
the execute with ORA-03146 (sijms/go-ora#697). `RETURNING` is a word outside
strings and comments, so `RETURNING(id)INTO` is DML RETURNING too. The server
cuts a value of RETURNING to the size of its OUT, and sends the length that it
had, and the driver returns an error for it, where go-ora returned the cut
value. An `UPDATE` of no row gives each OUT of RETURNING a NULL. `TestRebind`
and `TestReturning` hold it. A RETURNING of more than one row fills an OUT of
a slice, one element for each row, and fails for an OUT of one value (D35).
`TestReturningRows` holds it.

## Testing

A unit test needs no server. `internal/tnstest` holds the two halves:

- `Recorder` is a dialer that dials a real server and keeps every byte that
  passes. It splits the bytes into TNS packets, with the length of two bytes
  before the accept packet and of four after an accept of version 315 or
  later.
- `Player` is a dialer that returns one end of a `net.Pipe`, and plays a
  recorded exchange on the other end. It writes each packet that the server
  sent, and reads each packet that the client sends and compares its type.
  It compares no more, because the client sends random keys and the time.

An exchange is a text file under `testdata/exchanges/<release>/`, named for its
scenario. Each line is a note, which starts with `# `, or a packet, which is
`C` or `S` and the bytes in hex. The notes hold the release, the scenario, the
user, the password and the service, and one `result` line for each fact that
the scenario returns, quoted as a Go string. The password is the fixed test
password of `dbrun`. The O5LOGON exchange encrypts the session key of the server
with it, so a replay needs it.

`Player` reads the packets of the client in a goroutine of its own. A pipe
holds no buffer, and on 11g the server sends a marker packet that the client
answers while the server still writes, so both writes wait for each other
without the goroutine. Each exchange has a deadline of one minute, so that a
driver that waits for a packet that the exchange lacks fails the test.

`exchange_test.go` in the root package holds the scenarios: `ping`, `query`,
`error`, `dml` and `clob`. `TestExchanges` plays every file back and compares
the results. `TestRecordExchanges` records them again with `-record`, against
the release that `ORACLE_DSN` and `ORACLE_RELEASE` name. The comment at the
top of `exchange_test.go` gives the command. Every recorded
connection names the address `oracle.test:1521` and a fixed client identity,
in the `CID` key, so the packets do not depend on the machine.

Each `Connector` keeps its own fast login cookies (D9), and a cookie changes
the logon. Each scenario opens its own `Connector`, so each starts with none.

A replay passes only while the driver sends the same packet types in the same
order. A change that sends another packet, such as a new round trip at logon,
needs the exchanges recorded again. That is the point: it shows each change to
what the driver sends.

A test that needs a server reads `ORACLE_DSN` and skips when it is empty. CI
sets it from `dbrun dsn --json` for every release (D7). `upstream_test.go` in
the root package holds a test for each upstream fault that step 9 fixed,
named for its fault, and its doc comment names the issue. `integration`, which
was `TestIssues`, holds the integration tests of `go-ora`.

In step 9 the root package and `integration` passed on 11g, 18c, 19c, 21c,
23ai, 26ai, and the two Autonomous Database releases, `oracle-adb-19c` and
`oracle-adb-26ai`. CI leaves 19c out (D17), because its first start on a new
machine takes 14 minutes, and runs it on this machine (D29). `dbrun dsn`
prints the URL of an Autonomous Database with the TLS options of `go-ora`,
`ssl=true&ssl+verify=false`, which this parser refuses. A test gives it
`sslmode=require` in their place. `oracle-nne-26ai` is a server that requires
native network encryption and checksums, for `TestNetworkEncryption`. CI
leaves it and the Autonomous Database releases out, because dbmeta verifies
them on a machine.

## Idiomatic names and structure

Ken asked on 2026-10-31 that the internal packages read as idiomatic Go, with
names that agree inside and outside the module. The code of `go-ora` carries
the conventions of other languages. An inventory after step 6 found this, and
each item names its fix.

### Folders and packages

| Was | Fix |
| --- | --- |
| `TestIssues/`, 50 files of integration tests, package `TestIssues` | `integration/`, package `integration`, done. One file for each area, and not for each upstream issue, done on 2026-11-02 |
| `internal/utils` | Deleted. `Max` is the built in `max`. `CreateQuasiLocator`, `GetValue`, `GetType` and the `Ty` variables repeat code of `internal/types` and `internal/ttc`. `ExtractTag` moves to the code of user defined types |
| `internal/lazyinit` | Deleted. `sync.OnceValue` does the same |
| `internal/converters` | `internal/charset`, done in step 8. Its NUMBER code had no user, and is gone |
| `internal/configurations` | `internal/config`, done |
| `internal/types` | Stays, as the package of the Oracle values |
| `internal/trace` | Gone in step 9. `Config.Logger` replaces it (D14) |

### Names

| Was | Fix |
| --- | --- |
| `IConnection`, `IStringConverter` | No `I` prefix. An interface is named for what it does, and is defined where it is used, with the methods that the user calls. `IConnection` becomes the few methods that a coder calls |
| `NTSAuthInterface`, `KerberosAuthInterface`, `PacketInterface`, `StmtInterface`, `DialerContext` | No `Interface` suffix. `DialerContext` and `KerberosAuthInterface` repeat `oracle.Dialer` and `oracle.Kerberos` |
| `GetInt`, `GetByte`, `GetClr`, `GetMaxLen` and the other `Get` methods | No `Get` prefix: `Int`, `Byte`, `CLR`, `MaxLen` |
| `TNS_TYPE_REP_UNIVERSAL`, `MAX_LEN_TIMESTAMP`, `TimeStampTZ_DTY`, `LobReadMode_AUTO` | Mixed caps, with no underscore. The Oracle type IDs become constants of a named type |
| `Json`, `TimeStampTZ` | `JSON`, `TimestampTZ`. An initialism keeps one case, and timestamp is one word |
| `TyInt`, `TyNullString` and the other 52 `Ty` variables | Delete. A type switch replaces each map that is keyed by `reflect.Type` |
| `NewQuasiLocator`, `IsQuasi` | `inlineLocator`. A quasi locator is a LOB locator that the driver makes for a value that came inline, so the LOB code reads it with no round trip |

### Interfaces

24 interfaces exist. An interface stays when a caller supplies an
implementation, such as `oracle.Dialer`, `oracle.Kerberos` and `oracle.Querier`,
or when it breaks an import cycle with no other way. Every other one goes, and
its user takes the concrete type: `AdvNegoService`, `AdvancedNegotiator`,
`OracleNetworkEncryption`, `OracleNetworkDataIntegrity`, `TerminalReader`,
`ValueStreamReader`, `ValueStreamWriter`, `ValueStreamer`, `SessionReadWriter`,
`StmtInterface`, `OracleType`, `JsonCoder`, `JsonEncoder` and `JsonDecoder`.
`SessionReader` and `SessionWriter` stay only while `internal/codec` and
`internal/types` cannot import `internal/tns`, and the move of the coders into
`internal/ttc` removes that need.

### Reflection

`reflect` appears 295 times in `internal/ttc`, 158 in `internal/types`, 48 in
`internal/utils` and 42 in OSON. Most of it repeats `database/sql`:

- `types.RCopy`, `copyx.go` and the `CopyTo` method of each value type assign
  a value to a destination of any type. `sql.ConvertAssign` does that, and
  rows use it through `RowsColumnScanner` already. OUT arguments use it too,
  and these go.
- The maps of coders keyed by `reflect.Type`, and the `Ty` variables, choose a
  coder for an argument. A type switch over the Go types that the driver binds
  does it, after `database/sql` converts a `driver.Valuer` and a named type
  with `driver.DefaultParameterConverter`.
- The kind switches of `internal/utils` go with that.

Two uses stay, each in one file: the mapping of a user defined type to a
struct by its `udt` tags, which `encoding/json` does with `reflect` too, and an
array bind of a slice of any element type. OSON loses its own mapping of
structs in step 7, because `encoding/json/v2` does it.

### Dead code

7,553 lines of `internal/ttc`, `internal/types`, `internal/codec` and
`internal/tns` were code that was commented out. Two passes deleted them, and
git history keeps them. A sweep in step 9 deleted 860 more lines, the empty
files, and the functions that nothing calls, which `deadcode -test` found.

### Where the cleanup stands

Done, on 2026-10-31:

- A tool deleted 6,011 lines of code that was commented out. It parsed each
  group of comments as Go, so a comment that is prose stayed.
- `internal/utils` and `internal/lazyinit` are gone. Each regular expression
  is a `regexp.MustCompile` of the package, `Max` is `max`, and `ExtractTag`,
  `GetType`, `GetValue` and `Execuer` moved to the one package that used each.
  `aq.CreateQueue` takes an `aq.Execer`, which a caller can name.
- `types.RCopy`, `copyx.go` and the 11 `CopyTo` methods are gone.
  `copyValue` in `internal/ttc` stores an OUT argument, a field of a user
  defined type and an element of an array through `sql.ConvertAssign`. A NULL
  in a destination that cannot hold one is its zero value, as in `go-ora`.
- The 52 `Ty` variables are gone. `builtinCoder` chooses the coder of a Go
  type with a type switch, and the map keyed by `reflect.Type` holds only the
  types that `RegisterType` registers.

`reflect` fell from 158 to 19 uses in `internal/types` and from 295 to 151 in
`internal/ttc`, where the mapping of user defined types and struct tags holds
most of what is left.

Also done, the same day:

- Every declared name with an underscore, 74 of them, has mixed caps:
  `TNSTypeRepUniversal`, `IntervalDSDTY`, `TimestampTZDTY`, `LobReadModeAuto`.
  `gopls rename` made the changes, except two in `internal/ttc`, which the
  `gopls` of this machine cannot load, because it predates the promoted fields
  of Go 1.27 in a composite literal.
- `IStringConverter` is gone, because `*StringConverter` is its one
  implementation. `IConnection` is `codec.Conn`, `NTSAuthInterface` is
  `NTSAuthenticator`, `KerberosAuthInterface` is `KerberosAuthenticator`,
  `DialerContext` is `Dialer`, `PacketInterface` is `packet` and
  `StmtInterface` is `parentStmt`.
- `Json` is `JSON`, and `TimeStamp` is `Timestamp` in every name. The Oracle
  type ID that was `JSON` is `JSONType`. A quasi locator is an inline locator,
  `NewInlineLocator` and `IsInline`, with a comment that says what it is.
- `internal/configurations` is `internal/config`. `TestIssues` is
  `integration`, a package of tests alone, with a README for `dbrun`.

Then, with Ken's agreement on the names:

- No method starts with `Get`. A read from the stream of a session starts
  with `Read`: `ReadInt`, `ReadCLR`, `ReadDLC`, and `ReadByte`, which is now
  `io.ByteReader`. An accessor drops the prefix: `MaxLen`, `Locator`,
  `DBTimeZone`, `ParameterCoder`. `Session.GetError` is `ServerError`, so that
  a session is no `error`, and `StringConverter.GetLangID` is gone, because
  `LangID` is its field. `GetDBVersion` is `ReadDBVersion`.
- Each interface that stays has a reason. `negoService`, the ciphers, the
  wallet algorithms, the session readers and writers and `parentStmt` have
  more than one implementation. `Negotiator`, `LobStreamer`, `Lob`,
  `OracleType` and `codec.Conn` break an import cycle. `AdvNegoService`,
  `AdvancedNegotiator`, `TerminalReader` and `ValueStreamWriter` took the
  names `negoService`, `Negotiator`, `terminalReader` and `ValueWriter`, and
  `ValueStreamReader` and `ValueStreamer` had no user and are gone.

The Oracle type IDs are constants of the type `types.TypeID`, which
`ParameterCoder` takes. The integration tests are in one file for each area,
and a test of an upstream fault names the issue in its doc comment.

### JSON

Step 7 built JSON on `encoding/json/v2`. OSON is the binary JSON of Oracle on
the wire, so the driver keeps a codec for it, and that codec deals in JSON text
and nothing else:

- `oson.EncodeText` reads JSON text with a `jsontext.Decoder` into a tree of
  maps, slices, strings, numbers as text, booleans and nil, and writes OSON.
- `oson.DecodeText` writes the OSON as JSON text with a `jsontext.Encoder`,
  with a number as its exact digits. A date, a timestamp, an interval, a RAW,
  an infinity and a NaN are strings, in the form that `JSON_SERIALIZE`
  writes, such as `2022-12-05T15:06:05`, `P1DT2H3M4.500000S` and `0102AB`.
  A `VECTOR` is an array of its numbers, and a sparse one the array of its
  dimensions, its indices and its values, as Oracle writes it.
- `types.JSON` holds the JSON text. `NewJSON` and `SetValue` take JSON text,
  or any Go value through `json.Marshal`. `codec.JSONParameter` turns the text
  into OSON to bind it, and `ttc` turns the OSON of a row into text, after a
  load when the server streams it.
- A JSON column scans into `*any` as a `jsontext.Value`, into a `*string`, a
  `*[]byte` or a `*jsontext.Value` as text, and into any other destination
  through `json.Unmarshal`.
- A CLOB or a BLOB with an `IS JSON` check is a CLOB or a BLOB, and scans as
  one. The server gives it the flag of JSON, as it gives a column of the JSON
  and the VECTOR types. go-ora defined it as a locator in the inline mode and
  then read the row as inline text, which failed with `TTC error: received
  code 0`. `TestJSONCheckedLobs` holds it.

The interfaces `JSONCoder`, `JSONEncoder` and `JSONDecoder`, the `Coder`
field, the type `Oson`, the global map of field encoders and the `reflect` of
OSON are gone. `types.JSON` cannot import `oson`, which imports `types` for
NUMBER and dates, so `codec` and `ttc` make the conversions. A later step can
move NUMBER and dates below both. Binding JSON never worked in `go-ora`: it
sent a NULL, because the coder was set after the value. `TestJSON` binds and
reads it on 21c and 26ai.

OSON holds a name of up to 255 bytes in version 1 and of up to 65535 bytes in
version 3, which puts the long names in a segment of their own. The codec
reads both, and writes version 3 when a name is longer than 255 bytes. A name
hashes with the 32 bits of FNV-1a over its UTF-8, and the names sort by the
low byte of the hash, then the length, then the bytes, as the server sorts
them. go-ora read version 3 as corrupt, hashed with a sum of its own, and cut
a long name to its low byte of length. The flags that size the header follow
the count of names and the size of each segment, so a document of more than
255 names or 64 KiB of names reads back. A JSON column asks for 32 MiB with
its locator and a `VECTOR` column for 1 MiB, the largest of each, as
python-oracledb asks. go-ora asked for 32 KiB, and the server cut a longer
document with no error when the LOBs came inline. `TestJSONFieldNames` holds
the cases of python-oracledb, and `TestNames` in `internal/codec/oson` the
sizes of the header.

The codec reads each node type of OSON as python-oracledb reads it. go-ora
read no date, timestamp, interval, RAW, ID or `VECTOR`, read an interval as a
`NUMBER`, and read no integer that holds its length in its node type, so a
document of any of them failed or read wrong. A document that is a scalar,
such as `true` or `"text"`, has a header of its own, which the codec now
writes. go-ora refused it. Each object writes its own field IDs, as
python-oracledb does. go-ora pointed an object at the field IDs of an earlier
object with the same names, and the server failed with ORA-40596 on a path
into an array of such objects. An OUT JSON reads as its text, which go-ora
left empty, and a NULL JSON of stream mode reads as NULL. `TestJSONValues`
and `TestJSONTypes` hold the cases of python-oracledb, and each scalar type
reads as the text that the server writes for it.

A batch writes every row with one coder, that of its last value that is not
NULL, and a JSON or a `VECTOR` writes the locator of its own row, which holds
its length. go-ora wrote every row with the coder and the locator of the last
row, so the server refused a row longer than the last with ORA-51803, and a
NULL last row went as a `VARCHAR2` and hung the connection. `TestJSONBatch`
and the batch of `TestVectorColumns` hold the cases.

On 21c, `SELECT CAST(:1 AS JSON) FROM dual` with a JSON bind fails inside the
server, with ORA-00600 [kcbgtcr_1] or [25026]. python-oracledb fails on it
too, and the server drops its session. It is a fault of 21c. The driver
marks the connection bad on ORA-00600, and the pool serves the next query.
The same bind inserts into a JSON column, `JSON_SERIALIZE(CAST(:1 AS JSON))`
works, and 26ai returns it.

### VECTOR

D30 sets the Go types of a `VECTOR`: `[]float32`, `[]float64`, `[]int8`,
`[]byte` for `BINARY`, which holds 8 dimensions in each byte, and
`SparseVector`. `internal/types/vector.go` reads and writes the image of
python-oracledb: a magic byte, a version, flags, the format, the number of
dimensions, 8 bytes for the norm, and for a sparse vector the number of its
elements and their indices before them. go-ora read the number of dimensions
in a width that it took from the flags, so a `BINARY` vector read wrong, and
read `INT8` as unsigned. A NULL `VECTOR` scans into a slice as nil, in both
LOB modes. `BINARY_FLOAT` and `BINARY_DOUBLE` encode by their sign bit, where
go-ora flipped the bits of a zero, which read back as NaN, and decoded in
place, so a second read was wrong. `TestVectorColumns`, `TestVectorPLSQL`,
`TestBinaryVector` and `TestSparseVector` hold the cases of python-oracledb,
and the server converts between the formats.

## The order of the work

Each step leaves a driver that builds and passes its tests.

1. Make the tests build, and raise `go.mod` to Go 1.27.1. Run `go fix`.
2. Record the exchanges of logon, a query, a DML statement, a LOB and a
   transaction against each release, and write the fake server.
3. Move the protocol code to `internal/`, with no change of behavior.
4. Write the root package as this document says, as the package `oracle`:
   `Driver`, `Connector`, `Config` with the DSN, and the errors. Delete the
   package variable and the functions that take a `*sql.DB` for state.
4b. Move the TTC code of the root package to `internal/ttc`, so that
   `Connection`, `Stmt`, `DataSet`, `ParameterInfo` and the other types that
   `go-ora` exported are hidden from callers. Write the configuration of the
   protocol code from `Config` directly, and delete the parser of `go-ora`.
5. Replace `StartContext` with deadlines and `context.AfterFunc`.
6. Change the values to the table above, and add `RowsColumnScanner`.
7. Move JSON and OSON to `encoding/json/v2`.
8. Compact the charsets.
9. Fix the upstream faults in `UPSTREAM.md`, each with a test.
10. Tell the consumers about the package name and the keys of the DSN. They are
    `dburl`, `usql` and `dbmeta`. `dburl` staged a scheme on 2026-10-31 that
    passes the query through. The parser landed in step 4. The first tag,
    v0.1.0, exists since 2026-11-02, and `dburl` and `usql` were told.
11. Make the names and the structure idiomatic, as the section above says.
    It runs before step 7, so that JSON and the charsets move into the new
    layout, in this order: delete the dead code, delete `internal/utils` and
    `internal/lazyinit`, replace the copy machinery with `sql.ConvertAssign`,
    replace the maps keyed by `reflect.Type`, remove the interfaces, rename,
    and move `TestIssues` to `integration`.

Steps 4 to 6 change what a caller sees, and nothing was tagged until they were
done. v0.1.0 is the first tag (D36).
