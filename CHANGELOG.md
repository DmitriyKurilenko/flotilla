# Changelog

All notable changes to flotilla are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the project is on `0.y.z` versions, the CLI surface and the
`project.yml` schema are not yet stable — minor bumps may introduce
breaking changes. See [`docs/ARCHITECTURE.md` §10](docs/ARCHITECTURE.md#10-versioning-policy).

## [Unreleased]

### Added
- Three new reference projects in `examples/` (`echo-server`, `filebrowser`,
  `miniflux`) — lightweight real-world projects from GitHub that cover
  the explicit `https_entrypoint` port form, volume persistence, and
  multi-service manual-Traefik-labels deployments. All pass `deploy --dry`.

## [0.1.0] — 2026-05-16

First release. MVP per [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
§12.

### Added
- Repository scaffolding: MIT license, Makefile, CI + release
  workflows, GoReleaser config, embedded Traefik bundle, `AGENTS.md`.
- Design document `docs/ARCHITECTURE.md` (the locked v0.1 design).
- `project.yml` v1: 6-field schema with embedded JSON-Schema
  validation (`internal/project`), `https_entrypoint` string/object
  union.
- CLI (`cmd/flotilla`): `status` and `deploy`, flags `--all`,
  `--keep-going`, `--dry`, `--quiet`, `--json`; path-or-flag order
  tolerated.
- 9-step deploy pipeline (`internal/deploy`): autocert → parse → lint →
  env → symlinks → compose-up → wait-running → traefik-discover →
  smoke; `--dry` stops after env.
- Closed lint rule set L001-L008 (`internal/contract`) with
  positive/negative fixtures for each.
- Auto-cert: generate `compose.override.yml` Traefik labels from
  `https_entrypoint` (`internal/autocert`), port inference from
  expose/ports.
- Read-only Traefik API client and ACME (`acme.json`) expiry reader.
- `compose` wrapper over `docker compose config/up/ps`; `envcheck`
  (`${VAR}` scan + `.env` validation); `discover` (`/opt/*/project.yml`);
  `state` (`.flotilla/state.json`); structured `log`.
- `install.sh`: full VPS bootstrap (Docker, `proxy` network, Traefik
  with ACME, CLI binary), interactive, idempotent, shellcheck-clean.
- `examples/hello-world` (auto-cert) and `examples/crm-prvms`
  (multi-router) — both pass `deploy --dry` against real Docker.
- Integration tests behind the `integration` build tag.

### Fixed
- `compose`: `--env-file` is passed to `docker compose` only when the
  env file actually exists. Previously a project that declares no
  environment variables (and ships no `.env`) failed at `docker compose
  config` with «couldn't find env file». Surfaced by the ft-static /
  ft-multi testbed projects.

### Known limitations
- `install.sh` step 8 (binary download) needs a published GitHub
  Release; until the `v0.1.0` tag exists, build from source
  (`make build`).
- Full Let's Encrypt round-trip is not covered by automated tests
  (needs a public domain + DNS); see `docs/ARCHITECTURE.md` §11.
