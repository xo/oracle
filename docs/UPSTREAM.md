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
| sijms/go-ora#686 | `isEqualLoc` is always true | `types/utils.go` compares the zone name and offset. Checked |
| sijms/go-ora#764 | v3.0.1 cannot log in, fixed by commit `1f57ed4` | `1f57ed4` is in this history. Checked |

## Faults to fix

Ordered by priority. P0 loses data or crashes, P1 gives a wrong answer or
blocks a connection, P2 is a fault with a workaround.

| Issue | Priority | Fault | Fix | Status |
| --- | --- | --- | --- | --- |
| sijms/go-ora#767 | P0 | A cancelled transaction can commit. `Rollback` runs under the cancelled context, and `breakConn` is read without the lock | Roll back under `context.WithoutCancel`, and read `breakConn` under the lock | Fixed in step 5. `TestCancelledTransactionDoesNotCommit` fails before and passes after, on 11g, 21c and 26ai |
| sijms/go-ora#776 | P1 | `SELECT NULL a FROM dual` returns no row and no error. sijms/go-ora#755 is the same family in `v2` | Do not end the row loop on a row whose columns are all zero length | Fixed in step 9. A NULL of no type has a zero `MaxLen`, and `BasicRead` read it as a column that sends nothing. It reads the length now, and `Next` fetches when the execute brought no row. `TestUntypedNullRow` holds it, on 26ai |
| sijms/go-ora#769 | P1 | A CLOB over about 8K UTF-16 units with characters outside the BMP reads back as U+FFFD with `LOB FETCH=INLINE` | Fetch inline CLOB text as AL16UTF16. sijms/go-ora#768 does this | Fixed in step 9, as the issue says and without the pull request. An inline CLOB is defined in the national charset when that is AL16UTF16. It reproduced on 11g as well, with the same 4 U+FFFD, and 26ai read it right. `TestClobOutsideBMP` holds it, on 11g and 26ai. A write had the same fault on 11g, which no issue names: a string of over 4000 bytes bound into a CLOB stored U+FFFD for a character that a piece split. On 11g such a string binds as a temporary CLOB, and `TestStrings` and `TestLongColumn` hold it |
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
| sijms/go-ora#729 | P2 | A server that requires native network encryption or checksums refuses the client with ORA-12269 | Implement the negotiation | Open. Ken asked that it be reproduced and fixed (D21) |

## Faults that need a reproduction first

sijms/go-ora#724 and sijms/go-ora#697 (a malformed TTC packet), sijms/go-ora#706
and sijms/go-ora#700 (EOF under load), sijms/go-ora#704 (`CONTAINERS(DUAL)`),
sijms/go-ora#683 (a lower case user name), sijms/go-ora#687, sijms/go-ora#672,
sijms/go-ora#655 and sijms/go-ora#568 (time zones of DATE, TIMESTAMP WITH LOCAL
TIME ZONE and a zero `time.Time`), sijms/go-ora#635 and sijms/go-ora#688
(ORA-12564), sijms/go-ora#680 (a TLS cipher on RDS), sijms/go-ora#615 and
sijms/go-ora#673. Each needs a test against a release that `dbrun` starts
before it is a fault.

## Features

sijms/go-ora#777 (Transparent Application Continuity), sijms/go-ora#707 and
sijms/go-ora#619 (failover), sijms/go-ora#681, sijms/go-ora#559 and
sijms/go-ora#664 (streaming LOBs and rows), sijms/go-ora#666 (scrollable
cursors), sijms/go-ora#611 (skip the NLS query at connect), sijms/go-ora#584
(`v$session.terminal`), sijms/go-ora#560 (`/ as sysdba`), sijms/go-ora#530 (OCI
IAM tokens), sijms/go-ora#640 (pool settings), sijms/go-ora#618 (TimesTen) and
sijms/go-ora#403 (ASM). Ken decided to take #611, #530, #681, #559, #664,
#707 and #619 (D21). The rest wait for Ken.

sijms/go-ora#649 asks for smaller charset tables, which step 8 built. sijms/go-ora#653 (VECTOR) and sijms/go-ora#365 (Advanced Queuing) are in
the `v3` code already, as `types.Vector` and the `aq` package.

## Not a fault in this code

sijms/go-ora#690 is about the `v2` `go.mod`. This module requires nothing.
sijms/go-ora#609 expects `rows.Err` to return `sql.ErrNoRows`, which
`database/sql` returns only from `Row.Scan`. sijms/go-ora#602 is Oracle 8i,
which is older than any release this project tests. sijms/go-ora#685 and
sijms/go-ora#682 are questions.

## Pull requests

None is applied as it is. Ken decided to take sijms/go-ora#734, #721, #770
and #773, each written again for this code (D18). The rest wait for Ken (D5).

| Pull request | What it does | Verdict |
| --- | --- | --- |
| sijms/go-ora#768 | Fetches inline CLOB text as AL16UTF16, for sijms/go-ora#769 | Not applied. Step 9 fixed sijms/go-ora#769 the way the issue says, in its own code |
| sijms/go-ora#766 | Checks the certificate against the original host, for sijms/go-ora#765 | Not needed. Step 4 fixed sijms/go-ora#765 and sijms/go-ora#757 in its own code |
| sijms/go-ora#747 | Accepts named types through `reflect.Kind`, for sijms/go-ora#699 | Not needed. Step 9 fixed sijms/go-ora#699 through `driver.ErrSkip` |
| sijms/go-ora#734 | Stops `putString` writing UTF-16BE into an AL32UTF8 CLOB, for sijms/go-ora#733 | Apply the `v3` half |
| sijms/go-ora#721 | Encodes a NULL or large VARRAY and a NULL object, as in `SDO_GEOMETRY` | Apply after a test against a real server |
| sijms/go-ora#775 | Sends 234 as the logon types byte | Skip. `v2` only, and `v3` already sends 234 |
| sijms/go-ora#770 | Decodes XMLType by its flag bits | Written again in step 9 (D18), as `xmlParameter`, with its own tests |
| sijms/go-ora#773 | Writes a valid replacement for JA16EUC | Rewrite for `v3`, after the charset tables change |
| sijms/go-ora#760 and sijms/go-ora#761 | Rewrite the column metadata, 1,934 and 2,570 lines | Skip for now. Too large to take before the tests run. Step 9 gives the names of the column types without them |
| sijms/go-ora#616 | Reads `tnsnames.ora` with its own parser, 1,470 lines | Not applied. Ken decided that the driver reads `TNS_ADMIN` with a parser of its own (D20) |
| sijms/go-ora#743 | Changes the CI of three modules | Skip. The workflow here is new (D7) |
