# Integration tests

These tests run against a live Oracle database. Each one reads `ORACLE_DSN`
and skips when it is empty. Start a release with `dbrun`, from a checkout of
`dbmeta` next to this one, and pass the URL that it prints:

```bash
export ORACLE_DSN=$(cd ../dbmeta/test && go run ./cmd/dbrun dsn --json oracle-26ai | jq -r '.[0].url')
go test -count=1 ./integration
```

Each file holds the tests of one area. A test of an upstream fault names the
issue in its doc comment, as `sijms/go-ora#205`. The `python_` files hold the
tests of python-oracledb, written again here (D28).

For an Autonomous Database release, such as `oracle-adb-26ai`, replace the
query of the URL that `dbrun` prints, which names the TLS options of
`go-ora`, with `sslmode=require`.
