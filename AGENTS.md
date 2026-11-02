# oracle

`github.com/xo/oracle` is a pure Go Oracle Database driver for `database/sql`.
It is a fork of the `v3` code of [`sijms/go-ora`](https://github.com/sijms/go-ora),
made because fixes were not being accepted upstream (D1). It is under rewrite.

## Standing rules

These rules hold in every `xo` repository, for every coding agent (D3).

1. Stage changes for review. Commit and push only when Ken says so.
2. Load the `simple-english` skill before you write any text that a person
   reads: a document, a code comment, an error message or a commit message.
   Follow it for that text.
3. Load the `go-pedantry` skill before you write or review Go code. Follow it
   where it does not conflict with a rule in this file. A rule here wins.

`CLAUDE.md` holds one line that imports this file, so that Claude Code and
every other agent read the same rules. Edit this file, not that one.

## Which document to read

| If you are | Read |
| --- | --- |
| changing the shape of the driver | [docs/DESIGN.md](docs/DESIGN.md) |
| asking why something is the way it is | the table at the top of [docs/PLAN.md](docs/PLAN.md) |
| looking for work that is known and not done | [docs/BACKLOG.md](docs/BACKLOG.md) |
| testing against a database | the `dbrun` rule below, then `../dbmeta/docs/DBRUN.md` |
| looking at an upstream issue or pull request | [docs/UPSTREAM.md](docs/UPSTREAM.md) |
| comparing this driver with the other Oracle drivers | [docs/DRIVERS.md](docs/DRIVERS.md) |
| installing or updating an agent skill | [CONTRIBUTING.md](CONTRIBUTING.md) |
| resuming a session that ended | [docs/PROGRESS.md](docs/PROGRESS.md) |
| looking for what the driver does | [README.md](README.md) |

A document that is not in that table does not exist. If you cannot find where
something is written down, it is not written down. Ask. Do not decide it
yourself and do not write it as though it were settled.

## Hard rules

1. Never merge, cherry pick or copy a pull request from `sijms/go-ora` without
   asking Ken first. Triage it in `docs/UPSTREAM.md` and wait for his answer
   (D5).
2. The module imports the standard library and
   `github.com/cockroachdb/apd/v3`, for decimals (D16), and nothing else. Do
   not add a `require` line to `go.mod` without asking Ken.
3. The module path is `github.com/xo/oracle`. Do not write `sijms/go-ora`
   in code, except where a comment credits the origin of the code.
4. A change that fixes an upstream issue names the issue in its test and in
   `docs/UPSTREAM.md`, as `sijms/go-ora#771`.
5. A test that needs a database must say so and skip when none is configured.
   A test that needs none must pass with `CGO_ENABLED=0 go test ./...`.
6. Test against a real Oracle only through `dbrun`, the tool in the `test`
   module of the `dbmeta` checkout beside this one (D6). Never run
   `podman run` or `docker run` yourself. Run `dbrun status` first, and read
   the rules for a shared machine at the top of `dbmeta/docs/DBRUN.md`.
   `dbrun start oracle` starts the newest release, and `dbrun dsn oracle`
   prints the URL to give a test. Set `DBMETA_OWNER_NAME` to the name of your
   session on every `dbrun` command. To test an older release, name it, as in
   `dbrun start oracle-11g`, after `dbrun list oracle` shows what exists. This
   driver must work on 11g and 18c, which the upstream release panics on.
7. Never set an executable bit on a source file.

## Layout

- `/` is the driver, the package `oracle` (D8). `dsn.go` holds `Config` and
  `ParseDSN`, `connector.go` the `Connector`, `driver.go` the `Driver` and
  `RegisterType`, `errors.go` the sentinel errors, `values.go` the types
  of `internal/ttc` that a caller uses, `options.go` the options of one
  statement, and `output.go` the reader of `DBMS_OUTPUT`. `conn.go`,
  `stmt.go`, `rows.go` and `tx.go` hold what `database/sql` calls, and pass
  each call to the type of the same name in `internal/ttc`.
- `internal/ttc` holds the connection, statements, rows and the rest of the
  protocol above TNS (D12).
- `aq` is public. `internal/types` holds the value types, and `values.go`
  names the ones a caller uses.
- `internal/` holds everything that a caller does not use, and the table in
  `docs/DESIGN.md`, under Packages, says what each package holds (D12).
- `internal/repotest` holds the tests of the repository setup.
- `internal/tnstest` records the TNS packets of a connection and plays them
  back with no server. `testdata/exchanges/<release>/` holds the recordings,
  and `exchange_test.go` the scenarios. `docs/DESIGN.md`, under Testing, says
  how to record them again (D15).
- `integration` and `module_test` are the upstream integration tests. They need
  a live database.

## Go conventions

Match the surrounding code until the rewrite reaches it, and do not carry an
unidiomatic pattern forward just because it is there.

## Before you commit

```sh
gofmt -l . && go vet ./... && go build ./... && go test ./...
```

## Writing documentation

Only `README.md`, `AGENTS.md`, `CLAUDE.md` and `CONTRIBUTING.md` belong in the
repository root. A new document goes in `docs/`. Add it to the table at the
top of this file.

A decision goes in `docs/PLAN.md`, with its row in the table at the top. It
opens with `Status: Decided`, or names the decision it amends, and the older
decision says so back.

Standing rule 2 applies: load `simple-english` first. Use short sentences and
the active voice. Use `can`, `will` and `must`, and do not use `should`, `may`
or `might`. Do not use contractions, semicolons or em dashes.

Work that is known and not done goes in `docs/BACKLOG.md`. When an item is
done, delete it, and record anything decided on the way in `docs/PLAN.md`.
