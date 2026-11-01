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

### Write the tests of python-oracledb again

D28 asks for the tests of python-oracledb that test what this driver does.
Written: `tnsnames.ora` (`test_1500_tnsnames.py`), descriptors
(`test_1300_connect_params.py`), statements that change type
(`test_2500_type_changes.py`), runs with other binds (`test_1000_execute.py`
and `test_2000_char.py`), DML RETURNING of one row
(`test_2200_dml_returning.py`), batches of DML (`test_1200_executemany.py`),
the sizes of LOBs and the order of their binds (`test_1500_lobs.py`), and the
bounds of NUMBER (`test_1800_number.py`), objects
(`test_1900_dbobject.py`), LONG (`test_1600_longs.py`), ROWID
(`test_3400_rowid.py`), and cases of `test_1400_callproc.py`,
`test_2000_char.py` and the intervals, the bounds of dates
(`test_1100_date.py`), BOOLEAN (`test_2400_boolean.py`), and the field names
of JSON (`test_3200_json_26ai.py`). Left, in the order of their value: the
other cases of JSON (`test_3100_json.py`), NCHAR (`test_1700_nchar.py`), and
VECTOR, which waits on the item below.

### Decide the Go types of VECTOR

A `VECTOR` of `INT8` reads as `[]uint8`, so a negative element comes back
wrong. The `BINARY` format and sparse vectors have no Go type, although the
comment of `CreateVector` says that it takes a sparse one. python-oracledb
reads `INT8` as signed, `BINARY` as unsigned bytes, and has `SparseVector`.
Ken chooses the Go types, and then the tests of `test_2700_vector.py` and
`test_6000_vector.py` are written again.

### Return the rows of a RETURNING of more than one row

A RETURNING that touches more than one row fails with an error, because an
OUT takes one value. python-oracledb returns an array for each OUT. Find a
form for `database/sql`, such as an OUT of a slice, and ask Ken.

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
