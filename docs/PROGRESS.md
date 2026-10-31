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

Step 2 is done and staged. `internal/tnstest` records the TNS packets of a
connection and plays them back over a `net.Pipe`. `exchange_test.go` holds
five scenarios, `ping`, `query`, `error`, `dml` and `clob`, and
`testdata/exchanges/` holds them recorded against every release that `dbrun`
starts: 11g, 18c, 19c, 21c, 23ai and 26ai. `TestExchanges` plays them all back
with no server.

The next step is step 3: move the protocol code to `internal/`, with no change
of behavior. The exchanges prove that nothing changed.
