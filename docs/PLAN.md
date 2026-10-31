# Plan

`oracle` is the `xo` fork of the `v3` code of `sijms/go-ora`. This file holds
the plan, the decisions, and the open questions for Ken.

Each decision has a status. An amendment must be visible from both sides: if
D9 amends D4, then D4 says so too. A decision is never edited to change its
conclusion.

| Decision | Status |
| --- | --- |
| [D1](#d1-oracle-is-a-fork-of-the-v3-code-of-sijmsgo-ora) | Decided |
| [D2](#d2-main-holds-the-v3-code-at-the-root-with-no-old-tags) | Decided |
| [D3](#d3-oracle-is-set-up-for-coding-agents-as-every-xo-repository-is) | Decided |
| [D4](#d4-the-module-path-is-githubcomxooracle) | Decided, the package name settled by D8 |
| [D5](#d5-no-upstream-pull-request-is-merged-without-asking-ken) | Decided |
| [D6](#d6-oracle-is-tested-against-databases-started-by-dbrun) | Decided |
| [D7](#d7-ci-tests-three-operating-systems-and-every-oracle-release-dbmeta-knows) | Decided |
| [D8](#d8-the-driver-follows-the-design-of-the-dbimp-drivers) | Proposed, the package name decided and built |
| [D9](#d9-a-connector-holds-everything-that-belongs-to-one-database) | Proposed |
| [D10](#d10-a-context-ends-a-round-trip-through-deadlines-and-contextafterfunc) | Proposed |
| [D11](#d11-the-dsn-is-an-oracle-url-with-lower-case-keys) | Decided |
| [D12](#d12-the-protocol-code-moves-to-internal) | Proposed |
| [D13](#d13-the-module-requires-go-1271) | Decided |
| [D14](#d14-the-driver-logs-only-to-a-logger-that-the-caller-passes) | Decided |
| [D15](#d15-unit-tests-play-back-recorded-tns-exchanges) | Proposed |
| [D16](#d16-a-decimal-is-an-apddecimal) | Decided. Amends hard rule 2 of AGENTS.md |

## Purpose

`usql`, `dbtpl` and `dbmeta` reach Oracle through a pure Go driver. The
upstream driver has open faults and pull requests that the maintainer has not
taken. This project keeps a driver that `xo` can fix.

## D1. oracle is a fork of the v3 code of sijms/go-ora

Status: Decided.

Ken decided on 2026-10-31 to fork `sijms/go-ora`, because the maintainer
declined to make the fixes that `xo` needs. The fork starts from the `v3` code
at upstream commit `62f8a42`.

## D2. main holds the v3 code at the root, with no old tags

Status: Decided.

The `main` branch holds the contents of the old `v3` directory at the root. The
`v1` and `v2` code, the examples, the old workflows and the funding file are
deleted. The 186 upstream tags are not imported, because they name a code line
that this project does not carry. The history of the upstream commits stays in
the branch. Every file has mode 0644.

## D3. oracle is set up for coding agents as every xo repository is

Status: Decided.

The repository follows D110 of `dbmeta`: `AGENTS.md`, a one line `CLAUDE.md`,
`CONTRIBUTING.md`, `skills-lock.json`, the `simple-english` and `go-pedantry`
skills committed as copies, `.gitattributes`, `.gitignore`, and `docs/` with
`PLAN.md` and `BACKLOG.md`. A small library keeps its decisions in this file
(D111 of `dbmeta`). `TestSkillsAreCopies` and `TestClaudeImportsAgents` hold
the setup.

## D4. The module path is github.com/xo/oracle

Status: Decided.

Ken decided on 2026-10-31 that the project lives at
`https://github.com/xo/oracle`. The module path changed from
`github.com/sijms/go-ora/v3` to `github.com/xo/oracle`, and every import
followed. The package name did not change then. D8 settles it as `oracle`.

## D5. No upstream pull request is merged without asking Ken

Status: Decided.

The open pull requests of `sijms/go-ora` are triaged in `docs/UPSTREAM.md`.
Nothing from one is applied here until Ken says so. A fix can be written here
from the issue instead.

## D6. oracle is tested against databases started by dbrun

Status: Decided.

Ken decided on 2026-10-31 that this driver is tested against the Oracle
releases that `dbrun` starts. `dbrun` is the tool of `dbmeta`, and it is the
only thing that starts a database in the `xo` projects (D68 of `dbmeta`). It
names every release as `<product>-<release>`, shares the machine between
sessions, and prints the connection URL. This project builds no container
list of its own. When `dbrun` has no release that a fix needs, such as 11g or
18c, the release is added to `dbmeta` and not here.

## D7. CI tests three operating systems and every Oracle release dbmeta knows

Status: Decided.

Ken decided on 2026-10-31 to discard the upstream workflow and write a new one.
The unit job runs on `ubuntu-latest`, `macos-latest` and `windows-latest`. The
oracle job runs the integration tests against every release that `dbrun list`
names for Oracle, on every tier: 11g, 18c, 19c, 21c, 23ai and 26ai on the day of
writing. The workflow reads the list from `dbrun` and names no release itself.
Oracle runs only in a Linux container, and only the ubuntu runner has Docker for
one, so macOS and Windows run the unit job and not the oracle job. The 18c and
19c releases are verified in `dbmeta` and not in its CI, because 19c is built
from an installer that Oracle does not publish as a free image (see
`container/oracle.go` in `dbmeta`). Those two jobs can fail until `dbmeta` makes
them startable on a runner. See the backlog.

## D8. The driver follows the design of the dbimp drivers

Status: Proposed.

Ken asked on 2026-10-31 that the driver be redesigned in the way of the
`xo/dbimp` drivers, with the context flow and the idioms of Go 1.27.
[DESIGN.md](DESIGN.md) holds the design. It keeps the contract of dbimp D8,
the conventions of dbimp D6 and dbimp D25, and the file layout of `cassandra`.
D9 to D16 hold its parts. Each part is proposed until Ken accepts the design.

Ken decided on 2026-10-31 that the package is `oracle`, the name that it
registers with `database/sql` and the scheme of its DSN.

## D9. A Connector holds everything that belongs to one database

Status: Proposed.

The package variable `oracleDriver` and every function that takes a `*sql.DB`
go. The session parameters, the registered types, the TLS configuration and
the charsets belong to a `Connector` or to one connection. Two databases in one
process then share nothing, which fixes sijms/go-ora#692.

Step 4 deleted `oracleDriver`, `GetDefaultDriver`, `AddSessionParam`,
`DelSessionParam`, `AddParameterCoder`, `SetStringConverter`,
`RegisterConnConfig` and the root `ParseConfig`. Four pieces of state for the
whole process remain, and `BACKLOG.md` lists them.

## D10. A context ends a round trip through deadlines and context.AfterFunc

Status: Proposed.

`StartContext` and its goroutine for each call go. The deadline of the context
becomes the deadline of the connection, and `context.AfterFunc` sends the break.
A transaction keeps no context, so a rollback is never cancelled, which fixes
sijms/go-ora#767. The library never calls `context.Background()` or
`context.TODO()`.

## D11. The DSN is an oracle URL with lower case keys

Status: Decided.

Ken decided on 2026-10-31 that this is a new package, done differently, and
keeps nothing of the DSN of `go-ora`. The DSN follows dbimp D27 and dbimp D35.
The scheme must be `oracle`, the port is 1521 when the URL names none, and an
unknown or repeated key is an error. Every key is lower case with no space,
and no old key name is accepted.

The path is `/service/dbname`. Ken confirmed the same day that `dbname` is the
instance name, which goes in the connect data as `INSTANCE_NAME`, as in the
old `dburl` form `/service_name/instance_name`. A service name that holds a
`/` is escaped by the code that builds the URL. Ken did not find a key that
overrides the path necessary, so there is none.

The TLS settings follow `pgx` and the other `usql` drivers: `sslmode` with
`disable`, `require`, `verify-ca` and `verify-full`, and `sslrootcert`,
`sslcert`, `sslkey` and `sslservername`. dbimp has no one form to copy. Its
drivers use `tls=true` with `insecure` or `validateservercertificate`, because
each speaks HTTP, where TLS is one switch. Ken accepted these keys and values
on 2026-10-31. `DESIGN.md` names every key. This
changes what `dburl` sends, so `dburl` changes with it.

Step 4 built `ParseDSN`, `Config` and `Config.DSN` on 2026-10-31.

## D12. The protocol code moves to internal

Status: Proposed.

TNS, TTC, authentication, charsets and the type codecs move under `internal/`,
so that they can change with no new major version. The root package, `aq/`,
and the value types that a caller passes or scans stay public.

Step 3 of `DESIGN.md` moved the packages of `go-ora` on 2026-10-31, and
changed nothing else. The recorded exchanges passed before and after.

## D13. The module requires Go 1.27.1

Status: Decided. Amends the `go 1.24` line of `go.mod`.

`go.mod` says `go 1.27.1`, as in dbimp D2, and the code uses the idioms that
`DESIGN.md` lists. The earlier size analysis asked for Go 1.26. Go 1.27.1 is
newer, and every other `xo` driver requires it. Ken asked for the idioms of Go
1.27, and step 1 of `DESIGN.md` made the change on 2026-10-31.

## D14. The driver logs only to a logger that the caller passes

Status: Decided.

The `trace` package and the keys that name a trace file go. Ken decided on
2026-10-31 that a caller that needs the packets sets `Config.Logger`, a
`*slog.Logger`. A nil logger, the default, logs nothing, so the driver still
holds no configuration of its own, by dbimp hard rule 2. No password or key
reaches the logger.

## D15. Unit tests play back recorded TNS exchanges

Status: Proposed.

A unit test needs no server. A fake server on a `net.Pipe` plays back packets
recorded from the releases that `dbrun` starts. An integration test reads
`ORACLE_DSN` and skips when it is empty.

## D16. A decimal is an apd.Decimal

Status: Decided. Amends hard rule 2 of `AGENTS.md`.

Ken decided on 2026-10-31 that a `NUMBER` that does not fit an `int64` scans
into `*any` as an `*apd.Decimal`, from `github.com/cockroachdb/apd/v3`, as in
dbimp D33. It is the one dependency of the module. `NUMBER` holds 38 digits,
and a `float64` loses them.

## Open questions for Ken

1. Does the work go in the order that `DESIGN.md` gives, with nothing tagged
   until step 6 is done?
