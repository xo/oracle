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

### Report SELECT CAST(:1 AS JSON) on 21c

On 21c, `SELECT CAST(:1 AS JSON) FROM dual` with a JSON bind fails inside the
server with ORA-00600 [25026] or [kcbgtcr_1]. The same bind inserts into a JSON
column, `JSON_SERIALIZE(CAST(:1 AS JSON))` works, and 26ai returns it. Find
whether the driver can send the bind so that 21c accepts it, as python-oracledb
does, or record it as a fault of 21c.

### Bind and read a LOB attribute of an object as text

An object with a CLOB or a BLOB attribute reads into an `oracle.Clob` or an
`oracle.Blob` field, which loads after the call. A `string` or a `[]byte`
field is an error, because the decode fills the field while the reply is
read. A bind of such an object fails with ORA-03101 on 26ai. go-ora panicked
on the read. Load the LOBs of an object before its fields are filled, and
write a LOB attribute as python-oracledb does. `TestObjectLobAttributes`
holds what works.

### Tell dburl when the first tag exists

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
