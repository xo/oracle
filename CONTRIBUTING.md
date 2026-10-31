# Contributing to oracle

Read [`AGENTS.md`](AGENTS.md) before you change anything. It holds the rules,
is written for a coding agent, and everything in it applies to a person.
`CLAUDE.md` holds one line that imports it, for Claude Code.

[`docs/PLAN.md`](docs/PLAN.md) holds every decision this project has made and
the open questions. Do not decide an open question on your own. Ask Ken.

## Before you send a change

```bash
gofmt -l . && go vet ./... && go build ./... && go test ./...
```

`gofmt -l .` must print nothing. Tests in `integration` need a live Oracle
database and skip without one.

## Testing against Oracle

Tests that need a database run against a release that `dbrun` starts. `dbrun`
lives in the `test` module of [`dbmeta`](https://github.com/xo/dbmeta), and
nothing else starts a database. Read the rules for a shared machine at the top
of its `docs/DBRUN.md` first.

```bash
cd ../dbmeta/test && go run ./cmd/dbrun start oracle && go run ./cmd/dbrun dsn oracle
```

## Upstream issues and pull requests

This project fixes the open issues of `sijms/go-ora` where it can.
[`docs/UPSTREAM.md`](docs/UPSTREAM.md) records which issues and pull requests
are valid and what was done about each. Do not bring in an upstream pull
request without asking Ken.

## Agent skills

The repository carries two agent skills. A skill is a set of instructions that
a coding agent loads for a task. `simple-english` sets how prose is written,
and `go-pedantry` sets how Go is written. `skills-lock.json` names the source
of each skill. Codex and the other agents read `.agents/skills/<name>`, and
Claude Code reads `.claude/skills/<name>`.

To add or update a skill, run this in the repository root. The example updates
`simple-english`:

```bash
npx skills@1.7.0 add AminBlg/SimpleEnglish --skill simple-english --agent codex claude-code --copy -y
```

Keep `--copy`. Without it, the command writes a symbolic link, and a Windows
checkout writes a link as a text file, so Claude Code loads no skill and says
nothing. `TestSkillsAreCopies` fails on a link and when the two folders differ.

`.claude/settings.local.json` holds the Claude Code permissions of one person.
The `.gitignore` ignores it.
