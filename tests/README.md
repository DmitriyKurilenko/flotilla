# Tests

flotilla follows the Go convention: **unit tests live next to the code
they test** (`internal/<pkg>/<pkg>_test.go`). There is no separate
`tests/unit/` tree.

```
make test          # all unit tests (no Docker, fast)
make cover         # unit tests + HTML coverage report
```

## Integration tests

`tests/integration/` holds tests gated behind the `integration` build
tag. They require a working Docker daemon and are **not** run by
`go test ./...` or `make test`.

```
go test -tags=integration ./tests/integration/...
```

What they cover, and what they deliberately do not, is documented in
the package comment of `deploy_integration_test.go`. The short version:
they drive a real `docker compose up` through the flotilla binary; they
do **not** attempt a real Let's Encrypt round trip (that needs a public
domain + DNS and belongs in a manual/staging job — see
`docs/ARCHITECTURE.md` §11).
