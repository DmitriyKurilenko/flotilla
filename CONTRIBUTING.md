# Contributing to flotilla

## Before you start

Read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) end-to-end. The
design is locked, and any deviation must land there first — code that
contradicts the architecture document is a bug in either the code or
the doc, but never silently.

## Local setup

You need Go 1.22+, `make`, and `golangci-lint` (same linters as CI).

```bash
git clone https://github.com/DmitriyKurilenko/flotilla.git
cd flotilla

# Install golangci-lint (once) — must be in $PATH
# https://golangci-lint.run/usage/install/#local-installation
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.64.8

make build              # produces bin/flotilla
make test               # unit tests
make check              # fmt + vet + lint + test (pre-commit suite)
```

For integration tests (slow, needs Docker):

```bash
go test -tags=integration ./tests/integration/...
```

## Commit conventions

Conventional Commits, prefixed with the project version from `VERSION`:

```
0.1.0: feat(cli): add --dry flag to deploy
0.1.1: fix(autocert): emit explicit traefik.docker.network label
```

Types: `feat`, `fix`, `refactor`, `docs`, `chore`, `test`, `build`,
`perf`. Scopes mirror the `internal/<package>` directories.

## Pull requests

- **One concern per PR.** Big PRs get split.
- The PR description must reference the
  [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) section the change
  implements (or amends).
- **Coverage must not drop** on `internal/`.
- **`make check` must pass** on your branch.
- If you change the `project.yml` schema or any lint rule (L001-L008),
  update `docs/ARCHITECTURE.md`, `internal/project/v1.schema.json`, and
  `CHANGELOG.md` in the same PR.

## Adding a lint rule (L00X)

The list in `docs/ARCHITECTURE.md` §7 is **closed** for v0.1. Each new
rule needs:

1. A documented incident in `CHANGELOG.md` motivating it.
2. The rule definition in `internal/contract/`.
3. Positive and negative test fixtures under
   `internal/contract/testdata/<L00X>/`.
4. A row added to the table in `docs/ARCHITECTURE.md` §7.

«Best practice we read about on Hacker News» is not a valid motivation.

## Release process

1. Update `VERSION` to the new version.
2. Update `CHANGELOG.md` — move «Unreleased» entries to a new
   `[X.Y.Z] — YYYY-MM-DD` section.
3. Commit, then tag: `git tag vX.Y.Z && git push --tags`.
4. The release workflow (`.github/workflows/release.yml`) runs
   GoReleaser, which produces binaries for the four supported targets
   and attaches them to the GitHub Release.
