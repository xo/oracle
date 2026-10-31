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
| [D4](#d4-the-module-path-is-githubcomxooracle) | Decided |
| [D5](#d5-no-upstream-pull-request-is-merged-without-asking-ken) | Decided |
| [D6](#d6-oracle-is-tested-against-databases-started-by-dbrun) | Decided |
| [D7](#d7-ci-tests-three-operating-systems-and-every-oracle-release-dbmeta-knows) | Decided |

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
followed. The package name did not change. See the open question.

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

## Open questions for Ken

1. What should the Go package name be? It is `go_ora` now. The driver name that
   `sql.Open` takes is `oracle`, and `oracle` is the obvious package name. A
   rename touches every file and every importer that uses the old name.
