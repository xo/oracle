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

Step 9 is staged. It fixes sijms/go-ora#776, #703 for NCHAR, #699 for every
basic kind and #762, finds #671 does not break the connection here, and
`upstream_test.go` holds each against a live server. `TestIssue543` makes its
own index, and `integration` on 26ai gives 43 passes and 2 failures. On 11g the root package passes, and `integration` gives 40 passes and 3 failures, which `BACKLOG.md` lists. Left in
`UPSTREAM.md`: #769, #771, #692, #624, #695, #698 and #729, and the pull
requests, which wait for Ken.
