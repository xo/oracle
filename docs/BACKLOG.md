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

### Build the statement options

`DESIGN.md`, under Options, holds the options of a statement as dbimp D109
sets them out: `WithOptions`, `WithTimeout`, `WithReadonly`, `WithPrefetch`
and `WithLobFetch`. None is built. The DSN keys `prefetch` and `lob_fetch` set
the same for every statement of a connector.

### Report SELECT CAST(:1 AS JSON) on 21c

On 21c, `SELECT CAST(:1 AS JSON) FROM dual` with a JSON bind fails inside the
server with ORA-00600 [25026] or [kcbgtcr_1]. The same bind inserts into a JSON
column, `JSON_SERIALIZE(CAST(:1 AS JSON))` works, and 26ai returns it. Find
whether the driver can send the bind so that 21c accepts it, as python-oracledb
does, or record it as a fault of 21c.

### Write the integration tests again

Step 1 deleted the files of `TestIssues` that did not build against the `v3`
code. They used the API of `v2`, such as `go_ora.Clob`, `go_ora.Object` and
`go_ora.NVarChar`, or a package that the module does not carry: `gorm`, `xorm`
and the `dbms` package of `v2`. Each one is in commit `d9fa927` and earlier.
Write a test for each again, on the new API. Step 9 wrote `TestStrings`,
`TestBlob`, `TestBoolean` and `TestVector` in `values_test.go`, for
`bool_type_test.go`, `emoji_test.go`, `long_input_test.go`, `lob_test.go` and
`vector_test.go`. These are left:

- `regular_type_array_test.go`, `udt_string_test.go` and `varray_test.go`.
- `bulkinsert_bfile_test.go`, `bulkinsert_blob_test.go` and
  `inout_lob_test.go`.
- `issue_173_test.go`, `issue_264_test.go`, `issue_320_test.go` and
  `issue_363_test.go`.
- `dbms_aq_test.go`, which the `aq` package replaces. `TestOutput` in
  `values_test.go` replaces `dbms_output_test.go`.
- `gorm_test.go`, which tests `gorm` and not this driver. It does not come back.

Step 1 also deleted the unit tests of functions that `v3` removed:
`parameter_encode_test.go`, `bulkcopy_test.go`, the tests of `setString`,
`setFieldValue` and `setNull`, and `TestCreateNewType` with the dead function
that it tested. The vectors of `converters/other_types_test.go` moved to
`internal/types/vectors_test.go`.

### Take four upstream features

Build the requests of D21, each with a test. Step 9 built the key
`skip_nls` (sijms/go-ora#611). The query that it skips reads settings that
nothing uses, so delete it outright when the exchanges of every release can
be recorded again, 19c among them. Left: a logon with an OCI IAM token (#530),
streaming of LOBs and rows (#681, #559 and #664), and failover to the next
address of a descriptor (#707 and #619).

### Reproduce the refusal of required network encryption

A server with `SQLNET.ENCRYPTION_SERVER=REQUIRED` or
`SQLNET.CRYPTO_CHECKSUM_SERVER=REQUIRED` refuses the driver with ORA-12269
(sijms/go-ora#729). Reproduce it on a `dbrun` release, and fix the
negotiation (D21).

### Write four upstream pull requests again

Ken decided to take sijms/go-ora#734, #721, #770 and #773 (D18). Step 9 wrote
#770, which reads XMLType, #773, which replaces a character that a charset
lacks, and #721, which encodes a collection in an object. #734 is not
applied, because its fault does not reproduce, as `UPSTREAM.md` says. Left
of #721: an array bind of objects with a NULL row, which needs a test. `UPSTREAM.md` says what each does. On 2026-10-31
`dbrun` added the Autonomous Database Free image as `oracle-adb-26ai` and
`oracle-adb-19c`, which CI leaves out. The driver connects to both with
`sslmode=require` in place of the query that `dbrun dsn` prints, which names
the options of `go-ora`. The `v3` code reads no XMLType column, so #770 starts
with a reader of XMLType, tested on those releases and on the others.

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
