# Examples

Reference projects that pass the flotilla contract (lint rules
L001-L008). They double as fixtures for the integration tests.

## `hello-world/`

The simplest case: one service, one domain, **no Traefik labels in
compose**. `project.yml` sets `https_entrypoint: web`, so flotilla
generates the routing labels at deploy time.

```bash
flotilla deploy --dry examples/hello-world   # validate, no docker writes
flotilla deploy      examples/hello-world   # real deploy (needs Traefik)
```

## `echo-server/`

Lightweight **Alpine-based** HTTP echo server from GitHub.
Tests the **explicit `https_entrypoint` object form** (`service` + `port`)
and a compose-native healthcheck.

```bash
flotilla deploy --dry examples/echo-server
flotilla deploy      examples/echo-server
```

## `filebrowser/`

Single-service Go web file manager with a **persistent volume** for its
database. Uses the **shortcut `https_entrypoint` form** (`https_entrypoint: app`).

```bash
flotilla deploy --dry examples/filebrowser
flotilla deploy      examples/filebrowser
```

## `miniflux/`

A real-world **multi-service** project: Postgres + Miniflux RSS reader.
Uses **hand-written Traefik labels** (no `https_entrypoint`), tests
`depends_on` with `condition: service_healthy`, required env vars, and
lint rule L007.

```bash
flotilla deploy --dry examples/miniflux
flotilla deploy      examples/miniflux
```

## `crm-prvms/`

A real-world **multi-router** project (the prvms.crm platform): three
Traefik routers on one host, split by path prefix —

| router       | rule                                              | priority |
| ------------ | ------------------------------------------------- | -------- |
| `crm-api`    | `Host` + `PathPrefix(/api,/admin,/ws)`, `/healthz`| 100      |
| `crm-static` | `Host` + `PathPrefix(/static)`                    | 50       |
| `crm-spa`    | `Host` (catch-all)                                | 1        |

Because it has more than one router on the domain, it does **not** use
`https_entrypoint`; the labels are hand-written in `compose.yml` and
`project.yml` only carries identity + domain + path. This is the
migration target for the legacy
`prvms.crm:vps-deployment/crm_prvms/docker-compose.yml`.

What changed in the migration (each tied to a lint rule):

- file is `compose.yml` at the project root — no symlink-to-`/opt`
  dance (the class of bug that caused the original week-long outage).
- every healthcheck targets `127.0.0.1`, never `localhost` (**L006**).
- every Traefik-enabled service declares `traefik.docker.network=proxy`
  *and* lists `proxy` in `networks:` (**L002**).
- the three routers sharing the host each have an explicit `priority`
  (**L003**).
- no `HEALTHCHECK` in any Dockerfile — kept in compose where it can be
  disabled (**L004**); `celery`/`celery-beat` use `healthcheck.disable`.
- every required `${VAR}` (no `:-default`) is documented in
  `.env.example` (**L007**).

```bash
flotilla deploy --dry examples/crm-prvms
```

> `--dry` still needs Docker available, because the lint step runs
> `docker compose config` to resolve the compose model. It does not
> start any container or write anything. See `tests/README.md`.
