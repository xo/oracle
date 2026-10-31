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
| [D7](#d7-ci-tests-three-operating-systems-and-every-oracle-release-dbmeta-knows) | Decided, amended by D17 |
| [D8](#d8-the-driver-follows-the-design-of-the-dbimp-drivers) | Decided |
| [D9](#d9-a-connector-holds-everything-that-belongs-to-one-database) | Decided |
| [D10](#d10-a-context-ends-a-round-trip-through-deadlines-and-contextafterfunc) | Decided |
| [D11](#d11-the-dsn-is-an-oracle-url-with-lower-case-keys) | Decided |
| [D12](#d12-the-protocol-code-moves-to-internal) | Decided |
| [D13](#d13-the-module-requires-go-1271) | Decided |
| [D14](#d14-the-driver-logs-only-to-a-logger-that-the-caller-passes) | Decided |
| [D15](#d15-unit-tests-play-back-recorded-tns-exchanges) | Decided |
| [D16](#d16-a-decimal-is-an-apddecimal) | Decided. Amends hard rule 2 of AGENTS.md |
| [D17](#d17-ci-leaves-out-19c) | Decided. Amends D7, amended by D23 |
| [D18](#d18-four-upstream-pull-requests-are-written-again-here) | Decided |
| [D19](#d19-the-driver-reads-dbms_output) | Decided |
| [D20](#d20-the-driver-reads-tnsnamesora-with-a-parser-of-its-own) | Decided |
| [D21](#d21-four-upstream-features-are-taken) | Decided |
| [D22](#d22-a-lob-is-read-in-pieces-until-its-rows-close) | Decided |
| [D23](#d23-no-test-runs-on-19c) | Decided. Amends D17 |
| [D24](#d24-a-long-value-binds-as-a-temporary-lob-in-plsql-only) | Decided |
| [D25](#d25-a-collection-of-a-basic-type-binds-through-a-named-type-or-object) | Decided |

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
them startable on a runner. D17 amends this: CI leaves out 19c.

## D8. The driver follows the design of the dbimp drivers

Status: Decided. Ken accepted it on 2026-10-31, as built.

Ken asked on 2026-10-31 that the driver be redesigned in the way of the
`xo/dbimp` drivers, with the context flow and the idioms of Go 1.27.
[DESIGN.md](DESIGN.md) holds the design. It keeps the contract of dbimp D8,
the conventions of dbimp D6 and dbimp D25, and the file layout of `cassandra`.
D9 to D16 hold its parts. Ken accepted the design and its parts on
2026-10-31, as built.

Ken decided on 2026-10-31 that the package is `oracle`, the name that it
registers with `database/sql` and the scheme of its DSN.

## D9. A Connector holds everything that belongs to one database

Status: Decided. Ken accepted it on 2026-10-31, as built.

The package variable `oracleDriver` and every function that takes a `*sql.DB`
go. The session parameters, the registered types, the TLS configuration and
the charsets belong to a `Connector` or to one connection. Two databases in one
process then share nothing, which fixes sijms/go-ora#692.

Step 4 deleted `oracleDriver`, `GetDefaultDriver`, `AddSessionParam`,
`DelSessionParam`, `AddParameterCoder`, `SetStringConverter`,
`RegisterConnConfig` and the root `ParseConfig`. Four pieces of state for the
whole process remain, and `BACKLOG.md` lists them.

Step 4b kept `RegisterType` with its `*sql.DB`, and wrote no `WithType`. A
type is registered after the program creates it, through the same database,
and an option that acts at the first connection cannot wait for that.
`RegisterType` changes the registry of its own connector alone, so this
decision still holds. `DESIGN.md`, under Connector, says more.

## D10. A context ends a round trip through deadlines and context.AfterFunc

Status: Decided. Ken accepted it on 2026-10-31, as built.

`StartContext` and its goroutine for each call go. The deadline of the context
becomes the deadline of the connection, and `context.AfterFunc` sends the break.
A transaction keeps no context, so a rollback is never cancelled, which fixes
sijms/go-ora#767. The library never calls `context.Background()` or
`context.TODO()`.

Step 5 built it on 2026-10-31, with one change. The deadline of the context is
not the deadline of the connection, because a read that fails at a deadline
loses the connection, and a break keeps it. `DESIGN.md`, under Context, says
how. The tests of `context_test.go` pass on 11g, 21c and 26ai.

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

Status: Decided. Ken accepted it on 2026-10-31, as built.

TNS, TTC, authentication, charsets and the type codecs move under `internal/`,
so that they can change with no new major version. The root package, `aq/`,
and the value types that a caller passes or scans stay public.

Step 3 of `DESIGN.md` moved the packages of `go-ora` on 2026-10-31, and
changed nothing else. The recorded exchanges passed before and after. Step 4b
moved the TTC code of the root package to `internal/ttc` the same day, and the
root package now exports 31 names, where `go-ora` exported 167.

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

Status: Decided. Ken accepted it on 2026-10-31, as built.

A unit test needs no server. A fake server on a `net.Pipe` plays back packets
recorded from the releases that `dbrun` starts. An integration test reads
`ORACLE_DSN` and skips when it is empty.

## D16. A decimal is an apd.Decimal

Status: Decided. Amends hard rule 2 of `AGENTS.md`.

Ken decided on 2026-10-31 that a `NUMBER` that does not fit an `int64` scans
into `*any` as an `*apd.Decimal`, from `github.com/cockroachdb/apd/v3`, as in
dbimp D33. It is the one dependency of the module. `NUMBER` holds 38 digits,
and a `float64` loses them.

Step 6 built it on 2026-10-31. A `NUMBER` with a scale of 0 and a precision
from 1 to 18 is an `int64`, and any other is an `*apd.Decimal`.

## D17. CI leaves out 19c

Status: Decided. Amends D7.

Ken decided on 2026-10-31 that the oracle job of CI leaves out 19c. Its image
is built by `dbrun build` from an installer that Oracle does not publish, and
its first start takes about 15 minutes. A change that touches the protocol is
tested on 19c through `dbrun` on a machine that has the image. The workflow
names no other release, and reads the rest from `dbrun list`. D23 drops the
test on 19c.

## D18. Four upstream pull requests are written again here

Status: Decided.

Ken decided on 2026-10-31 to take sijms/go-ora#734, #721, #770 and #773, as
D5 asks. Each is written again for this code, with a test, and none is merged
as it is. sijms/go-ora#770 serves #771, which needs an Autonomous Database to
test, so its test is a unit test of the flag bits.

## D19. The driver reads DBMS_OUTPUT

Status: Decided.

Ken decided on 2026-10-31 that the driver reads the output of `DBMS_OUTPUT`,
as `go-ora` v2 did through `dbms.NewOutput`, with a helper and a live test.

## D20. The driver reads tnsnames.ora with a parser of its own

Status: Decided.

Ken decided on 2026-10-31 that a DSN can name an alias of `tnsnames.ora`,
which the driver finds through `TNS_ADMIN`, as sijms/go-ora#616 asks. The
parser is written here, small, and does not take the 1,470 lines of that pull
request.

## D21. Four upstream features are taken

Status: Decided.

Ken decided on 2026-10-31 to take these requests of `sijms/go-ora`: an option
to skip the NLS query at connect (#611), a logon with an OCI IAM token
(#530), streaming of LOBs and rows (#681, #559 and #664), and failover to
another address (#707 and #619). He also asked that the refusal of a server
that requires native network encryption (#729) be reproduced and fixed. The
other features stay open.

## D22. A LOB is read in pieces until its rows close

Status: Decided.

Ken decided on 2026-10-31 how a caller reads one LOB in pieces, for
sijms/go-ora#681 and sijms/go-ora#559 (D21). `Blob`, `Clob` and `BFile` have
`NewReader(ctx)`, which returns an `io.Reader`. Each read of the server
watches `ctx`. A CLOB reader returns UTF-8. A reader asks for pieces of
1 MiB, and `NewReaderSize` sets another size.

A LOB of a row can be read until its rows close. A read after that fails
with an error. A LOB reads through the connection of its rows, and after
`rows.Close` `database/sql` can give that connection to another goroutine,
so a later read would race with it. The `Clob.Read` of `go-ora` had that
race.

## D23. No test runs on 19c

Status: Decided. Amends D17.

Ken decided on 2026-11-01 that no test runs on 19c, not even a change to the
protocol. The tests on 18c and 26ai cover what lies between, so 19c repeats
them. No exchange is recorded on 19c. `oracle-adb-19c` is another image, and
the tests still run on it.

## D24. A long value binds as a temporary LOB in PL/SQL only

Status: Decided.

Ken decided on 2026-11-01 how a string or a `[]byte` longer than the longest
`VARCHAR2` or `RAW` binds. In SQL it stays a `LONG` or a `LONG RAW`, so an
`INSERT` or an `UPDATE` takes one round trip and a `LONG` column still takes
it on 11g. SQL refuses such a bind in an expression or in the `SELECT` of a
`MERGE`, and the caller binds `CreateClob` or `CreateBlob` there, as JDBC and
python-oracledb ask for an explicit LOB type. In a PL/SQL block a value binds
as a `VARCHAR2` or a `RAW` up to 32,767 bytes, and as a temporary LOB past
that, as python-oracledb does. A per-statement option that binds a temporary
LOB in SQL can come later, if a caller needs one.

Gemini 3.1 Pro said that 12c and later take the data of a LOB in a bind with
no locator, and that python-oracledb binds it so. The source of
python-oracledb shows a temporary LOB for each value instead, so the driver
does not build such a bind. DeepSeek V4 Pro gave no answer in time, and no
OpenAI model was at hand.

## D25. A collection of a basic type binds through a named type or Object

Status: Decided.

Ken decided on 2026-11-01 how a caller binds a collection of a basic type,
such as a `TABLE OF DATE`, which `RegisterType` registers with the name of its
element type. The main way is a named Go element type, such as
`type Name string`, which `RegisterType` maps to the collection. The root
package also exports `Object`, which names the type at the bind, as
`go_ora.Object` did, for a basic type with no named Go type. Registering a
`[]time.Time` or a `[][]byte` as the collection would take those types from
every bind of the connector, PL/SQL arrays among them.

## Open questions for Ken

1. When is the first tag? Ken will say. Step 6 is done, and `dburl` waits on
   it.
