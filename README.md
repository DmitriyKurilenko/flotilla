# flotilla

> Thin orchestrator on top of Docker Compose for shared-VPS multi-project
> deployments.

`flotilla` bootstraps a fresh VPS (Docker, Traefik with Let's Encrypt, the
`proxy` network) in a single interactive `install.sh` run. After that,
every project deploys with `flotilla deploy`.

The design is documented in [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).
That document is the source of truth for any disagreement; this README is
only the operator-facing summary.

## Status

**Pre-MVP.** v0.1 is in active development. The CLI surface is intentionally
small (two commands) and may still change in minor versions. Do not depend
on internals.

## Install

On a fresh Debian or Ubuntu VPS, as root:

```bash
curl -fsSL https://raw.githubusercontent.com/DmitriyKurilenko/flotilla/main/install.sh | sh
```

The installer prompts once for the email Let's Encrypt should use for
expiry notifications, then sets up Docker, Traefik, and `flotilla` on
`/usr/local/bin/`. It requires a terminal — there are no flags and no
non-interactive mode.

Re-running the installer on a server that already has flotilla is
idempotent: it upgrades the binary and re-verifies the ingress, never
destroys existing project state.

## Deploy a project

Once `install.sh` has finished:

```bash
flotilla deploy /opt/my-project
```

Where `/opt/my-project` is a directory that contains a `project.yml`
and a `compose.yml`. The minimum `project.yml`:

```yaml
version: 1
name: my-project
domain: my-project.example.com
path: /opt/my-project
```

If you also set `https_entrypoint: <compose-service>` in `project.yml`,
flotilla generates the Traefik labels for that service automatically and
you get HTTPS without ever editing `compose.yml`.

Full schema and semantics: [`docs/ARCHITECTURE.md` §3](docs/ARCHITECTURE.md#3-the-contract-projectyml).

## CLI

```
flotilla status [path] [--all]
flotilla deploy [path] [--all] [--keep-going] [--dry]
```

`--all` discovers every `project.yml` under `/opt/*/project.yml`.
`--dry` (on `deploy`) runs validation only — useful in CI before merging
PRs that touch `project.yml` or `compose.yml`.

## Building from source

```bash
git clone https://github.com/DmitriyKurilenko/flotilla.git
cd flotilla
make build              # produces bin/flotilla
make test               # unit tests
make check              # fmt + vet + test
```

Go 1.22+ required.

## License

[MIT](LICENSE).
