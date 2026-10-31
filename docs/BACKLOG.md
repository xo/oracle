# Backlog

This document lists work that is known and not done. Each item names the
decision or the fact that found it.

A decision is not a backlog item. It goes in [`PLAN.md`](PLAN.md). When an
item here is done, delete it, and record in `PLAN.md` anything that was
decided on the way (D3).

## Repository

### Rename the package

D8 settled the name as `oracle`. The package is still `go_ora`, and step 4 of
`DESIGN.md` renames it.

### Fix the decoding of intervals

`Interval.Value` decodes no interval correctly. It returns a `time.Time`, so a
year to month interval of `+2021-10` comes back as 2021-09-30, and a negative
one comes back as an unrelated date. It multiplies the fractional seconds of a
day to second interval by 1000. The vectors from Oracle are in
`types/vectors_test.go`, in two tests that skip. Step 6 of `DESIGN.md` replaces
the type with `IntervalYM` and `time.Duration`. Remove the skips then.

### Size the OUT parameters again

`ParameterInfo.encodeValue` in `parameter_encode.go` ended in a block after its
`return nil`, so it never ran. The block set `MaxCharLen` and `MaxLen` of an
`NCHAR` or `RAW` OUT parameter from its size, and cleared `BValue` of every
OUT parameter. Step 1 deleted it as dead code, which changed nothing. An OUT
string with no size breaking the connection, sijms/go-ora#703, can be the
fault that this block was written for. Look at it with that issue.

### Read the NLS settings on 11g

The recorded exchanges show that `GetNLS`, which runs at every logon, fails on
11g with `ORA-06502: PL/SQL: numeric or value error: character string buffer
too small`. `OpenWithContext` traces the error and goes on, so a connection to
11g has no NLS settings, and the server sends two marker packets that the
driver answers. Every release also answers a statement at logon with
`ORA-01403: no data found`, which nothing checks. Find which statement it is.
A design that logs nothing (D14) has to return such an error or not run the
statement. sijms/go-ora#611 asks for an option to skip the NLS query.

### Write the integration tests again

Step 1 deleted the files of `TestIssues` that did not build against the `v3`
code. They used the API of `v2`, such as `go_ora.Clob`, `go_ora.Object` and
`go_ora.NVarChar`, or a package that the module does not carry: `gorm`, `xorm`
and the `dbms` package of `v2`. Each one is in commit `d9fa927` and earlier.
Step 9 of `DESIGN.md` writes a test for each again, on the new API:

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
that it tested. The vectors of `converters/other_types_test.go`, now under `internal/`, moved to
`types/vectors_test.go`.

### Make 11g to 19c pass

`TestIssues` reads `ORACLE_DSN` and skips without it, and CI sets it from
`dbrun dsn --json`. Nothing ran it against a server yet. Run it on every
release, and fix or record each failure.

### Make 19c start in CI

`dbmeta` marks 18c and 19c as verified, and not tested in its CI (D7). On this
machine on 2026-10-31, 18c started in less than a minute from its public image.
19c needs the image that `dbrun build` makes from an Oracle installer that is
not public, and its first start creates the database, which took more than the
5 minutes that `dbrun start` waits. `dbrun start --timeout 25m oracle-19c`
started it. The oracle job in the workflow lists 19c and fails on a runner
until `dbmeta` can start it there. Decide with Ken whether to publish the image
or to leave 19c out of CI.

### Shrink the charset tables

An earlier analysis of the upstream `v3` code found that 88 percent of it, 11.3
of 12.7 MB, was one generated file, `converters/string_conversion_new.go`, now
`internal/converters/string_conversion_new.go`. It is
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

### Fix the root level leftovers

Only `./internal/codec/oson/...` and a few other unit tests run with no server. Most
tests need a live database.

### Tell dburl when the parser lands

dburl staged its `oracle` scheme as provisional on 2026-10-31 (its D69), with
`github.com/xo/oracle` as the package. Its `GenOracle` writes
`oracle://user:pass@host:1521/service?<query>`. It adds the host `localhost`
and the port 1521, and it passes the path and the query through as they are,
so `%2F` in a service name and every `ssl*` key reach the parser unchanged.
dburl maps no `tls` key to `sslmode` for any scheme, and will not. The parser
of D11 is not written yet. When it lands, and again when the first tag
exists, tell the dburl session, which then rechecks its tests and documents.

### Fix the upstream issues

[`UPSTREAM.md`](UPSTREAM.md) holds the triage of every open issue and pull
request of `sijms/go-ora`, and the work that follows from it.
