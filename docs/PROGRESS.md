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

Step 4 is done and staged. The package is `oracle`. `ParseDSN`, `Config` and
`Config.DSN` read and write the DSN of D11. `Driver` holds nothing, and each
`Connector` holds the state of one database. The package variable and the
functions that kept state through a `*sql.DB` are gone, except `RegisterType`,
which now reaches its own database alone. `configurations` moved to
`internal/configurations`. `Config` still hands its values to the parser of
`go-ora`, so the 30 exchanges pass with no new recording.

The next step is step 4b: move the TTC code to `internal/ttc`, unexport the
types that `go-ora` exported, and write the configuration directly.
