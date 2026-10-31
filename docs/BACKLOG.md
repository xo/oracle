# Backlog

This document lists work that is known and not done. Each item names the
decision or the fact that found it.

A decision is not a backlog item. It goes in [`PLAN.md`](PLAN.md). When an
item here is done, delete it, and record in `PLAN.md` anything that was
decided on the way (D3).

## Repository

### Settle the package name

The package is `go_ora`, which is not an idiomatic name. See the open
question in `PLAN.md`.

### Make the tests build

`go vet ./...` fails on tests that came with the code and no longer match it.
`TestIssues` imports `github.com/godoes/gorm-oracle`, which the module does not
require. `converters/other_types_test.go` calls `ConvertBinaryFloat`, which no
longer exists. The root package tests use `NCHAR`, `Number` and `BulkCopy`,
which that package does not define. `result_set_test.go` had a missing brace,
which is fixed. A test that needs a database must skip without one
(AGENTS.md rule 5).

### Wire the integration tests to dbrun

`TestIssues` and `module_test` read their connection string from variables
that upstream set by hand. Make them read the URL that `dbrun dsn oracle`
prints, and skip without one (D6). Check that `dbmeta` has Oracle 11g, 12c and
18c releases for `dbrun`, because `dbrun list oracle` names only `oracle-26ai`.

### Make 18c and 19c start in CI

`dbmeta` marks 18c and 19c as verified because their images are built by hand
from an Oracle installer (D7). The oracle job in the workflow lists them, and
they fail on a runner until `dbmeta` can start them there. Decide with Ken
whether to publish the images or to let those two jobs wait.

### Shrink the charset tables

An earlier analysis of the upstream `v3` code found that 88 percent of it, 11.3
of 12.7 MB, was one generated file, `converters/string_conversion_new.go`. It is
a `switch` over 236 Oracle charset IDs that returns a converter with inline
`[]int` and `map[int]int` literals. A minimal program that imports the driver
built to 23.6 MB, or 20.2 MB stripped.

That analysis chose a lossless compaction and not `golang.org/x/text`, because
Oracle's variants of ZHS16GBK, JA16SJIS, EBCDIC and the shift-in and shift-out
charsets can differ from the standard mappings. It serialised each table as
varints, deflated one stream per charset into one `go:embed` blob of about 2.0
MB, and decoded a charset on first use. It checked that all 236 were
byte-identical to the old tables. The binary fell to 13.0 MB, or 9.5 MB
stripped. That work was never committed and this repository does not carry it.
Redo it here, and re-check losslessness after every format change. Time the
first use of a charset, which was not measured. Ways to shrink the blob further
are delta coding the values, deriving `eBuffer` as the inverse of `dBuffer`
where they agree, and one solid compressed stream.

`buildTypeNego` in `data_type_nego.go` is a 144 KB symbol. It is probably a
large literal table that can be data.

### Fix Clone dropping the leading table

`StringConverter.Clone()` does not copy the `leading` table, so a clone of an
EUC style charset, such as 0x35C and 0x35F, decodes wrongly. Add a test.

### Set the minimum Go version

The owner of the earlier analysis wanted Go 1.26 as the minimum and no new
dependency. `go.mod` says `go 1.24`. Ask Ken, then raise it and run `go fix`,
which turns `interface{}` into `any` and uses `reflect.TypeFor` and ranges over
integers. `go fix` refuses a package whose tests do not build, so fix those
first (see Make the tests build).

### Fix the root level leftovers

The earlier analysis also found `go test ./types/` panics in `TestCreateNewType`
(`reflect.Set: value of type *types.Blob is not assignable to types.Blob`), and
that most other tests need a live database. Only `./types/oson/...` passes alone.
`data_type_nego.go.old` was a v1 leftover and is already deleted here.

### Tell dburl when the URL changes

dburl staged its `oracle` scheme as provisional on 2026-10-31 (its D69), with
`github.com/xo/oracle` as the package. Its `GenOracle` writes
`oracle://user:pass@host:1521/service?<query>`. It adds the host `localhost`
and the port 1521, and it passes the query through untouched. It does not
change `tls` to `SSL`. It was checked against `ParseConfig` in this tree. Tell
the dburl session when any of these changes: a host with no port, a `tls` key,
the package name, or the first tag. dburl checks again before it tags.

### Fix the upstream issues

[`UPSTREAM.md`](UPSTREAM.md) holds the triage of every open issue and pull
request of `sijms/go-ora`, and the work that follows from it.
