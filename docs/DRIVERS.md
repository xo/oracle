# Drivers

This document compares the features of this driver with the other Oracle
drivers, so that a later decision to add a feature can start here. It was
checked on 2026-11-01. A row says what each driver offers, and not what this
driver will offer. A feature comes in only through a decision in `PLAN.md`.

The columns are:

- **xo**: this driver, from its code, updated on 2026-11-02.
- **Py-T** and **Py-K**: python-oracledb, thin and thick, v26.1.0.
- **Nd-T** and **Nd-K**: node-oracledb, thin and thick, v26.0.0.
- **JDBC**: the Oracle JDBC thin driver of 23ai and 26ai.
- **ODP**: ODP.NET Core and Managed, 23ai and 26ai.
- **godror**: v0.51.5, over ODPI-C.
- **rust**: rust-oracle v0.6.3, over ODPI-C.
- **go-ora**: sijms/go-ora v2. Its repository also ships v3, which this driver
  started from.

A cell is "yes", "no", "partial" with a note, or "?" where nobody checked it.
"yes (client)" is a feature that the Oracle Client libraries give, through
OCI, `sqlnet.ora` and `tnsnames.ora`, and not the driver. The thick drivers,
godror and rust get those, and for godror and rust that is inferred from
their use of OCI. A note marked "(k)" comes from what the checker knew and
was not checked again.

## Sources

The thin and thick columns of python-oracledb and node-oracledb come from
appendix A of the
[python-oracledb](https://python-oracledb.readthedocs.io/en/latest/user_guide/appendix_a.html)
and
[node-oracledb](https://node-oracledb.readthedocs.io/en/latest/user_guide/appendix_a.html)
user guides. JDBC comes from the table of contents of the
[26ai JDBC Developer's Guide](https://docs.oracle.com/en/database/oracle/oracle-database/26/jjdbc/toc.htm).
ODP.NET comes from its
[release changes](https://docs.oracle.com/en/database/oracle/oracle-database/26/odpnt/release_changes.html)
and its
[differences between the drivers](https://docs.oracle.com/en/database/oracle/oracle-database/23/odpnt/intro004.html).
The columns of godror, rust-oracle and go-ora come from their source, read
through `gh api`. The column of this driver comes from its code.

## Connection

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| Easy Connect Plus | partial (1) | yes (2) | yes | yes (2) | yes | yes | yes | yes | yes (client) | partial (1) |
| TNS descriptor | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| `tnsnames.ora` | yes (D20) | yes | yes | yes | yes | yes | yes | yes | yes (client) | no |
| LDAP naming | no | partial (hook) | yes | no | yes | yes | ? | yes (client) | yes (client) | no |
| Wallet | yes (SSO, PKCS#12) | partial (PEM) | yes | partial (PEM) | yes | yes | yes | yes (client) | yes (client) | yes |
| TLS and mutual TLS | yes | yes | yes | yes | yes | yes | yes | yes (client) | yes (client) | yes |
| Native network encryption and checksums | yes (AES, SHA) | no | yes | no | yes | yes | yes | yes (client) | yes (client) | yes |
| Kerberos | yes | no | yes | no | yes | yes | yes | yes (client) | yes (client) | yes |
| RADIUS | no | no | yes | no | yes | yes | no | yes (client) | yes (client) | no |
| OCI IAM token | yes (`auth=token`) | yes | yes | yes | yes | yes | yes | yes | ? | no |
| Entra ID (Azure AD) token | no | yes | yes | yes | yes | yes | yes | partial | ? | no |
| External and OS logon | partial (`auth=os`, NTS) | no | yes | no | yes | partial (k) | ? | yes | yes | partial (NTS) |
| Proxy logon | yes (`proxy_user`) | yes | yes | yes | yes | yes | yes | yes (client) | yes (client) | yes |
| DRCP | no | yes | yes | yes | yes | yes | yes | yes | yes | ? |
| Pool in the driver | no (`database/sql`) | partial | yes | partial | yes | yes (UCP) | yes | yes | yes | no |
| Application Continuity, TAC | no | no | yes | no | yes | yes | yes | yes (client) | yes (client) | no |
| Fast Application Notification | no | no | yes | no | yes | yes | yes | partial | partial | no |
| RAC runtime load balancing | no | no | yes | no | yes | yes | yes | yes (client) | yes (client) | ? |
| Failover over addresses | partial (3) | yes | yes | yes | yes | yes | yes | yes (client) | yes (client) | partial |
| Connect timeout | yes | yes | yes | yes | yes | yes | yes | yes | yes (client) | yes |
| Keepalive (`EXPIRE_TIME`) | no | yes | yes | yes | yes | yes (k) | yes | yes (client) | yes (client) | no |
| Statement cache | no | yes | yes | yes | yes | yes | yes | yes | yes | no |
| Session tagging | no | no | yes | no | yes | yes | ? | no | yes | no |
| Sharding keys | no | no | yes | no | yes | yes | ? | yes | ? | no |
| Edition-based redefinition | partial (session parameter) | yes | yes | yes | yes | yes (k) | ? | yes | yes | partial |
| Module, action, client ID, client info | no (4) | yes | yes | yes | yes | yes (k) | yes (k) | yes | yes | partial |
| Call timeout | yes (context, `WithTimeout`) | yes | yes | yes | yes | partial | yes | yes | yes | partial |
| Cancel a call | yes (context) | yes | yes | yes | yes | yes | yes | yes | yes | yes |

1. This driver and go-ora take a URL DSN. This driver reads an Easy Connect
   string as a value of `tnsnames.ora`, and takes no Easy Connect Plus
   options.
2. The thin drivers ignore the Easy Connect options that they do not know, and
   read no `sqlnet.ora`.
3. A DSN can name more addresses, and a failed handshake tries the next one
   (sijms/go-ora#619 and #707). The descriptor keys `RETRY_COUNT` and
   `RETRY_DELAY` are read by no code.
4. The logon names the program and the machine. Nothing sets the module or
   the action.

The thin drivers of python-oracledb and node-oracledb connect to 12.1 and
later. This driver and go-ora connect to 11g as well.

## Statements

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| Prefetch and array size | yes (`prefetch`, sized by LOBs) | yes | yes | yes | yes | yes | yes | yes | yes | partial |
| Scrollable cursors | no | yes | yes | no | no | yes | no (k) | no | no | no |
| Implicit results | partial (`NextResultSet`) | yes | yes | yes | yes | yes | ? | yes | yes | partial |
| REF CURSOR | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| Array DML | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| Batch errors | yes (D34) | yes | yes | yes | yes | partial | ? | yes | yes | no |
| Row count of each row | yes (D34) | yes | yes | yes | yes | yes | partial | no | yes | no |
| DML RETURNING | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| PL/SQL associative arrays | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| PL/SQL BOOLEAN | yes | yes | yes | yes | yes | yes | yes | yes | yes | partial |
| PL/SQL records | yes (D33) | yes | yes | yes | yes | yes (k) | ? | yes | yes | ? |
| Close piggybacked on the next call | no | yes | yes | yes | yes | yes (k) | ? | yes | yes | no |
| Network compression | no | no | yes (client) | ? | yes (client) | yes (k) | ? | yes (client) | yes (client) | no |

Python thin and node thin send the close of a cursor with the next call, as
`constants.pxi` and `execute.js` show.

## Types

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| NUMBER as an exact decimal | yes (`apd.Decimal`) | yes | yes | partial (string) | partial | yes | yes | yes (string) | yes | partial |
| BINARY_FLOAT and BINARY_DOUBLE | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| DATE, TIMESTAMP, TZ, LTZ | yes | yes | yes | yes | yes | yes | yes | partial | yes | yes |
| INTERVAL YM and DS | yes | yes | yes | yes | yes | yes | yes | partial | yes | ? |
| Temporary LOBs, pieces, inline fetch | yes (D22) | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| BFILE | yes (read) | partial | partial | yes | yes | yes | yes | ? | partial | yes |
| LONG and LONG RAW | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| ROWID and UROWID | yes | yes | partial | yes | yes | yes | yes | yes | yes | yes |
| XMLType | yes (as text) | yes | partial | yes | partial | yes | yes | ? | no | ? |
| JSON and OSON | yes | yes | yes | yes | yes | yes | yes | yes | no | yes |
| VECTOR, dense and sparse | yes | yes | yes | yes | yes | yes | yes | yes | yes | partial (dense) |
| BOOLEAN of 23ai | yes | yes | yes | yes | yes | yes (k) | yes | yes | yes | yes |
| Objects, VARRAY, nested tables | yes | yes | yes | yes | yes | yes | yes | yes | partial | yes |
| REF | no | no | yes | ? | ? | yes | partial | ? | ? | no |
| ANYDATA | no | ? | ? | ? | ? | yes | ? | ? | ? | ? |
| Domains and annotations of 23ai | partial (names) | yes (k) | yes (k) | yes (k) | yes (k) | ? | ? | ? | ? | ? |

## Transactions

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| Autocommit | yes | yes | yes | yes | yes | yes | yes | yes | yes | yes |
| Isolation levels | yes (read committed, serializable, read only) | partial (SQL) | partial | partial | partial | yes | yes | ? | no | no |
| Savepoints | SQL | SQL | SQL | SQL | SQL | yes | yes | SQL | SQL | SQL |
| Two-phase commit (XA) | no | yes | yes | yes | yes | yes | partial | no | no | no |
| Sessionless transactions of 23ai | no | yes | yes | yes | yes | yes | yes | no | no | no |

Ken decided on 2026-11-01 not to build two-phase commit or sessionless
transactions, since no caller needs them and neither fits `database/sql`.

## Messages and events

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| Advanced Queuing | yes (`aq`) | yes | yes | yes | yes | yes | yes | yes | partial | partial |
| AQ payloads RAW, object, JSON | ? | yes (k) | yes | yes | yes | yes | yes | partial | partial | ? |
| Array enqueue and dequeue | ? | yes | yes | yes | yes | ? | yes | yes | ? | ? |
| Transactional Event Queues | ? | yes | yes | yes | yes | yes | yes | yes (client) | ? | ? |
| Continuous Query Notification | no | yes | yes | no | yes | yes | yes | yes | no | no |
| `DBMS_OUTPUT` | yes (D19) | no | no | no | no | no | no | yes | no | yes |

## Other

| Feature | xo | Py-T | Py-K | Nd-T | Nd-K | JDBC | ODP | godror | rust | go-ora |
|---|---|---|---|---|---|---|---|---|---|---|
| SODA | no | no | yes | no | yes | yes (k) | no | no | no | no |
| Direct path load | no | yes | no | yes | no | partial | yes | no | no | yes |
| Data frames and Arrow | no | yes | yes | no | no | no | no | no | no | no |
| Pipelining of 23ai | no | yes | no | yes | no | yes | yes | no | no | no |
| OpenTelemetry | no (`slog`, D14) | yes | yes | yes | yes | partial | yes | no | no | no |
| Startup and shutdown | no | no | yes | no | yes | yes (k) | no | yes | yes | no |
| Servers | 11g to 26ai, tested | 12.1+ | 11.2+ | 12.1+ | 11.2+ | 19c+ | ? | the client's | the client's | 10.2+ |

## What the comparison shows

1. JDBC thin and ODP.NET Managed build native network encryption, Kerberos,
   RADIUS, Application Continuity and FAN in protocol code with no Oracle
   Client. So each of these can be built over TNS and TTC here too.
2. The thin drivers of Oracle have these, and this driver lacks them:
   statement caching, DRCP, pipelining, direct path load, keepalive, the
   tracing attributes, and Entra ID tokens.
3. The features that a `database/sql` driver can add with no new API are
   statement caching, keepalive, the tracing attributes as connector options,
   and `RETRY_COUNT` with `RETRY_DELAY`. Pipelining needs an API of its own.
   Batch errors and the count of each row took the options of D34.

## Not checked

The ODP.NET column is not checked for LDAP, external logon, sharding,
editions, session tagging, implicit results, batch errors, PL/SQL records and
servers. ANYDATA, the metadata of domains, and REF are not checked outside
JDBC. No feature of godror or rust-oracle that comes from the client was
tested.
