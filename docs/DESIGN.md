# Driver design

This document is the target design for the rewritten driver. It is a proposal
until Ken accepts it, and D8 to D16 in [PLAN.md](PLAN.md) record it. The design
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
| One package variable, `oracleDriver`, holds the session parameters, the registered types, the coders, the TLS configuration and the string converters of every database | Two databases in one process share them. A string converter set by the first connection decodes the text of the second, which is sijms/go-ora#692 | Connector |
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
`internal/`, and changed nothing else. The third column says where each one
was. The 30 recorded exchanges passed before and after the move, so the driver
sent the same packets.

| Path | Holds | In `go-ora` |
| --- | --- | --- |
| `/` | The driver, the connector, the DSN, the options, the errors and the value types that a caller passes or scans | the root package. Step 6 moved `types` to `internal/types`, and the root package names the types a caller uses, as aliases |
| `aq/` | Advanced Queuing | `aq` |
| `internal/ttc/` | The connection, statements, rows, transactions, LOBs, cursors, user defined types, the negotiation of data types, O5LOGON, and the registry of types | the root package, which step 4b moved |
| `internal/configurations/` | The configuration of the protocol code, and wallets | `configurations` |
| `internal/tns/` | Packets, the session, TLS, the redirect, out of band breaks | `network` |
| `internal/auth/` | The advanced negotiation of Oracle Net: encryption, checksums, Kerberos and NTS | `advanced_nego` |
| `internal/auth/security/` | DES and RC4, and the padding that O5LOGON uses | `network/security` |
| `internal/auth/ntlmssp/` | NTLM, with the MD4 hash that it needs | `advanced_nego/ntlmssp`, `network/security/md4` |
| `internal/codec/` | The encoders and decoders of each Oracle type | `parameter_coder` |
| `internal/codec/oson/` | OSON, the binary JSON of Oracle | `types/oson` |
| `internal/converters/` | The charsets, NUMBER, dates and floats | `converters` |
| `internal/trace/`, `internal/lazyinit/`, `internal/utils/` | Helpers | `trace`, `lazy_init`, `utils` |

Later steps finish the layout:

- Step 4b moved the TTC code of the root package to `internal/ttc/`. O5LOGON
  is in `internal/ttc/auth_object.go`, and the wallets in
  `internal/configurations`. Both can move to `internal/auth/` later.
- Step 8 splits `internal/converters` into `internal/charset`, the compacted
  charsets, and `internal/codec`, the number and date encoders.
- `internal/trace` goes with D14, and the code of `internal/lazyinit` and
  `internal/utils` moves to the package that uses it.

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
| `options.go` | `Option` and `WithOptions` |
| `errors.go` | `Error`, the sentinel errors, and `*ServerError` |
| `values.go` | The Oracle types that Go lacks, such as `IntervalYM`, `RowID`, `Vector` and `JSON` |
| `lob.go` | `Clob`, `Blob` and `BFile` |
| `object.go` | User defined types and collections |

After step 4b the root package holds `driver.go`, `connector.go`, `dsn.go`,
`errors.go` and `values.go`. `values.go` names the types of `internal/ttc`
that a caller uses, such as `Out` and `RefCursor`, as aliases. `conn`, `stmt`,
`rows` and `tx` are still `ttc.Connection`, `ttc.Stmt`, `ttc.DataSet` and
`ttc.Transaction`, and steps 5 and 6 reshape them there.

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
sends them at logon, and `conn` keeps them. Nothing caches them across
connections. That fixes sijms/go-ora#692.

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

A LOB that the driver reads while it fetches rows, or uploads while it binds a
statement, runs inside a call that is watched already, or under `Next`, where
`database/sql` gives no context. It reads with `Lob.Load`, which watches
nothing, so one call never has two watches. `CreateBlob`, `CreateClob` and
`CreateBFile` take the context first, and the LOB types keep none.

`Driver.Open` and `Connection.Begin`, the forms with no context, fail with an
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

A user named `SYS` gets no privilege by its name. The DSN names it with
`privilege=sysdba`.

## Options

A statement takes options as dbimp D109 sets them out. An option comes from the
DSN, then from the context through `WithOptions`, then from an argument of the
type `Option`, and a later one wins. `CheckNamedValue` takes an `Option` out of
the arguments. The common options are `WithTimeout`, the time that the server
gives one statement, and `WithReadonly`. The options of this driver are
`WithPrefetch` and `WithLobFetch`. dbimp holds the machinery in its root
package. This module cannot import it (hard rule 2), so it holds a copy of the
part it needs.

## Values

A NULL is nil. A value has the Go type that fits its Oracle type, and `rows`
implements `driver.RowsColumnScanner`, so that `Scan` fills the destination of
the caller with no copy through `any`.

| Oracle | Go, for a scan into `*any` | Also scans into |
| --- | --- | --- |
| `VARCHAR2`, `CHAR`, `NVARCHAR2`, `NCHAR`, `LONG`, `CLOB`, `NCLOB` | `string` | `[]byte`, `Clob` |
| `NUMBER` with scale 0 and precision 18 or less | `int64` | the numeric types, `string` |
| `NUMBER` with any other precision or scale | `*apd.Decimal`, which holds every digit | `apd.Decimal`, `string`, `float64`, `int64` |
| `BINARY_FLOAT`, `BINARY_DOUBLE` | `float64` | `float32` |
| `DATE`, `TIMESTAMP` | `time.Time` in UTC, because the type holds no zone | |
| `TIMESTAMP WITH TIME ZONE` | `time.Time` in its zone | |
| `TIMESTAMP WITH LOCAL TIME ZONE` | `time.Time` in the zone of the session | |
| `INTERVAL YEAR TO MONTH` | `IntervalYM` | `string` |
| `INTERVAL DAY TO SECOND` | `time.Duration` | `string` |
| `RAW`, `LONG RAW`, `BLOB` | `[]byte` | `Blob` |
| `BOOLEAN` | `bool` | |
| `JSON` | `JSON`, which holds the document | `string`, any type that `encoding/json/v2` decodes |
| `VECTOR` | `Vector` | the slices of each element type |
| `ROWID`, `UROWID` | `string` | `RowID` |
| `REF CURSOR` | `*sql.Rows` through `driver.Rows` | |
| An object or a collection | the type that `RegisterType` registered | |

Step 6 built the rows of `NUMBER`, `BINARY_FLOAT`, the intervals and
`RowsColumnScanner`, and `values_test.go` checks each against a server. These
rows are not built yet: `JSON` and OSON on `encoding/json/v2` are step 7, and
the dates keep the behavior of `go-ora` until `TestTime` is fixed. The
backlog holds both.

The type of a column decides the Go type, and the value never does, so every
row of a column has the same type. `COUNT(*)` is a `NUMBER` with no precision,
so it is an `*apd.Decimal`, and it scans into an `*int64`.

`ScanColumn` stores a value as dbimp `Assign` does. A `*any` receives it as it
is, and an `*apd.Decimal` receives a decimal. Every other pair goes to
`sql.ConvertAssign`, and a decimal goes to it as text.

An interval is an `IntervalYM` or a `time.Duration`, and both bind as an
argument. `go-ora` read an interval as a `time.Time`, so `+2021-10` came back
as 2021-09-30, and it wrote the nanoseconds of a day to second interval as
microseconds and read them back multiplied by 1000. The same fault sent the
time zone of the client at logon with 30 days added.

A decimal is an `*apd.Decimal` from `github.com/cockroachdb/apd/v3`, a new one
for each value, as in dbimp D33 (D16). `NUMBER` holds 38 digits, which neither
`float64` nor `int64` holds.

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

JSON and OSON (the binary JSON of Oracle) are read and written with
`encoding/json/v2` and `encoding/json/jsontext`.

## Errors

A sentinel error is a constant of the type `Error`, as in dbimp D6:
`ErrNotSupported`, `ErrScheme`, `ErrUnknownKey`, `ErrRepeatedKey`,
`ErrInvalidValue`, `ErrConnectorClosed` and `ErrAuthentication`.

An error from the server is a `*ServerError`, with the `ORA-` code, the message
and the position in the statement. Its `Is` method makes ORA-01017 and
ORA-28000 match `ErrAuthentication`, as dbimp D197 asks. A message is lower
case, starts with a gerund and names what failed, such as
`reading the accept packet: %w`.

`driver.ErrBadConn` is returned only when the statement did not reach the
server, so that `database/sql` runs it again only when that is safe.

## Logging

The driver writes no file and holds no logger of its own. The `trace` package
and the `TRACE FILE` key go. A caller that needs to see the packets sets
`Config.Logger`, a `*slog.Logger`. The connection writes each packet it sends
and receives to it at `slog.LevelDebug`, and each break and each reconnect at
`slog.LevelInfo`. A nil logger, the default, logs nothing. A password and a
key never reach the logger (D14).

## Charsets

The 236 charset tables move to `internal/charset` in the compact form that
`BACKLOG.md` describes: one blob of varints, deflated one charset at a time,
read with `go:embed`, and decoded on first use. The work is in the backlog, and
it is a separate change from this design.

## Go 1.27 idioms

The code uses these, where each fits:

- `any`, `min`, `max`, a `for` range over an integer, and `new(expr)`.
- The `slices`, `maps` and `iter` packages, in place of a loop that copies.
- `sync.OnceValue` and `sync.OnceValues`, in place of a flag and a mutex.
- `context.AfterFunc`, for a break.
- `errors.Join`, where two errors both reach the caller, as for a statement
  that failed and a cursor that did not close.
- `encoding/json/v2`, `uuid.UUID`, and `sql.Null[T]`.
- `driver.RowsColumnScanner`.
- In tests, `t.Context()`, `b.Loop()`, and `testing/synctest` for deadlines.
- `reflect.TypeFor[T]()`, in place of `reflect.TypeOf((*T)(nil)).Elem()`.

The `go fix` of Go 1.27 does most of the mechanical part. The rest is review.

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

The driver keeps a cache of fast login cookies for the whole process, and a
cookie changes the logon. So each scenario clears it first, through
`ResetCookies` in `export_test.go`. D9 moves the cache to the `Connector`, and
the hook goes with it.

A replay passes only while the driver sends the same packet types in the same
order. A change that sends another packet, such as a new round trip at logon,
needs the exchanges recorded again. That is the point: it shows each change to
what the driver sends.

An integration test reads `ORACLE_DSN` and skips when it is empty. CI sets it
from `dbrun dsn --json` for every release (D7). `integration`, which was `TestIssues`, becomes these
tests, one for each upstream issue that this design fixes, named for the issue.

## Idiomatic names and structure

Ken asked on 2026-10-31 that the internal packages read as idiomatic Go, with
names that agree inside and outside the module. The code of `go-ora` carries
the conventions of other languages. An inventory after step 6 found this, and
each item names its fix.

### Folders and packages

| Today | Fix |
| --- | --- |
| `TestIssues/`, 50 files of integration tests, package `TestIssues` | `integration/`, package `integration`, done. One file for each area, and not for each upstream issue, is still to do |
| `internal/utils` | Delete. `Max` is the built in `max`. `CreateQuasiLocator`, `GetValue`, `GetType` and the `Ty` variables repeat code of `internal/types` and `internal/ttc`. `ExtractTag` moves to the code of user defined types |
| `internal/lazyinit` | Delete. `sync.OnceValue` does the same |
| `internal/converters` | `internal/charset`, the charsets, and the NUMBER and date encoders into `internal/types` (step 8) |
| `internal/configurations` | `internal/config` |
| `internal/types` | Stays, as the package of the Oracle values |
| `internal/trace` | Goes with D14, when `Config.Logger` replaces it |

### Names

| Today | Fix |
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
`internal/tns` are code that is commented out. Delete them. Git history keeps
them.

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

Left: the JSON coder interfaces, which step 7 removes, `internal/converters`
until step 8 splits it, and one file of integration tests for each area.

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
    passes the query through, and it rechecks when the parser of step 4 lands.
11. Make the names and the structure idiomatic, as the section above says.
    It runs before step 7, so that JSON and the charsets move into the new
    layout, in this order: delete the dead code, delete `internal/utils` and
    `internal/lazyinit`, replace the copy machinery with `sql.ConvertAssign`,
    replace the maps keyed by `reflect.Type`, remove the interfaces, rename,
    and move `TestIssues` to `integration`.

Steps 4 to 6 change what a caller sees. Nothing is tagged until they are done.
