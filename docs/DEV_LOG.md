# Dev Log

Хронология. Новые записи — сверху. Каждая: дата, что сделано, файлы,
валидация, риски.

## 2026-05-21 — reference examples expanded

**Сделано.** Добавлены 3 лёгких reference-проекта в `examples/`, чтобы
на тестовом сервере можно было проверить весь функционал flotilla.

| project | GitHub source | что тестирует |
|---------|---------------|---------------|
| `echo-server` | Ealenn/Echo-Server | explicit `https_entrypoint` object form (service+port), healthcheck L006 |
| `filebrowser` | filebrowser/filebrowser | auto-cert shortcut form, volume persistence |
| `miniflux` | miniflux/v2 | manual Traefik labels, multi-service `depends_on` + `service_healthy`, required env vars L007 |

**Файлы.** `examples/{echo-server,filebrowser,miniflux}/{project.yml,compose.yml,.env.example,README.md}`,
`examples/README.md`.

**Валидация.** `gofmt -l` пусто; `go build ./...` ок; `go vet ./...` ok;
`go test ./...` — 13/13 пакетов; `sh -n install.sh` чисто.
Все 5 `examples/` (hello-world, echo-server, filebrowser, miniflux,
crm-prvms) прошли `flotilla deploy --dry` против настоящего docker compose
(hello-world/echo-server/filebrowser — exit 0 с 8/8 lint pass; miniflux
— exit 0 при наличии `.env`; crm-prvms — требует `.env` для L005, как и
ранее).

**Риски.** `crm-prvms` fail L005 без `.env` (известная особенность
examples с env-интерполированными labels, не блокер).

## 2026-05-16 — v0.1.0 MVP собран и проверен

**Сделано.** Полная реализация flotilla v0.1 за 11 «волн» поверх
зафиксированного `ARCHITECTURE.md`: bootstrap репо → Go-скелет →
`project` (парсер+JSON-schema) → `compose`/`envcheck` →
`contract` (L001-L008) → `autocert`/`traefik` → `state`/`acme`/
`discover`/`status`/`deploy` (9-шаговый pipeline) → `cmd/flotilla`
(CLI) → `install.sh` → integration-тесты (build-tag) → dogfood
`examples/` (hello-world + crm-prvms).

**Файлы.** `cmd/flotilla/`, `internal/{project,compose,envcheck,
contract,autocert,traefik,state,acme,discover,status,deploy,log}/`,
`install.sh`, `embed/traefik/`, `examples/`, `tests/integration/`,
инфра (`Makefile`, `.github/workflows/`, `.goreleaser.yml`),
`docs/ARCHITECTURE.md`.

**Решения по ходу (не молча):**
- ARCHITECTURE §5 step 2 исправлен — lint валидирует авторский
  compose.yml, не merged override (иначе L008 ложно срабатывает на
  каждом auto-cert проекте). См. DEC-005.
- CLI flag-ordering: stdlib `flag` останавливается на первом
  позиционном → `flotilla deploy /opt/x --dry` не работал. Добавлен
  `splitArgs` (все флаги булевы → безопасно). Реальный UX-фикс.
- Docker-трогающие вызовы (`compose.Load/Up/PS`) вынесены в
  package-vars-seam → pipeline тестируется без daemon.

**Testbed-фикс (DEC-010).** 3 тестовых проекта (ft-static/ft-echo/
ft-multi, каждый ≤1cpu/1gb) вскрыли баг: `--env-file` передавался в
`docker compose` всегда, даже когда `.env` нет → падение на проектах
без env-переменных. Исправлено в `internal/compose` + `TestEnvFileArgs`
+ CHANGELOG.

**Валидация.** `gofmt -l` пусто; `go build ./...` ок; `go vet ./...`
и `-tags=integration` ок; `go test ./...` — 13/13 пакетов, 117
тест-функций; `sh -n install.sh` + `shellcheck` чисто; оба `examples/`
+ все 3 testbed-проекта прошли `flotilla deploy --dry` против
**настоящего** `docker compose` (exit 0, 8/8 lint-правил pass).

**Риски / границы.** install.sh шаг 8 (скачивание бинаря) заработает
после публикации GitHub Release с тегом `v0.1.0`; до тех пор —
`make build`. Полный Let's Encrypt e2e не покрыт авто-тестами (нужен
публичный домен+DNS) — KNOWN_ISSUES.
