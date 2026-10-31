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

Step 1 of the design is done and staged. `go.mod` requires Go 1.27.1, `go fix`
ran over both modules, and `go vet ./...` and `go test -race -count=2 ./...`
pass with no server, on Linux, and vet passes for Windows and macOS. The
tests that did not build are deleted or moved, and `BACKLOG.md` lists them.
Two faults came out of it and are fixed: `types.defaultCopy` panicked or
failed when a value needed a conversion, and `result_set_test.go` lacked two
braces. One fault is recorded and not fixed: the decoding of intervals.
Nothing ran against a server yet.

The next step is step 2: record the exchanges of each release for the fake
server.
