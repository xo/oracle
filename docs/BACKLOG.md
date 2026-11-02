# Backlog

This document lists work that is known and not done. Each item names the
decision or the fact that found it.

A decision is not a backlog item. It goes in [`PLAN.md`](PLAN.md). When an
item here is done, delete it, and record in `PLAN.md` anything that was
decided on the way (D3).

## Repository

### Reshape the TTC code

Step 4b moved the TTC code to `internal/ttc` as it was, and steps 5 and 6
reshaped it in place. Its names are still the ones of `go-ora`: `Connection`,
`Stmt`, `DataSet`, `ParameterInfo`, `Transaction`. `DESIGN.md`, under The root
package, holds the target, with `conn`, `stmt`, `rows` and `tx`. The Oracle
type IDs are constants of `uint16`, and want a named type. `Config` has no keys for the OS user and password of NTS, the domain, the
list of authentication services or out of band breaks, which `go-ora` read. Add each when a caller needs it.

### Register a record of %ROWTYPE

`RegisterType` takes a type of SQL and a type of a package (D33). A record
of `TABLE%ROWTYPE`, alone or as the element of a collection of a package, is
in neither view. python-oracledb reads its type OID from
`DBMS_PICKLER.GET_TYPE_SHAPE` and its attributes from `ALL_TAB_COLS`, and the
driver can do the same.

### Report batch errors and the count of each row

A batch of DML stops at its first error, and returns the count of all its
rows. python-oracledb can go on past the rows that fail and return the error
of each, with `batcherrors`, and return the count of each row, with
`arraydmlrowcounts`. Their cases of `cursor/test_2700_features_12_1.py`
(2700, 2701, 2717 to 2719, 2722 to 2724, 2726, 2727, 2730 and 2731) are not
written. Each needs an API of its own, as [`DRIVERS.md`](DRIVERS.md) says.
Find a form for `database/sql`, and ask Ken.

### Return the rows of a RETURNING of more than one row

A RETURNING that touches more than one row fails with an error, because an
OUT takes one value. python-oracledb returns an array for each OUT. Find a
form for `database/sql`, such as an OUT of a slice, and ask Ken.

### Tell dburl and usql when the first tag exists

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

### Fix the upstream issues

[`UPSTREAM.md`](UPSTREAM.md) holds the triage of every open issue and pull
request of `sijms/go-ora`, and the work that follows from it.
