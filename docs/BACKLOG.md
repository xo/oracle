# Backlog

This document lists work that is known and not done. Each item names the
decision or the fact that found it.

A decision is not a backlog item. It goes in [`PLAN.md`](PLAN.md). When an
item here is done, delete it, and record in `PLAN.md` anything that was
decided on the way (D3).

## Repository

### Reshape the TTC code

Step 4b moved the TTC code to `internal/ttc` as it was, and steps 5 and 6
reshaped it in place. Its names are still the ones of `go-ora`: `Connection`,
`Stmt`, `DataSet`, `ParameterInfo`, `Transaction`. `DESIGN.md`, under The root
package, holds the target, with `conn`, `stmt`, `rows` and `tx`. The Oracle
type IDs are constants of `uint16`, and want a named type. `Config` has no keys for the OS user and password of NTS, the domain, the
list of authentication services, out of band breaks or the location of the
server, which `go-ora` read. Add each when a caller needs it.

### Remove the state that the whole process shares

Step 4 removed the package variable `oracleDriver` (D9). Step 9 moved the fast
login cookies to the `Connector`, and deleted the Kerberos authenticator of
the process, since `Config.Kerberos` sets one for each connector. One piece of
state for the whole process remains: `auth.NTSAuth`, which `SetNTSAuth` sets,
and which a configuration with an OS password sets for every connection after
it. Move it to the `Connector`.

### Build the statement options

`DESIGN.md`, under Options, holds the options of a statement as dbimp D109
sets them out: `WithOptions`, `WithTimeout`, `WithReadonly`, `WithPrefetch`
and `WithLobFetch`. None is built. The DSN keys `prefetch` and `lob_fetch` set
the same for every statement of a connector.

### Find the statement that answers ORA-01403 at logon

Every release answers a statement at logon with `ORA-01403: no data found`,
which nothing checks. The recorded exchanges show it. Find which statement it
is, and return the error or stop running the statement (D14).
sijms/go-ora#611 asks for an option to skip the NLS query.

### Report SELECT CAST(:1 AS JSON) on 21c

On 21c, `SELECT CAST(:1 AS JSON) FROM dual` with a JSON bind fails inside the
server with ORA-00600 [25026] or [kcbgtcr_1]. The same bind inserts into a JSON
column, `JSON_SERIALIZE(CAST(:1 AS JSON))` works, and 26ai returns it. Find
whether the driver can send the bind so that 21c accepts it, as python-oracledb
does, or record it as a fault of 21c.

### Finish the values of dates

`TestTime` in `integration` fails on 26ai: a timestamp comes back in UTC and
without its fraction of a second. The table in `DESIGN.md`, under Values,
says what each date type returns. Fix it with a test in `values_test.go`.

### Write the integration tests again

Step 1 deleted the files of `TestIssues` that did not build against the `v3`
code. They used the API of `v2`, such as `go_ora.Clob`, `go_ora.Object` and
`go_ora.NVarChar`, or a package that the module does not carry: `gorm`, `xorm`
and the `dbms` package of `v2`. Each one is in commit `d9fa927` and earlier.
Write a test for each again, on the new API:

- `bool_type_test.go`, `emoji_test.go`, `long_input_test.go`,
  `regular_type_array_test.go`, `udt_string_test.go`, `varray_test.go` and
  `vector_test.go`.
- `bulkinsert_bfile_test.go`, `bulkinsert_blob_test.go`, `inout_lob_test.go`
  and `lob_test.go`.
- `issue_173_test.go`, `issue_264_test.go`, `issue_320_test.go` and
  `issue_363_test.go`.
- `dbms_aq_test.go` and `dbms_output_test.go`. The `aq` package replaces the
  first. Nothing in `v3` replaces `DBMS_OUTPUT`, which `v2` read through
  `dbms.NewOutput`. Ask Ken whether the driver reads it.
- `gorm_test.go`, which tests `gorm` and not this driver. It does not come back.

Step 1 also deleted the unit tests of functions that `v3` removed:
`parameter_encode_test.go`, `bulkcopy_test.go`, the tests of `setString`,
`setFieldValue` and `setNull`, and `TestCreateNewType` with the dead function
that it tested. The vectors of `converters/other_types_test.go` moved to
`internal/types/vectors_test.go`.

### Make the integration tests pass on every release

`integration`, which was `TestIssues`, reads `ORACLE_DSN` and skips without it, and CI sets it from
`dbrun dsn --json`. On 2026-10-31 it ran against 26ai alone: 42 tests passed
and 3 failed, the same 3 at `eb5e173` before step 4 and after it. In step 9
`TestIssue543` makes its own index, where it read one of the APEX schema that
the free images do not have, so 43 pass and 2 fail.

- `TestColumnType`: `ColumnTypeDatabaseTypeName` returns "" for an `NCHAR`
  column. sijms/go-ora#760 rewrites the column metadata.
- `TestTime`: a timestamp comes back in UTC and without its fraction of a
  second. See Finish the values of dates.

On 11g in step 9, 40 pass and 3 fail. `TestTime` fails as on 26ai.
`TestIssue186` uses `FETCH NEXT`, and `TestIssue335` an identifier longer than
30 characters, which 11g does not have. Those two tests need SQL that 11g
accepts, or a skip on 11g. A second run on the same 11g database also fails
`TestArray`, because the package body `GOORA_TEMP_PKG` does not compile, and
`TestColumnType`, as on 26ai. A test leaves an object behind that the next run
meets. Each test must drop what it makes.

Run it on the other four releases, and fix or record each failure.

### Make 19c start in CI

`dbmeta` marks 18c and 19c as verified, and not tested in its CI (D7). On this
machine on 2026-10-31, 18c started in less than a minute from its public image.
19c needs the image that `dbrun build` makes from an Oracle installer that is
not public, and its first start creates the database, which took more than the
5 minutes that `dbrun start` waits. `dbrun start --timeout 25m oracle-19c`
started it. The oracle job in the workflow lists 19c and fails on a runner
until `dbmeta` can start it there. Decide with Ken whether to publish the image
or to leave 19c out of CI.

### Tell dburl when the first tag exists

dburl staged its `oracle` scheme as provisional on 2026-10-31 (its D69), with
`github.com/xo/oracle` as the package. Its `GenOracle` writes
`oracle://user:pass@host:1521/service?<query>`. It adds the host `localhost`
and the port 1521, and it passes the path and the query through as they are,
so `%2F` in a service name and every `ssl*` key reach the parser unchanged.
dburl maps no `tls` key to `sslmode` for any scheme, and will not. The parser
of D11 landed in step 4. When the first tag exists, tell the dburl session,
which then rechecks its tests and documents.

### Fix the upstream issues

[`UPSTREAM.md`](UPSTREAM.md) holds the triage of every open issue and pull
request of `sijms/go-ora`, and the work that follows from it.
