# AGENTS: protocol for this repository

Operating protocol for AI/code agents working in flotilla. Goal: no
regressions, no lost context between sessions.

flotilla is a **single Go binary + one bash installer** (not a
Django/Docker app — there is no `docker compose` dev stack here).
The sources of truth are short and specific; read them, don't guess.

## Mandatory read order before any code change

1. [`docs/TASK_STATE.md`](docs/TASK_STATE.md) — what's done / in
   progress / next.
2. [`docs/DECISIONS.md`](docs/DECISIONS.md) — DEC-NNN: what was decided
   and why (do not re-litigate).
3. [`docs/KNOWN_ISSUES.md`](docs/KNOWN_ISSUES.md) — open/closed.
4. [`docs/DEV_LOG.md`](docs/DEV_LOG.md) — newest entries first.
5. [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — the locked design;
   the full rationale lives here. Code that contradicts it is a bug in
   the code **or** the doc — fix one, never silently diverge.

Also: [`CONTRIBUTING.md`](CONTRIBUTING.md) (commit format, lint-rule
procedure), [`CHANGELOG.md`](CHANGELOG.md). If instructions conflict,
stop and ask — do not guess.

## Invariants (don't break without updating ARCHITECTURE.md + a DEC)

- **`project.yml` schema** v1: 6 fields, 4 required (§3, DEC-001/002).
  Changes ⇒ edit `internal/project/v1.schema.json`, bump `version`,
  update §3, add CHANGELOG entry.
- **Lint rules L001-L008 are a closed set** (§7, DEC-006). A new rule
  needs a documented incident in `CHANGELOG.md`, the rule +
  positive/negative fixtures, and a §7 row.
- **compose is the runtime source of truth; `project.yml` carries only
  what compose can't express** (§3.2, DEC-001). No fields that
  duplicate compose.
- **Lint validates the authored `compose.yml`, never the autocert
  override** (§5 step 2, DEC-005).
- **Pure Go for the CLI; bash only for `install.sh`** (§8.1, DEC-007).
  No embedded bash, no Docker SDK — `docker`/`docker compose` via
  `os/exec`.
- **`install.sh`** is the whole VPS bootstrap, interactive, no flags;
  there is no `flotilla init`/`new` (DEC-003/004).
- **No `git commit` by an agent.** Stage, draft the message, validate,
  stop. The human commits and tags.

## Update ritual (required after a non-trivial change)

1. Add/adjust a `DEC-NNN` in `docs/DECISIONS.md` if a behaviour or
   invariant changed.
2. Update `docs/TASK_STATE.md` (done / in progress / blocked).
3. Add a concise `docs/DEV_LOG.md` entry: date, files, validation,
   risks.
4. Update `docs/KNOWN_ISSUES.md` if a bug was found or closed.
5. Add a `CHANGELOG.md` entry under `[Unreleased]`
   (Added/Changed/Fixed/Deprecated/Removed).
6. If the change is operator-visible, add a `docs/RELEASE_NOTES.md`
   item: **Russian, no technical detail, operator's perspective**,
   grouped by date then «Новое» / «Улучшения» / «Исправления», one
   sentence per item.
7. If schema or a lint rule changed, also touch
   `internal/project/v1.schema.json` and §3/§7 in the same change.

## Validation baseline (no Docker daemon needed for unit work)

Go 1.22+/1.23 (locally, or `golang:1.23-alpine` with the module cache
mounted). Required tooling:

```
gofmt -l .            # must be empty
go build ./...
go vet ./... && go vet -tags=integration ./...
go test ./...         # every package must be ok
golangci-lint run     # same linters as CI; must be installed locally
sh -n install.sh && shellcheck install.sh
```

`make check` = fmt+vet+lint+test. Integration tests are build-tagged and
need real Docker:

```
go test -tags=integration ./tests/integration/...
```

End-to-end sanity that actually exercises `docker compose`: cross-build
the binary and run `flotilla deploy --dry examples/<name>` on a host
with Docker — both `examples/` must exit 0 with all 8 lint rules
passing. Partial validation is not "done".

## Release

Per `CONTRIBUTING.md`/`docs/VERSIONING.md`: bump `VERSION`, move
`[Unreleased]` → `[X.Y.Z] — date` in `CHANGELOG.md`, add the
`RELEASE_NOTES.md` block, then the human commits and tags `vX.Y.Z`
(GoReleaser builds the four target binaries on the tag).
