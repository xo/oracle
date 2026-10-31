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

- `/` is the driver, in package `go_ora` until the rename in the open
  question in `docs/PLAN.md` is settled.
- `advanced_nego`, `network`, `converters`, `configurations`, `types`, `aq`,
  `parameter_coder`, `trace` and `utils` are internal pieces of the driver.
- `internal/repotest` holds the tests of the repository setup.
- `TestIssues` and `module_test` are the upstream integration tests. They need
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
