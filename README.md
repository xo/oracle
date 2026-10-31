<div align="center">
  <a href="#installation" title="Installation">Installation</a> |
  <a href="#quick-start" title="Quick Start">Quick Start</a> |
  <a href="#connection-options" title="Connection options">Connection options</a> |
  <a href="#values" title="Values">Values</a> |
  <a href="#migration-from-go-ora" title="Migration from go-ora">Migration</a> |
  <a href="#related-projects" title="Related Projects">Related Projects</a> |
  <a href="https://pkg.go.dev/github.com/xo/oracle" title="Go Reference">Reference</a> |
  <a href="CONTRIBUTING.md" title="Contributing">Contributing</a>
</div>

<br/>

[![Unit Tests][oracle-ci-status]][oracle-ci]
[![Go Reference][goref-oracle-status]][goref-oracle]
[![Releases][release-status]][releases]
[![Discord Discussion][discord-status]][discord]

[oracle-ci]: https://github.com/xo/oracle/actions/workflows/test.yml "Test CI"
[oracle-ci-status]: https://github.com/xo/oracle/actions/workflows/test.yml/badge.svg "Test CI"
[goref-oracle]: https://pkg.go.dev/github.com/xo/oracle "Go Reference"
[goref-oracle-status]: https://pkg.go.dev/badge/github.com/xo/oracle.svg "Go Reference"
[release-status]: https://img.shields.io/github/v/release/xo/oracle?display_name=tag "Latest Release"
[releases]: https://github.com/xo/oracle/releases "Releases"
[discord]: https://discord.gg/WDWAgXwJqN "Discord Discussion"
[discord-status]: https://img.shields.io/discord/829150509658013727.svg?label=Discord&logo=Discord&colorB=7289da&style=flat-square "Discord Discussion"

# oracle

A pure Go Oracle Database driver for [`database/sql`](https://pkg.go.dev/database/sql).

`github.com/xo/oracle` is a fork of [`sijms/go-ora`](https://github.com/sijms/go-ora)
at its `v3` code, kept by the [`xo`](https://github.com/xo) project because
fixes to the upstream driver were not being accepted. It carries the `v3` code
only. It does not carry the `v1` or `v2` code, and it does not carry the
upstream tags. It is under rewrite, and [`docs/PLAN.md`](docs/PLAN.md) holds
the plan.

[`usql`][usql], [`dbtpl`][dbtpl] and [`dbmeta`][dbmeta] read Oracle through
this driver.

## Features

- **Pure Go** -- no C dependencies, no Oracle client installation required
- **`database/sql` compatible** -- works with any `database/sql` pool, retry logic, and health checking
- **Oracle 23ai types** -- VECTOR, BOOLEAN, JSON
- **Advanced Queuing** -- native AQ support (RAW, JSON, UDT, XML)
- **Fast Authentication** -- reduced round-trips with cookie-based caching and token login
- **TTC v24** -- latest protocol version with FSAP capability
- **User-Defined Types** -- nested objects, collections, and struct mapping
- **TLS/SSL** -- full support with wallet, mutual TLS, and Kerberos
- **Logging** -- the packets and events of each connection, through a `*slog.Logger`

## Installation

```bash
go get github.com/xo/oracle
```

Requires Go 1.27.1 or later.

## Quick Start

```go
package main

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/xo/oracle"
)

func main() {
    db, err := sql.Open("oracle", "oracle://user:pass@host/service")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    if err := db.Ping(); err != nil {
        log.Fatal(err)
    }

    fmt.Println("Connected to Oracle")
}
```

## Connection options

A DSN is a URL whose scheme is `oracle`:

```text
oracle://user:password@host:port/service/instance?key=value
```

The port is 1521 when the DSN names none. The path is the service name, and
an instance name after it. A service name that holds a `/` is written as
`%2F`. Every key is lower case. A key that the driver does not know is an
error, and so is a key that appears twice, other than `server`. The DSN takes
none of the key names of `go-ora`, such as `SSL VERIFY` or `SERVICE NAME`.

| Key | Holds | Default |
| --- | --- | --- |
| `sid` | A SID, in place of a service name | none |
| `descriptor` | A TNS descriptor, `(DESCRIPTION=...)`, in place of the host, the port and the path | none |
| `server` | One more address, `host:port`. Repeat it for more | none |
| `sslmode` | `disable`, `require`, `verify-ca` or `verify-full`, as in `pgx` | `disable` |
| `sslrootcert`, `sslcert`, `sslkey` | The PEM files of the authority, and of the certificate and key of the client | none |
| `sslservername` | The name to check the certificate against | the host |
| `wallet`, `wallet_password` | A folder with `cwallet.sso` or `ewallet.p12`, and its password | none |
| `timeout`, `connect_timeout` | Go durations, such as `30s` | none, and `60s` |
| `prefetch` | The rows that one fetch asks for. With `25`, the driver sizes each fetch by the size of its rows and their LOB values | `25` |
| `lob_fetch`, `lob_read` | `inline` or `stream`, and `auto` or `explicit` | `inline`, `auto` |
| `auth` | `password`, `os`, `kerberos` or `token` | `password` |
| `privilege` | `sysdba`, `sysoper`, `sysasm`, `sysbackup`, `sysdg`, `syskm` or `sysrac` | none |
| `token`, `token_key` | The files of a token and its private key | none |
| `encryption`, `checksum` | `accepted`, `rejected`, `requested` or `required` | the server decides |
| `language`, `territory`, `charset` | The NLS settings of the client | `AMERICAN`, `AMERICA` |
| `program`, `proxy_user` | The program that the session reports, and a proxy user | none |
| `unix_socket` | A Unix socket in place of TCP | none |
| `alias`, `tns_admin` | An alias of `tnsnames.ora`, in place of the host, the port and the path, and the folder of the file | none, and `$TNS_ADMIN` |

A user named `SYS` gets no privilege by its name. Write `privilege=sysdba`.

The server reads a user name in upper case unless the name is quoted, as in
SQL. A user created as `"scott"` logs on as `oracle://%22scott%22:pass@host/service`.

`ParseDSN` returns a `Config`. A caller that needs more than a DSN can say,
such as a `*tls.Config`, a `Dialer` or a `*slog.Logger`, changes the `Config`
and opens the database with a connector:

```go
cfg, err := oracle.ParseDSN(dsn)
if err != nil {
    return err
}
cfg.Dialer = myDialer
c, err := oracle.NewConnector(cfg, oracle.WithSessionParam("nls_date_format", "'YYYY-MM-DD'"))
if err != nil {
    return err
}
db := sql.OpenDB(c)
```

Each connector holds the state of its own database: the session parameters,
the registered types and the coders. Two databases in one process share none
of it.

## Values

A NULL scans as nil, and a nullable column scans into `sql.Null[T]`. The type
of a column decides the Go type that a scan into `*any` gets:

| Oracle | Go |
| --- | --- |
| `NUMBER` with a scale of 0 and a precision of 18 or less | `int64` |
| Any other `NUMBER`, such as `COUNT(*)` | `*apd.Decimal`, from `github.com/cockroachdb/apd/v3` |
| `BINARY_FLOAT`, `BINARY_DOUBLE` | `float64` |
| `INTERVAL YEAR TO MONTH` | `oracle.IntervalYM` |
| `INTERVAL DAY TO SECOND` | `time.Duration` |
| `DATE`, `TIMESTAMP` | `time.Time` in UTC |
| `TIMESTAMP WITH TIME ZONE` | `time.Time` in its zone |
| `TIMESTAMP WITH LOCAL TIME ZONE` | `time.Time` in the zone of the database |

A `DATE` and a `TIMESTAMP` hold no zone, so the driver returns the wall clock
of the value in UTC, whatever the zone of the server. To read one as the time
in a zone, such as `Europe/Prague`, call `time.Date` with the fields of the
value and that zone. A `time.Time` binds as its wall clock, and the server
reads it in the zone of the session. `oracle.TimestampTZ` binds with its zone.

A string or a `[]byte` longer than 32,767 bytes, or 4,000 on 11g, binds as a
`LONG` or a `LONG RAW`. SQL takes one only as a value to insert or set. Where
SQL uses the value in an expression, as in the `SELECT` of a `MERGE`, bind
`oracle.CreateClob` or `oracle.CreateBlob` in its place. In a PL/SQL block the
driver binds a value that long as a temporary LOB itself.

A decimal scans into a `*string`, a `*float64` or an `*int64` too. Both
interval types bind as arguments. `docs/DESIGN.md`, under Values, holds the
whole table.

### Reading a LOB in pieces

A CLOB scans into a `*string` and a BLOB into a `*[]byte`, which hold the
whole value. To read a large value in pieces, fetch it as a locator with
`lob_fetch=stream`, scan it into an `oracle.Clob` or an `oracle.Blob`, and
read it with `NewReader`:

```go
rows, err := db.QueryContext(ctx, `SELECT doc FROM files`, oracle.WithLobFetch(oracle.LobFetchStream))
// ...
for rows.Next() {
	var doc oracle.Clob
	if err := rows.Scan(&doc); err != nil {
		return err
	}
	if _, err := io.Copy(w, doc.NewReader(ctx)); err != nil {
		return err
	}
}
```

Each piece is one round trip of 1 MiB, and `NewReaderSize` sets another
size. A CLOB reads as UTF-8. Each read watches `ctx`. A LOB of a row reads
until its rows close, and a read after that fails with
`oracle.ErrRowsClosed`, because the connection of the rows can serve another
caller by then. An `oracle.BFile` reads the same way once it is open.

## New Types

### VECTOR (Oracle 23ai)

```go
// Create from Go slices
v1, _ := oracle.CreateVector([]uint8{10, 20, 30})          // INT8
v2, _ := oracle.CreateVector([]float32{-10.1, -20.2})       // FLOAT32
v3, _ := oracle.CreateVector([]float64{10.1, 20.2, 30.3})   // FLOAT64

// Scan from database
var vec oracle.Vector
err := row.Scan(&vec)

// Value returns the slice of its element type
v, err := vec.Value()
data := v.([]float32)
```

Formats: `INT8` (`[]uint8`), `FLOAT32`, `FLOAT64` -- dense and sparse.

### JSON (Oracle 21c+)

```go
// bind any Go value, through encoding/json/v2
js, err := oracle.NewJSON(Order{ID: 7, Items: []string{"a", "b"}})
_, err = db.ExecContext(ctx, "INSERT INTO orders (doc) VALUES (:1)", js)

// scan into any type that encoding/json/v2 decodes into
var order Order
err = db.QueryRowContext(ctx, "SELECT doc FROM orders").Scan(&order)

// or as the JSON text
var text jsontext.Value
err = db.QueryRowContext(ctx, "SELECT doc FROM orders").Scan(&text)
```

A JSON column scans into `*any` as a `jsontext.Value`. The driver turns JSON
text into OSON, the binary JSON of Oracle, and back, and `encoding/json/v2`
maps it to Go values, so a number keeps every digit.

### BOOLEAN (Oracle 23c+)

```go
input := oracle.Bool{}
input.SetValue(true)

db.Exec("BEGIN my_proc(:1, :2); END;", input, oracle.Out{Dest: &message})
```

## Advanced Queuing

```go
import "github.com/xo/oracle/aq"

// Create a queue
queue, err := aq.CreateQueue(db, "my_queue", aq.RAW, "")

// Enqueue
msg, _ := queue.NewMessage([]byte("hello"))
queue.Enqueue(msg)

// Dequeue
msg, err = queue.Dequeue(&aq.DequeueOptions{
    Consumer: "my_consumer",
    Mode:     aq.Browse,
    Wait:     5,
})
```

Message types: `RAW`, `JSON`, `UDT`, `XML`

Features: batch enqueue/dequeue, persistent and buffered delivery, visibility modes, navigation modes, message expiration, correlation filtering.

## User-Defined Types

```go
// Register with array counterpart
err := oracle.RegisterType(db, "MY_OBJECT", "MY_ARRAY", MyStruct{})

// Register with explicit owner
err = oracle.RegisterTypeWithOwner(db, "SCHEMA", "MY_OBJECT", "MY_ARRAY", MyStruct{})
```

Supports nested objects, collections (VARRAY, TABLE OF), and struct mapping via `udt` tags.
A type registers with the database of `db` alone. A value of the Go type that
`RegisterType` names binds as its Oracle type, and a field of a named type,
such as `type Status string`, binds as its kind.

A collection of a basic type registers with the name of its element type. Its
Go element type can be a named type, which binds with no more help:

```go
type Name string

err := oracle.RegisterType(db, "varchar2", "NAME_LIST", Name(""))
rows, err := db.Query("SELECT column_value FROM TABLE(:1)", []Name{"a", "b"})
```

With no Go type, `oracle.Object` names the collection at the bind. A
`[]time.Time` or a `[][]byte` binds as a PL/SQL array without it:

```go
err := oracle.RegisterType(db, "date", "DATE_LIST", nil)
var out []time.Time
_, err = db.Exec("BEGIN :1 := :2; END;",
    oracle.Out{Dest: oracle.Object{Name: "DATE_LIST", Value: &out}},
    oracle.Object{Name: "DATE_LIST", Value: []time.Time{time.Now()}})
```

A CLOB or a BLOB attribute of an object reads into an `oracle.Clob` or an
`oracle.Blob` field, which loads after the call.

## Session parameters

```go
c, err := oracle.NewConnector(cfg,
    oracle.WithSessionParam("cursor_sharing", "force"),
    oracle.WithSessionParam("nls_language", "arabic"))
```

The connector sets each parameter with `ALTER SESSION` on each connection
after its logon.

## Options of one statement

An option sets one statement, as an argument or through the context, and a
later one wins over the DSN:

```go
rows, err := db.QueryContext(ctx, query, arg1, oracle.WithPrefetch(1000))

ctx = oracle.WithOptions(ctx, oracle.WithTimeout(5*time.Second))
_, err = db.ExecContext(ctx, "BEGIN slow_proc; END;")
```

`WithTimeout` ends the call after the time, and leaves the connection
usable. `WithPrefetch` and `WithLobFetch` are the keys `prefetch` and
`lob_fetch` for one statement. `WithReadonly(true)`, `WithParameter` and
`WithDatabase` fail with `ErrNotSupported`, because Oracle has no such
setting for one statement.

## DBMS_OUTPUT

`DBMS_OUTPUT` belongs to one session, so the helpers take a `*sql.Conn` or a
`*sql.Tx`:

```go
conn, err := db.Conn(ctx)
if err != nil {
    return err
}
defer conn.Close()
if err := oracle.EnableOutput(ctx, conn, 0); err != nil {
    return err
}
if _, err := conn.ExecContext(ctx, "BEGIN my_proc; END;"); err != nil {
    return err
}
lines, err := oracle.ReadOutput(ctx, conn)
```

`ReadOutput` reads every line in one round trip, and removes them.

## Logging

The driver logs nothing unless the caller sets `Config.Logger`:

```go
cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
```

Each packet that a connection sends and receives goes to the logger at
`slog.LevelDebug` as a hex dump, with each statement, bind and event of the
connection. A break and a bad connection go at `slog.LevelInfo`. The packets
of the logon carry the encrypted password, so the logger gets only their
length until the logon ends.

## Architecture

```
oracle/
├── aq/                # Advanced Queuing
├── integration/       # Integration tests, which need a server
├── internal/
│   ├── auth/          # Encryption, checksums, Kerberos, NTS, NTLM
│   ├── charset/       # The Oracle charsets, compacted
│   ├── codec/         # Type encoding and decoding, and OSON
│   ├── config/        # The configuration of a connection, and wallets
│   ├── repotest/      # Tests of the repository setup
│   ├── tns/           # TNS packets and the session
│   ├── tnstest/       # Recorded exchanges for the tests
│   ├── ttc/           # Connections, statements, rows, LOBs, types
│   └── types/         # The values of each Oracle type
├── testdata/          # Exchanges recorded against each release
├── connector.go       # Connector, and the configuration of each connection
├── driver.go          # Driver registration, RegisterType
├── dsn.go             # Config and ParseDSN
├── errors.go          # Sentinel errors
└── values.go          # Out, RefCursor and the other types a caller uses
```

### Design Principles

- **One coder for each Oracle type** -- each type has its own `OracleParameterCoder`, and a type switch chooses the coder of a Go value
- **A connector for each database** -- the registered types, session parameters and fast login cookies of one database reach no other
- **Context on every call** -- a context that ends breaks the call with an Oracle Net break, and the connection stays usable
- **MemorySession** -- lightweight in-memory buffer for parameter encoding and AQ message marshaling
- **No driver-level failover** -- relies on `database/sql` connection pooling and retry logic

## Migration from go-ora

| go-ora v2 | oracle |
|----|-----|
| `github.com/sijms/go-ora/v2` | `github.com/xo/oracle` |
| Types in the driver package | Types in the driver package, such as `oracle.Clob` and `oracle.Vector` |
| `go_ora/dbms.NewAQ` | `aq.CreateQueue` |
| `go_ora/dbms.NewOutput` | `oracle.EnableOutput` and `oracle.ReadOutput` |
| Manual UDT setup | `oracle.RegisterType` with struct tags |
| `go_ora.AddSessionParam` / `DelSessionParam` | `oracle.WithSessionParam` on the connector |
| `go_ora.NewConnector(dsn)`, `Dialer`, `WithTLSConfig` | `oracle.NewConnector(cfg)` with `Config.Dialer` and `Config.TLS` |
| `go_ora.AddParameterCoder`, `SetStringConverter`, `RegisterConnConfig` | Removed |
| `go_ora.SetNTSAuth`, `SetKerberosAuth` | `Config.NTS` and `Config.Kerberos` on the connector |
| `go_ora.BuildUrl`, `BuildJDBC` | `Config.DSN` |
| The package `go_ora` | The package `oracle` |
| A `DATE` in the zone of the server | A `DATE` in UTC |

The DSN is not compatible. See Connection options.

## Related Projects

- [`usql`][usql] is a universal command line client for databases, and reads
  Oracle through this driver.
- [`dbtpl`][dbtpl] generates Go code from a database schema.
- [`dbmeta`][dbmeta] reads the metadata of a database, and its `dbrun` starts
  the Oracle releases that this driver is tested against.
- [`dburl`][dburl] parses database URLs, and builds the `oracle` URL of this
  driver.
- [`dbimp`][dbimp] holds `database/sql` drivers for other databases, whose
  design this driver follows.
- [`tblfmt`][tblfmt] renders result sets as tables.

[usql]: https://github.com/xo/usql "usql"
[dbtpl]: https://github.com/xo/dbtpl "dbtpl"
[dbmeta]: https://github.com/xo/dbmeta "dbmeta"
[dburl]: https://github.com/xo/dburl "dburl"
[dbimp]: https://github.com/xo/dbimp "dbimp"
[tblfmt]: https://github.com/xo/tblfmt "tblfmt"

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) before you change anything, and
[`AGENTS.md`](AGENTS.md) if you are a coding agent. Work that is known and not
done is in [`docs/BACKLOG.md`](docs/BACKLOG.md).

## License

MIT. The code descends from `sijms/go-ora`, and [`LICENSE`](LICENSE) holds its
notice.
