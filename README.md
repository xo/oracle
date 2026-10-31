# oracle

A pure Go Oracle Database driver for [`database/sql`](https://pkg.go.dev/database/sql).

`github.com/xo/oracle` is a fork of [`sijms/go-ora`](https://github.com/sijms/go-ora)
at its `v3` code, kept by the [`xo`](https://github.com/xo) project because
fixes to the upstream driver were not being accepted. It carries the `v3` code
only. It does not carry the `v1` or `v2` code, and it does not carry the
upstream tags. It is under rewrite, and [`docs/PLAN.md`](docs/PLAN.md) holds
the plan.

[usql](https://github.com/xo/usql), [dbtpl](https://github.com/xo/dbtpl) and
[dbmeta](https://github.com/xo/dbmeta) read Oracle through this driver.

The code is a major rewrite focused on extensibility, modern Oracle features, and clean architecture. It uses Go interfaces throughout to make adding new types, nested objects, and array types straightforward.

## Features

- **Pure Go** -- no C dependencies, no Oracle client installation required
- **`database/sql` compatible** -- works with any `database/sql` pool, retry logic, and health checking
- **Oracle 23ai types** -- VECTOR, BOOLEAN, JSON
- **Advanced Queuing** -- native AQ support (RAW, JSON, UDT, XML)
- **Fast Authentication** -- reduced round-trips with cookie-based caching and token login
- **TTC v24** -- latest protocol version with FSAP capability
- **User-Defined Types** -- nested objects, collections, and struct mapping
- **Extensible type system** -- plug in custom encoders/decoders for any Oracle type
- **TLS/SSL** -- full support with wallet, mutual TLS, and Kerberos

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
| `prefetch` | The rows that one fetch asks for | `25` |
| `lob_fetch`, `lob_read` | `inline` or `stream`, and `auto` or `explicit` | `inline`, `auto` |
| `auth` | `password`, `os`, `kerberos` or `token` | `password` |
| `privilege` | `sysdba`, `sysoper`, `sysasm`, `sysbackup`, `sysdg`, `syskm` or `sysrac` | none |
| `token`, `token_key` | The files of a token and its private key | none |
| `encryption`, `checksum` | `accepted`, `rejected`, `requested` or `required` | the server decides |
| `language`, `territory`, `charset` | The NLS settings of the client | `AMERICAN`, `AMERICA` |
| `program`, `proxy_user` | The program that the session reports, and a proxy user | none |
| `unix_socket` | A Unix socket in place of TCP | none |

A user named `SYS` gets no privilege by its name. Write `privilege=sysdba`.

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

## New Types

### VECTOR (Oracle 23ai)

```go
import "github.com/xo/oracle/types"

// Create from Go slices
v1, _ := types.CreateVector([]uint8{10, 20, 30})          // INT8
v2, _ := types.CreateVector([]float32{-10.1, -20.2})       // FLOAT32
v3, _ := types.CreateVector([]float64{10.1, 20.2, 30.3})   // FLOAT64

// Scan from database
var vec types.Vector
row.Scan(&vec)

// Copy to typed slices
var data []float32
vec.CopyTo(&data)
```

Formats: `INT8` (`[]uint8`), `FLOAT32`, `FLOAT64` -- dense and sparse.

### JSON (Oracle 21c+)

```go
var js types.Json
js.SetValue(`{"key": "value"}`)

var s string
js.CopyTo(&s)

var m map[string]interface{}
js.CopyTo(&m)
```

The driver encodes and decodes Oracle Binary JSON (OSON) itself.

### BOOLEAN (Oracle 23c+)

```go
input := types.Bool{}
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
    Mode:     aq.DequeueModeBrowse,
    Wait:     5,
})
```

Message types: `RAW`, `JSON`, `UDT`, `XML`

Features: batch enqueue/dequeue, persistent and buffered delivery, visibility modes, navigation modes, message expiration, correlation filtering.

## User-Defined Types

```go
// Register with array counterpart
oracle.RegisterType(db, "MY_OBJECT", "MY_ARRAY", MyStruct{})

// Register with explicit owner
oracle.RegisterTypeWithOwner(db, "SCHEMA", "MY_OBJECT", "MY_ARRAY", MyStruct{})
```

Supports nested objects, collections (VARRAY, TABLE OF), and struct mapping via `udt` tags.
A type registers with the database of `db` alone.

## Session parameters

```go
c, err := oracle.NewConnector(cfg,
    oracle.WithSessionParam("cursor_sharing", "force"),
    oracle.WithSessionParam("nls_language", "arabic"))
```

The connector sets each parameter with `ALTER SESSION` on each connection
after its logon.

## Architecture

```
oracle/
├── aq/                # Advanced Queuing
├── configurations/    # Connection string parsing
├── types/             # Oracle type implementations
├── internal/
│   ├── auth/          # Encryption, checksums, Kerberos, NTS, NTLM
│   ├── codec/         # Type encoding and decoding, and OSON
│   ├── converters/    # Charsets, NUMBER, dates
│   ├── tns/           # TNS packets and the session
│   └── tnstest/       # Recorded exchanges for the tests
├── testdata/exchanges # Exchanges recorded against each release
├── connection.go      # Connection implementation
├── driver.go          # Driver registration
├── command.go         # Statement execution
├── parameter.go       # Parameter handling
├── lob.go             # LOB streaming
├── udt.go             # User-Defined Types
├── transaction.go     # Transactions
└── bulk_copy.go       # Bulk copy
```

### Design Principles

- **Interface-based type system** -- each Oracle type has its own `OracleParameterCoder` implementation, making it trivial to add new types
- **Three-way type mapping** -- Oracle type ID ↔ Go `reflect.Type` ↔ SQL type name
- **Pluggable coders** -- register custom encoders/decoders without modifying driver internals
- **MemorySession** -- lightweight in-memory buffer for parameter encoding and AQ message marshaling
- **No driver-level failover** -- relies on `database/sql` connection pooling and retry logic

## Migration from go-ora

| go-ora v2 | oracle |
|----|-----|
| `github.com/sijms/go-ora/v2` | `github.com/xo/oracle` |
| Types in driver package | Types in `github.com/xo/oracle/types` |
| `go_ora/dbms.NewAQ` | `aq.CreateQueue` |
| Manual UDT setup | `oracle.RegisterType` with struct tags |
| `go_ora.AddSessionParam` / `DelSessionParam` | `oracle.WithSessionParam` on the connector |
| `go_ora.NewConnector(dsn)`, `Dialer`, `WithTLSConfig` | `oracle.NewConnector(cfg)` with `Config.Dialer` and `Config.TLS` |
| `go_ora.AddParameterCoder`, `SetStringConverter`, `RegisterConnConfig` | Removed |
| `go_ora.BuildUrl`, `BuildJDBC` | `Config.DSN` |
| The package `go_ora` | The package `oracle` |

The DSN is not compatible. See Connection options.

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) before you change anything, and
[`AGENTS.md`](AGENTS.md) if you are a coding agent. Work that is known and not
done is in [`docs/BACKLOG.md`](docs/BACKLOG.md).

## License

MIT. The code descends from `sijms/go-ora`, and [`LICENSE`](LICENSE) holds its
notice.
