# Backlog

This document lists work that is known and not done. Each item names the
decision or the fact that found it.

A decision is not a backlog item. It goes in [`PLAN.md`](PLAN.md). When an
item here is done, delete it, and record in `PLAN.md` anything that was
decided on the way (D3).

## Repository

### Add the keys of NTS when a caller needs them

`Config` has no keys for the OS user and password of NTS, the domain, the
list of authentication services or out of band breaks, which `go-ora` read.
Add each when a caller needs it.

### Tell dburl and usql when the first tag exists

The first tag is v0.1.0 (D36). It is made when Ken says, and the two
sessions are told then.

usql moved to this module on 2026-11-02, at a pseudo-version, and keeps one
hook, `IsPasswordErr`, until a release has `AuthenticationRefused`. When the
first tag exists, tell the usql session, which then registers the driver
with no hook.

dburl staged its `oracle` scheme as provisional on 2026-10-31 (its D69), with
`github.com/xo/oracle` as the package. Its `GenOracle` writes
`oracle://user:pass@host:1521/service?<query>`. It adds the host `localhost`
and the port 1521, and it passes the path and the query through as they are,
so `%2F` in a service name and every `ssl*` key reach the parser unchanged.
dburl maps no `tls` key to `sslmode` for any scheme, and will not. The parser
of D11 landed in step 4. When the first tag exists, tell the dburl session,
which then rechecks its tests and documents.

### Test the upstream fixes that have no test

D27 finished the open issues of `sijms/go-ora`, and
[`UPSTREAM.md`](UPSTREAM.md) holds the triage of each. These fixes have no
test:

- sijms/go-ora#774: a logon with a password longer than 30 characters.
- sijms/go-ora#757: a dial leaves the `*tls.Config` of the caller as it was.
- sijms/go-ora#765: after a listener redirect, TLS checks the certificate
  against the host of the DSN. No release of `dbrun` redirects.
- sijms/go-ora#692: two databases with different charsets in one process.
  Every release of `dbrun` is AL32UTF8.
