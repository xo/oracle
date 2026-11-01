# Upstream

This document records the open issues and pull requests of
[`sijms/go-ora`](https://github.com/sijms/go-ora) and what this project does
about each. Nothing from an upstream pull request comes in without Ken's
agreement (D5). A fix can be written here from the issue instead.

The list was read on 2026-10-31: 58 open issues and 12 open pull requests.
Gemini 3.1 Pro gave a verdict on each item. DeepSeek V4 Pro timed out three
times and gave none, so a second opinion is still owed. Each verdict below was
then checked against the code at `62f8a42`, and a check overrides the model
where the two differ. "Checked" means a person or an agent read the code. It
does not mean the fault was reproduced against a database.

## Already fixed in this code

These are open upstream, but the `v3` code this project starts from already
holds the fix. Each needs a test against an old release before it is closed.

| Issue | Fault | What the code shows |
| --- | --- | --- |
| sijms/go-ora#759 | panic on connect to 11g, reading bytes 41 to 45 of a 32 byte ACCEPT packet | `internal/tns/accept_packet.go` reads them only when the packet is 45 bytes or longer. Checked |
| sijms/go-ora#774 | ORA-01017 for a password longer than 30 characters | `data_type_nego.go` sends 234 as the logon types byte, which holds `O9LOGON_LONG_PASSWORD`. The fault is in `v2` only. Checked |
| sijms/go-ora#686 | `isEqualLoc` is always true | Step 9 deleted `isEqualLoc`, since a `DATE` no longer takes the zone of the server and nothing compares zones. Checked |
| sijms/go-ora#764 | v3.0.1 cannot log in, fixed by commit `1f57ed4` | `1f57ed4` is in this history. Checked |

## Faults to fix

Ordered by priority. P0 loses data or crashes, P1 gives a wrong answer or
blocks a connection, P2 is a fault with a workaround.

| Issue | Priority | Fault | Fix | Status |
| --- | --- | --- | --- | --- |
| sijms/go-ora#767 | P0 | A cancelled transaction can commit. `Rollback` runs under the cancelled context, and `breakConn` is read without the lock | Roll back under `context.WithoutCancel`, and read `breakConn` under the lock | Fixed in step 5. `TestCancelledTransactionDoesNotCommit` fails before and passes after, on 11g, 21c and 26ai |
| sijms/go-ora#776 | P1 | `SELECT NULL a FROM dual` returns no row and no error. sijms/go-ora#755 is the same family in `v2` | Do not end the row loop on a row whose columns are all zero length | Fixed in step 9. A NULL of no type has a zero `MaxLen`, and `BasicRead` read it as a column that sends nothing. It reads the length now, and `Next` fetches when the execute brought no row. `TestUntypedNullRow` holds it, on 26ai |
| sijms/go-ora#769 | P1 | A CLOB over about 8K UTF-16 units with characters outside the BMP reads back as U+FFFD with `LOB FETCH=INLINE` | Fetch inline CLOB text as AL16UTF16. sijms/go-ora#768 does this | Fixed in step 9, as the issue says and without the pull request. An inline CLOB is defined in the national charset when that is AL16UTF16. It reproduced on 11g as well, with the same 4 U+FFFD, and 26ai read it right. `TestClobOutsideBMP` holds it, on 11g and 26ai. A write had the same fault on 11g, which no issue names: a string of over 4000 bytes bound into a CLOB stored U+FFFD for a character that a piece split. Before 19c such a string binds as a temporary CLOB, since 18c has the fault too, and `TestStrings` and `TestLongColumn` hold it |
| sijms/go-ora#765 | P1 | After a RAC SCAN redirect, TLS checks the certificate against the redirect address | Keep the host of the connect string as `ServerName`. sijms/go-ora#766 does this | Fixed in step 4. The configuration names the host of the DSN, and `negotiate` keeps it. No test with a redirect yet |
| sijms/go-ora#757 | P1 | `negotiate` writes `ServerName` into the caller's `*tls.Config` on every dial | Clone the configuration before writing to it. Fix with sijms/go-ora#765 | Fixed in step 4. `negotiate` clones the configuration. No test with TLS yet |
| sijms/go-ora#771 | P1 | XMLType is NULL on Autonomous Database, whose flag word is 0x8014, and a value over about 254 bytes hangs | Test the flag bits, not equality. sijms/go-ora#770 does this in `v2` | Fixed in step 9. The `v3` code read no XMLType column at all, and gave "no parameter coder registered for name XMLTYPE". `xmlParameter` reads the image of an XMLType and tests each bit of its flag word, as python-oracledb does, so the 0x8014 of Autonomous Database reads as text. 11g sends the locator of a CLOB, which is loaded after the fetch. `TestParseXMLImage` holds the images that the issue captured, and `TestXMLType` passes on 11g, 18c, 21c, 23ai, 26ai, `oracle-adb-19c` and `oracle-adb-26ai` |
| sijms/go-ora#692 | P1 | Two databases with different `NLS_CHARACTERSET` in one process garble text | Keep the charset per connection | Fixed in step 9. The converters were per connection already, and the fast login cookie, which names the charsets, was kept for the whole process under the UUID of the server and its service name, so two databases named XE could share one. Each `Connector` keeps its own cookies now. `TestCookiesPerRegistry` holds it. No test with two charsets, since every image of `dbrun` is AL32UTF8 |
| sijms/go-ora#702 | P1 | The context deadline is not kept in `PrepareContext` and `ExecContext` | Set a deadline on the network connection | Fixed in step 5, unless the reporter saw another cause. The deadline breaks a query and an exec in about one second, on 11g, 21c and 26ai |
| sijms/go-ora#703 | P1 | `INSERT ... RETURNING` into a string through `sql.Out` with no size breaks the connection | Give a default size, or return an error that does not break the connection | Fixed. An OUT string gets a buffer of its size in characters. `TestNullOutArguments` holds it, and the logon NLS query of 11g no longer fails. In step 9 an NCHAR OUT with no size gets the longest string, and its byte length counts the bytes of a character, so `RETURNING` no longer cuts the value. `TestReturningStringWithNoSize` holds it, on 26ai |
| sijms/go-ora#762 | P2 | `DataSet.Close` on a nil `*DataSet` panics | Return early on a nil receiver | Fixed in step 9. `TestNilDataSetClose` holds it |
| sijms/go-ora#624 | P2 | Panic, slice bounds out of range in `Session.GetInt64` | Check the length before slicing | Fixed in the `v3` code, which rejects a size over 8. Step 9 adds `TestReadInt64Size`, and `ReadString` returns an error where it sliced past the end |
| sijms/go-ora#695 | P2 | Index out of range in a batch insert | Check that every parameter array has the batch length | Fixed in step 9. An empty array panicked at `Index(0)`, and a shorter array cut the batch short. Both are errors now. `TestBatchLengths` holds it, on 11g and 26ai |
| sijms/go-ora#699 | P2 | A named type such as `type S string` gives `unsupported go type` | Fall back to `reflect.Kind`. sijms/go-ora#747 does this | Fixed in step 9. `CheckNamedValue` returns `driver.ErrSkip` for a named type of a basic kind that the driver does not know, so `database/sql` converts it by its kind. `time.Duration` still binds as an interval. `TestNamedTypes` holds it, on 26ai, without sijms/go-ora#747 |
| sijms/go-ora#671 | P2 | `RefCursor.Query` on a cursor the procedure never opened breaks the connection | Return an error before the round trip | Not reproduced. The execute returns ORA-01001, and the connection stays usable. `TestUnopenedRefCursor` holds it, on 26ai |
| sijms/go-ora#698 | P2 | A `MERGE` that violates a key returns a timeout and not ORA-00001 | Needs a trace | Fixed in step 9. On 11g, which is before TTC 7, the driver read the errors of an array DML as three empty fields, so a batch that failed waited for bytes that never came. 26ai returned ORA-00001 already. `TestSummaryArrayDMLError` holds the 11g reply, and `TestBatchError` holds it on 11g and 26ai |
| sijms/go-ora#655 | P1 | A `TIMESTAMP WITH LOCAL TIME ZONE` is wrong when `DBTIMEZONE` is a region, and the zone `+00:00` is not `time.UTC` | Read the zone of the database right | Fixed in step 9. The negotiation of data types sends a region in the form of a `TIMESTAMP WITH TIME ZONE`, and the driver read it as an offset of 121 hours, so a value of a database in `Europe/London` read five days off. An offset of zero is `time.UTC` now. The issue also asks for the value in the zone of the session. The driver gives the same instant in the zone of the database, as DESIGN says, since it learns the zone of the session only by a query. `TestDBTimeZone` holds the images, and `TestLocalTimeZone` holds the values on 11g with `+00:00`, `-05:00` and `Europe/London` |
| sijms/go-ora#672 | P2 | A `DATE` reads in a fixed offset, so a date in winter gets the offset of summer | Read a `DATE` in UTC, as DESIGN says | Fixed in step 9. The driver gave a `DATE` and a `TIMESTAMP` the offset of `SYSTIMESTAMP`, which a query at each logon read. They are in UTC now, the wall clock of the value, and the query is gone. sijms/go-ora#687 asks for the zone of the server, which a fixed offset cannot give, and is the same fault. sijms/go-ora#568 compares a `DATE` with a bind, which works, since a bind sends its wall clock. `TestLocalTimeZone` holds all three |
| sijms/go-ora#697 | P1 | A statement run again fails with ORA-03146, invalid buffer length for a TTC field | Send the bind option of DML RETURNING every time | Fixed in step 9. A `RETURNING` statement that ran again with an argument changed from a value to NULL, or from NULL to a value, sent its binds again without the option that the server needs for the binds of RETURNING, and 18c and 26ai refused it with ORA-03146, and 11g closed the connection. python-oracledb sends that option for each execute of PL/SQL and of RETURNING. `TestRebind` in `integration` holds it, the case 1022 of python-oracledb, on 11g, 18c and 26ai |
| sijms/go-ora#729 | P2 | A server that requires native network encryption or checksums refuses the client with ORA-12269 | Implement the negotiation | Fixed in step 9 (D21). The refusal did not reproduce: on `oracle-nne-26ai`, whose server requires AES256 and SHA256, the driver negotiates both, with the client at any level. With `encryption=required` on 11g, the driver panicked: it gave AES the IV of 20 bytes that a server before the new version sends, which the 12.1 report in the issue also points at. AES takes an IV of zeros there, as in `v2`. `TestNetworkEncryption` requires both services from the client and finds both in `V$SESSION_CONNECT_INFO`, on 11g, 18c, 26ai and `oracle-nne-26ai` |

## Faults that need a reproduction first

Ken asked on 2026-11-01 to finish the open issues (D27). Step 9 tried each
one that this section listed:

- sijms/go-ora#700 and sijms/go-ora#706 (EOF under load) reproduced. With 150
  connections on 26ai Free, over its limit of sessions, the server closed
  some connections in the middle of the logon, and the driver returned a bare
  EOF. Each other failure was ORA-12516 or ORA-00018. With 50 connections on
  26ai and 40 on 11g, inserts and calls that return a CLOB, 245,000 of them,
  all passed. The error now says that the server closed the connection during
  the logon, and wraps `io.EOF`. `TestConnectClosedDuringLogon` holds it.
- sijms/go-ora#724 and sijms/go-ora#673 (a malformed packet from the queries
  of an exporter) did not reproduce. Eleven queries of `v$session`,
  `v$sysstat`, `v$sysmetric`, `v$waitclassmetric`, `v$resource_limit`,
  `gv$sql` and others ran 20 times each through prepared statements on 11g,
  18c, 26ai and `oracle-adb-19c`. Both reports came with go-ora v2.9.0, before
  sijms/go-ora#776, a row of untyped NULLs, which step 9 fixed.
- sijms/go-ora#615 (wrong rows with `FETCH NEXT :n ROWS ONLY`) did not
  reproduce. A prepared query with three binds and a bound `FETCH NEXT` gave
  the right rows 300 times on 18c, 26ai and `oracle-adb-19c`.

Step 9 reproduced
sijms/go-ora#683, #687, #672, #655 and #568, which the table above and the
section below hold. It did not reproduce sijms/go-ora#704: in the root
container of 18c and 26ai, `SELECT * FROM CONTAINERS(DUAL)` returns a row for
the root and one for the PDB, as `system` and as `sys`.

Step 9 did not reproduce three more. sijms/go-ora#635: a descriptor with
spaces around each `=` connects. sijms/go-ora#688: a user of a PDB that logs
on to the root gets ORA-01017, and an unknown service gets ORA-12514, on 18c,
and neither gets ORA-12564. sijms/go-ora#680 is the cipher
`TLS_RSA_WITH_AES_256_CBC_SHA`, which Go leaves out of its default list since
Go 1.22, because it has no forward secrecy. A caller that needs it names it in
the `CipherSuites` of `Config.TLS`.

sijms/go-ora#697 reports a malformed packet, ORA-03146, for statements run
again, and for a BLOB over 50 KB in a `MERGE`. The packet reproduced, and the
table above holds it. The BLOB in a `MERGE` did not. A `[]byte` longer than the longest `RAW`
binds as a `LONG RAW`, and a string longer than the longest `VARCHAR2` as a
`LONG`, and the server refuses either in a `SELECT` of a `MERGE` with
ORA-01461, on 11g, 18c and 26ai. `CreateBlob` and `CreateClob` bind a
temporary LOB, which works there at any size, and D24 keeps it so. In PL/SQL
the same values failed with ORA-01483, which D24 fixed.

## Features

sijms/go-ora#777 (Transparent Application Continuity), sijms/go-ora#707 and
sijms/go-ora#619 (failover), sijms/go-ora#681, sijms/go-ora#559 and
sijms/go-ora#664 (streaming LOBs and rows), sijms/go-ora#666 (scrollable
cursors), sijms/go-ora#611 (skip the NLS query at connect), sijms/go-ora#584
(`v$session.terminal`), sijms/go-ora#560 (`/ as sysdba`), sijms/go-ora#530 (OCI
IAM tokens), sijms/go-ora#640 (pool settings), sijms/go-ora#618 (TimesTen) and
sijms/go-ora#403 (ASM). Step 9 built #611 as the key `skip_nls`, and
then deleted the query, and the key with it, so a logon runs no query.
`TestLogonRunsNoQuery` holds it. It made a handshake that fails after the dial try the
next address, as a gateway or a VPN that accepts the dial and resets the
connection needs (#619), which also serves the failover of a descriptor of
many addresses (#707), and `TestFailover` holds both. #530 asked for the
logon with an OCI IAM token, which the `v3` code had already as
`auth=token` with `token` and `token_key`, and which no `dbrun` release can
test. Rows were read one fetch at a time already, so a query of 400,000
plain rows held under 1 MiB (#664). Rows with LOB values did not: a fetch
counted a LOB as its locator, or as 2 bytes when it came inline as a LONG,
and so asked for up to 65,537 rows and held every value at once. 300 rows
of 1 MB CLOBs held 300 MB. A fetch with LOB values inside it is sized by
the size of the values now, and a LOB fetched as a locator is loaded one
row at a time, in `Next`. The same rows hold 5 MB inline and 7 MB
streamed, and `TestLobRows` holds it. `Clob`, `Blob` and `BFile` have
`NewReader(ctx)`, which reads one LOB in pieces of 1 MiB (#681 and #559,
D22). A read after the rows close fails with `ErrRowsClosed`, where
`Clob.Read` raced with the next user of the connection. `TestLobReader`
holds it, on 11g, 18c and 26ai. Ken decided to take #611, #530, #681, #559, #664,
#707 and #619 (D21).

D27 settles the rest. #584 is built: `Client.Terminal` names the terminal of
the session, and `Client.Host` and `Client.User` reach `v$session` as the
machine and the user, where only the connect packet carried them.
`TestClientInSession` holds it. #560 needs the bequeath protocol of a local
Oracle Client, which a driver over the network cannot speak. #640 asks for
settings that `database/sql` has, `SetMaxOpenConns`, `SetConnMaxLifetime`
and a context for the wait, except a minimum of open connections, which
`database/sql` has not. #618 is TimesTen, which speaks a protocol of its own.
#403 is a logon to ASM, which `privilege=sysasm` makes and which no `dbrun`
release can test. #666 (scrollable cursors) and #777 (Transparent Application
Continuity) are not built now, and `DRIVERS.md` holds what each other driver
offers for them.

sijms/go-ora#649 asks for smaller charset tables, which step 8 built. sijms/go-ora#653 (VECTOR) and sijms/go-ora#365 (Advanced Queuing) are in
the `v3` code already, as `types.Vector` and the `aq` package.

## Not a fault in this code

sijms/go-ora#690 is about the `v2` `go.mod`. This module requires nothing.
sijms/go-ora#609 expects `rows.Err` to return `sql.ErrNoRows`, which
`database/sql` returns only from `Row.Scan`. sijms/go-ora#602 is Oracle 8i,
which is older than any release this project tests. sijms/go-ora#685 and
sijms/go-ora#682 are questions. sijms/go-ora#683 is a user with a lower case
name: the server reads a name in upper case unless it is quoted, as in SQL,
and the quoted name logs on. `TestLogonRefused` holds it, with a user named
`"go_logon"`.

## Pull requests

None is applied as it is. Ken decided to take sijms/go-ora#734, #721, #770
and #773, each written again for this code (D18). The rest wait for Ken (D5).

| Pull request | What it does | Verdict |
| --- | --- | --- |
| sijms/go-ora#768 | Fetches inline CLOB text as AL16UTF16, for sijms/go-ora#769 | Not applied. Step 9 fixed sijms/go-ora#769 the way the issue says, in its own code |
| sijms/go-ora#766 | Checks the certificate against the original host, for sijms/go-ora#765 | Not needed. Step 4 fixed sijms/go-ora#765 and sijms/go-ora#757 in its own code |
| sijms/go-ora#747 | Accepts named types through `reflect.Kind`, for sijms/go-ora#699 | Not needed. Step 9 fixed sijms/go-ora#699 through `driver.ErrSkip` |
| sijms/go-ora#734 | Stops `putString` writing UTF-16BE into an AL32UTF8 CLOB, for sijms/go-ora#733 | Not applied, though D18 took it. sijms/go-ora#733 does not reproduce: text outside ASCII written through the LOB API reads back whole on 11g, 26ai and `oracle-adb-19c`, because a locator of variable width wants UTF-16, which this code writes. The pull request writes AL32UTF8 there. `TestClobCharset` holds the case |
| sijms/go-ora#721 | Encodes a NULL or large VARRAY and a NULL object, as in `SDO_GEOMETRY` | Written again in step 9 (D18). In `v3` a VARRAY in an object went as its body alone, of any size, and the server read it as a length that it never stopped allocating for, so an insert hung and its session held its lock until the server restarted. Each collection in an object is an image of its own now, with its header and its length in the form of python-oracledb, a NULL one is 0xFF, a nil slice is NULL, and a NULL object reads no bytes it was not sent. A NULL object is the envelope with no image, where an image of 0xFD made a batch store the object of the row before. `TestObjectCollection` holds it on 11g, 26ai and `oracle-adb-26ai`, one by one and in a batch |
| sijms/go-ora#775 | Sends 234 as the logon types byte | Skip. `v2` only, and `v3` already sends 234 |
| sijms/go-ora#770 | Decodes XMLType by its flag bits | Written again in step 9 (D18), as `xmlParameter`, with its own tests |
| sijms/go-ora#773 | Writes a valid replacement for JA16EUC | Written again in step 9 (D18), for every charset: each of the 236 tables names U+FFFD as its replacement, and go-ora wrote its low byte, 0xFD, which is ý in WE8ISO8859P1. A character that a charset lacks is the '?' of that charset, 0x3F in JA16EUC as Oracle writes it, and 0x6F in EBCDIC. `TestEncodeReplacement` holds it |
| sijms/go-ora#760 and sijms/go-ora#761 | Rewrite the column metadata, 1,934 and 2,570 lines | Skip for now. Too large to take before the tests run. Step 9 gives the names of the column types without them |
| sijms/go-ora#616 | Reads `tnsnames.ora` with its own parser, 1,470 lines | Not applied. Step 9 reads `tnsnames.ora` with a parser of its own, through the keys `alias` and `tns_admin` (D20). `TestTNSAlias` connects through an alias |
| sijms/go-ora#743 | Changes the CI of three modules | Skip. The workflow here is new (D7) |
