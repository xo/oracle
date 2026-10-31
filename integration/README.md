# Integration tests

These tests run against a live Oracle database. Each one reads `ORACLE_DSN`
and skips when it is empty. Start a release with `dbrun`, from a checkout of
`dbmeta` next to this one, and pass the URL that it prints:

```bash
export ORACLE_DSN=$(cd ../dbmeta/test && go run ./cmd/dbrun dsn --json oracle-26ai | jq -r '.[0].url')
go test -count=1 ./integration
```

Many files are named for the upstream issue of `sijms/go-ora` that they hold.
