# Progress

This file records where the work stands, so that a session that ends can
resume. Update it when a piece of work starts or ends. Work that is known and
not done goes in [`BACKLOG.md`](BACKLOG.md), and a decision goes in
[`PLAN.md`](PLAN.md).

## Where the work stands

2026-10-31. The repository holds the `v3` code of `sijms/go-ora` at its root,
on the `main` branch, with the module path `github.com/xo/oracle` (`cb68b12`).
[`DESIGN.md`](DESIGN.md) holds the design of the rewrite, and D8 to D16 in
[`PLAN.md`](PLAN.md) record it (`d9fa927`).

Step 1 of the design is done (`7bc994c`). `go.mod` requires Go 1.27.1, `go
fix` ran over both modules, and every package vets and passes its unit tests
with no server.

Step 2 is done (`5e1e3a8`). `internal/tnstest` records the TNS packets of a
connection and plays them back over a `net.Pipe`. `exchange_test.go` holds
five scenarios, recorded under `testdata/exchanges/` against 11g, 18c, 19c,
21c, 23ai and 26ai. `TestExchanges` plays all 30 back with no server.

Step 3 is done (`eb5e173`). Every package that a caller does not use moved
under `internal/`, and the 30 exchanges passed after the move.

Step 4 is done (`c825f8d`). The package is `oracle`, with `ParseDSN`,
`Config`, `Driver`, `Connector` and the sentinel errors, and no package
variable.

Step 4b is done (`cbc4d30`). The TTC code is in `internal/ttc`, and the root
package exports 31 names.

Step 5 is done (`3982ceb`). `context.AfterFunc` breaks a call when its context
ends, and a transaction keeps no context, which fixed sijms/go-ora#767.

Step 6 is done (`c95385a`). A `NUMBER` is an `int64` or an `*apd.Decimal` by
its column type, the intervals decode right, and `DataSet` implements
`RowsColumnScanner`.

Ken asked on 2026-10-31 to use `sql.Null[T]` and drop the driver's own nullable
types. That is done and staged. `NullTimeStampTZ` is gone, `TimeStampTZ` binds
as `TIMESTAMP WITH TIME ZONE` for the first time, and the per-type cases of the
`sql.NullX` types are gone. On the way, an OUT string of size 10 could not
hold 'abc', which also made the logon NLS query fail on 11g. That is fixed,
which was sijms/go-ora#703. The tests pass on 11g and 26ai, and `TestIssues`
on 26ai gives the same 42 passes and 3 failures.

The `sql.Null` work is done (`ff69666`). The cleanup of step 11 is done in
`d993e1c` and `2ca0f62`, and its last part is staged: no method starts with
`Get`, and each interface that stays has more than one implementation or
breaks an import cycle. `DESIGN.md`, under Where the cleanup stands, lists
what is left. The tests pass, and `integration` on 26ai gives the same 42
passes and 3 failures.

Step 11 is done (`ba43353`), and step 7, JSON on `encoding/json/v2`, is done
(`83f010d`).

Step 8 is done and staged. The charsets are a compressed blob of 2.0 MB in
`internal/charset`, equal to the old tables byte for byte, and a program that
imports the driver is 12.5 MB, 9.2 MB stripped, where it was 22.6 MB and 19.3
MB. The integration tests on 26ai give the same 42 passes and 3 failures.

Step 8 is done (`073dd07`).

Step 9 is in progress. Its first part is done (`bf54b03`). It fixes
sijms/go-ora#776, #703 for NCHAR, #699 for every basic kind and #762, finds
that #671 does not break the connection here, and `upstream_test.go` holds
each against a live server. `TestIssue543` makes its own index.

The second part of step 9 is done (`399f398`). It fixes #698, a batch that
failed and hung on 11g, #695, #624 and #692, whose cookies are now per
`Connector`.

The third part of step 9 is done (`cda6693`). It fixes #769, sends the trace
to `Config.Logger` (D14), and sweeps the dead code and the documents.

The fourth part of step 9 is done (`9d4ea66`). `ColumnTypeDatabaseTypeName`
gives the names of the data dictionary, a `time.Time` with a fraction of a
second binds as a `TIMESTAMP`, and `Config.NTS` replaces `SetNTSAuth`.

The fifth part of step 9 is done (`91a5448`, pushed). It reads XMLType,
which fixes #771, binds long text safely on 11g, and records D8 to D21. CI found the same
fault of long text on 18c, and the sixth part covers every release before
19c.

The sixth part of step 9 is done (`a06284d`, pushed). It replaces a missing
character with '?' (#773), reads `DBMS_OUTPUT` (D19), and covers 18c.

The seventh part of step 9 is done (`42b556e`, pushed). It encodes
collections in objects (#721), reads `tnsnames.ora` (D20), and adds
`skip_nls` (#611).

The eighth part of step 9 is done (`c8a1f39`, pushed). A NULL object of a
batch is NULL (#721), and a failed handshake tries the next address (#619,
#707).

The ninth part of step 9 is done (`3ef13ad`, pushed). It builds the options
of one statement of dbimp D109.

The tenth part of step 9 is done (`284844c`, pushed). A fetch of rows with
LOB values holds the values of about 4 MiB (#664), and AES takes a zero IV
on a server before 12c (#729).

The eleventh part of step 9 is done (`63282cc`, pushed). `Clob`, `Blob` and `BFile` have
`NewReader(ctx)`, which reads a LOB in pieces of 1 MiB (#681 and #559, D22).
`ScanColumn` gives the locator to an `oracle.Clob` or an `oracle.Blob`, and
loads the value for any other destination. A read after the rows close
fails with `ErrRowsClosed`. With this, D21 is done.

The twelfth part of step 9 is done (`dfba5e0`, pushed). A refused logon on 23ai and later gives
ORA-01017, where the driver read the reply to the protocol negotiation twice
and gave "message code error". `Query` of a statement that is not a query
gives no rows, where `Next` failed with ORA-01002. A `DATE` is in UTC (#672),
and the zone of the database reads right when it is a region (#655). The logon
runs no query, so `skip_nls` is gone (#611), and the exchanges of 11g, 18c,
21c, 23ai and 26ai are recorded again. Ken decided that no test runs on 19c
(D23), so its exchanges are deleted. #683 is not a fault: a lower case user name logs
on quoted.

The thirteenth part of step 9 is done (`c037ddd`, pushed). In a PL/SQL block a string or a
`[]byte` binds as a `VARCHAR2` or a `RAW` up to 32,767 bytes on every release,
and as a temporary LOB past that (D24), where the driver bound a `LONG` over
4,000 bytes on 11g and 32,767 on the rest, and PL/SQL refused it with
ORA-01483. `UPSTREAM.md` records #635, #688, #680 and #697, which did not
reproduce.

The fourteenth part of step 9 is done (`2db0d89`, pushed). The integration tests that step 1
deleted are written again on the new API, in `collections_test.go`,
`lobs_test.go`, `objects_test.go` and `named_types_test.go`. `gorm_test.go`
and `issue_363_test.go` test `gorm` and `xorm`, and do not come back. The
tests found five faults, which are fixed:

- An object with a CLOB or a BLOB attribute panicked on a read.
- An OUT collection of CLOBs or BLOBs gave the locators of its elements.
- A named type of a basic kind, `time.Time` or `[]byte` inside an object or a
  collection failed to bind, and `RegisterType` refused a named `[]byte`.
- A PL/SQL IN OUT string longer than 32,767 bytes failed. It binds as a
  CLOB now, under D24.
- With `lob_fetch=stream`, a NULL CLOB or BLOB scanned into a string or a
  `[]byte` failed, which the eleventh part brought in.

`Object` names a type at a bind (D25).

The fifteenth part of step 9 is done (`922cd35`, pushed). A CLOB or a BLOB attribute of an
object reads into a `string` or a `[]byte` field, in an OUT value, a
collection, a row and an object inside another, and an object with one binds,
where the server refused it with ORA-03101. A nil `*oracle.Clob` or
`*oracle.Blob` binds as NULL, where it panicked.

The sixteenth part of step 9 is done (`922cd35`, pushed). It finishes the open issues of
go-ora (D27) and starts the tests of python-oracledb (D28):

- A prepared query whose column changed type under it failed in 14 of 18
  cases. It reads the type that the cursor fetched, and parses again on
  ORA-00932 (`TestTypeChanges`).
- A RETURNING that ran again with other binds failed with ORA-03146, which
  was sijms/go-ora#697. An OUT of RETURNING too small for its value got a cut
  value, an UPDATE of no row failed, and `RETURNING(id)INTO` was not found
  (`TestRebind`, `TestReturning`).
- An address of a descriptor with a key other than HOST, PORT and PROTOCOL
  was dropped, and `tnsnames.ora` took no Easy Connect entry, kept the first
  of two entries, and read text that is no entry as an alias.
- A logon that the server closed returns an error that says so, which was
  sijms/go-ora#700. `Client.Terminal` names the terminal of the session
  (sijms/go-ora#584).
- `DRIVERS.md` compares the features of the Oracle drivers, and D26 records
  that two-phase commit and sessionless transactions are not built.

The seventeenth part of step 9 is staged. It writes more of the tests of
python-oracledb (D28), and they found these faults, which are fixed:

- A batch went on past a row that failed, and stored the rows after it,
  because the driver asked for the errors of a batch.
- A batch kept the type of a row for the next, so a decimal after an `int`
  went as text labeled NUMBER, and the server stored NULL. A mix of types
  that cannot share one is an error.
- A decimal bound as text, which the server read with the decimal point of
  the session. It binds as a NUMBER now.
- A NUMBER with an exponent in its text, such as `9.9E+125`, bound as another
  number. A value past the range of NUMBER, an infinity and a NaN bound with
  no error, and 1e-130 read as 0.
- An NCLOB of stream mode read in pieces skipped half of each piece.

