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

Step 5 is done and staged. `context.AfterFunc` sends the break when a context
ends, and no goroutine starts for each call. A transaction keeps no context,
which fixes sijms/go-ora#767. The library calls neither
`context.Background()` nor `context.TODO()`. `context_test.go` holds five
integration tests, and they pass on 11g, 21c and 26ai with `-race`. The 30
exchanges pass, and `TestIssues` on 26ai gives the same 42 passes and 3
failures as before, with no data race.

The next step is step 6: change the values to the table in `DESIGN.md`, and
add `RowsColumnScanner`.
