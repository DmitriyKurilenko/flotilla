# Changelog

All notable changes to flotilla are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the project is on `0.y.z` versions, the CLI surface and the
`project.yml` schema are not yet stable — minor bumps may introduce
breaking changes. See [`docs/ARCHITECTURE.md` §10](docs/ARCHITECTURE.md#10-versioning-policy).

## [0.1.0] — 2026-05-21

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
- `examples/echo-server`, `examples/filebrowser`, `examples/miniflux` —
  lightweight real-world projects from GitHub covering explicit
  `https_entrypoint` port form, volume persistence, and multi-service
  manual-Traefik-labels deployments. All pass `deploy --dry`.
- Integration tests behind the `integration` build tag.

### Fixed
- `compose`: `--env-file` is passed to `docker compose` only when the
  env file actually exists. Previously a project that declares no
  environment variables (and ships no `.env`) failed at `docker compose
  config` with «couldn't find env file». Surfaced by the ft-static /
  ft-multi testbed projects.

### Fixed
- `install.sh` step 6 now **auto-removes** a pre-existing Traefik
  container that lacks `--providers.docker=true`, instead of failing
  fast. A pre-existing Traefik without Docker provider silently
  ignored flotilla project labels, causing `traefik-discover` to fail
  on every deploy. Now `install.sh` stops and removes the offender
  automatically, then deploys a flotilla-compatible Traefik.
- `install.sh` step 6 now **auto-removes** Docker containers blocking
  ports 80/443/8080, instead of failing fast. If another container
  occupies 8080, Traefik's dashboard binding to `127.0.0.1:8080`
  causes a cryptic Docker networking error during `docker compose up`.
  Now `install.sh` removes the conflicting containers automatically and
  proceeds.
- `Makefile`: `make check` now includes `golangci-lint run` (target
  `lint`). Previously CI ran `golangci-lint` but local `make check`
  did not, causing CI-only lint failures (e.g. `gosimple S1016`). The
  local validation baseline now matches CI exactly.
- `embed/traefik/compose.yml`: updated image from `traefik:v3.1` to
  `traefik:v3.7` and added `DOCKER_API_VERSION=1.54`. Modern Docker
  daemons (25.0+) reject the legacy API v1.24 that Traefik v3.1's
  embedded Docker SDK sends by default, causing the provider to fail
  with «client version 1.24 is too old». This made Traefik ignore all
  project labels, so `traefik-discover` found zero routers. The newer
  Traefik image uses an updated SDK. `install.sh` now requires Docker
  25.0+.

### Known limitations
- `install.sh` step 8 (binary download) needs a published GitHub
  Release; until the `v0.1.0` tag exists, build from source
  (`make build`).
- Full Let's Encrypt round-trip is not covered by automated tests
  (needs a public domain + DNS); see `docs/ARCHITECTURE.md` §11.
