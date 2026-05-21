# Dev Log

Хронология. Новые записи — сверху. Каждая: дата, что сделано, файлы,
валидация, риски.

## 2026-05-21 — miniflux: remove fake default from ${DOMAIN}

**Сделано.** `miniflux/compose.yml` использовал
`Host(\`${DOMAIN:-rss.example.com}\`)` как костыль, чтобы L005 не
падал при отсутствии `.env` на локальной машине. Но на сервере с
реальным `.env` linter всё равно видел `rss.example.com` вместо
`miniflux.prvms.ru` и падал с «domain and compose labels have drifted».

**Фикс.** Убран дефолт — теперь `${DOMAIN}` (required). L007 требует
его наличия в `.env.example` → добавлен `DOMAIN=miniflux.example.com`.
На сервере `.env` должен содержать реальный домен.

**Файлы.** `examples/miniflux/{compose.yml,.env.example}`.

## 2026-05-21 — Traefik: Docker API version mismatch

**Сделано.** После всех фиксов `install.sh` Traefik запускался, но
`flotilla deploy` всё ещё падал на `traefik-discover` («no router for
Host(...)»). Логи Traefik: «Error response from daemon: client version
1.24 is too old. Minimum supported API version is 1.40».

**Причина.** Traefik v3.1 внутри контейнера использует Docker SDK,
который по умолчанию шлёт API v1.24. Docker daemon на сервере
(25.0+) требует минимум v1.40 и отклоняет запрос. В результате Docker
provider в Traefik не работает — он не видит ни одного контейнера,
ни одного label'а, ни одного router'а.

**Фикс.** В `embed/traefik/compose.yml` добавлено:
```yaml
environment:
  DOCKER_API_VERSION: "1.54"
```
Это заставляет Docker client внутри Traefik использовать актуальную
версию API. `install.sh` теперь требует Docker 25.0+.

**Файлы.** `embed/traefik/compose.yml`, `CHANGELOG.md`,
`docs/DEV_LOG.md`, `docs/KNOWN_ISSUES.md`, `docs/RELEASE_NOTES.md`.

**Валидация.** `docker compose -f embed/traefik/compose.yml config`
проходит (exit 0), `DOCKER_API_VERSION` виден в выводе.

## 2026-05-21 — install.sh: fail-fast на занятом порту 8080

**Сделано.** После ручного удаления старого Traefik `docker compose up`
в install.sh step 7 упал с ошибкой:
`Bind for 0.0.0.0:8080 failed: port is already allocated`.
Пользователь не понял причину, потому что install.sh step 6 проверял
только порты 80 и 443, а 8080 (Traefik dashboard/API) не проверял.

**Фикс.** В `detect_ingress()` добавлен `:8080->` в grep портов.
Если порт 8080 занят — fail fast с понятным сообщением:
«ports 80/443/8080 are bound by the container(s) above... If port 8080
is in use, Traefik dashboard cannot bind to 127.0.0.1:8080».

**Файлы.** `install.sh`, `docs/ARCHITECTURE.md` §4.1 step 6,
`CHANGELOG.md`, `docs/DEV_LOG.md`, `docs/KNOWN_ISSUES.md`.

**Валидация.** `sh -n install.sh` чисто.

## 2026-05-21 — `make check` включает golangci-lint

**Сделано.** CI упал с `gosimple S1016` (type conversion vs struct
literal), хотя локально `make check` был чистый. Причина: `make
check` запускал fmt+vet+test, а CI ещё и `golangci-lint run`. Разрыв
между локальным workflow и CI — серьёзный дефект процесса.

**Фикс.** `lint` добавлен в target `check` в Makefile (`check: fmt vet
lint test`). `AGENTS.md` validation baseline обновлён: `golangci-lint
run` теперь обязательный шаг локально. CI и локальная проверка
совпадают.

**Файлы.** `Makefile`, `AGENTS.md`, `docs/DEV_LOG.md`,
`docs/KNOWN_ISSUES.md`, `CHANGELOG.md`.

**Валидация.** `make check` (fmt+vet+lint+test) — 13/13 ok; `sh -n
install.sh` чисто.

## 2026-05-21 — install.sh: detect non-flotilla Traefik

**Сделано.** Обнаружена и закрыта проблема: `install.sh` step 6 при
обнаружении существующего Traefik просто «reused» его, даже если это
был чужой Traefik без Docker provider. В результате `flotilla deploy`
pipeline падал на шаге `traefik-discover` (нет router'ов для
`Host(<domain>)`), потому что Traefik не видел labels проектов.

**Фикс.** В `detect_ingress()` добавлена проверка
`docker inspect --format='{{range .Config.Cmd}}{{.}} {{end}}' | grep
providers.docker=true`. Если не найдено — fail fast с понятным
сообщением: «It was not deployed by flotilla and will not discover
project labels. Stop and remove it, then re-run install.sh».

**Файлы.** `install.sh`, `docs/ARCHITECTURE.md` §4.1 step 6,
`CHANGELOG.md`, `docs/RELEASE_NOTES.md`, `docs/KNOWN_ISSUES.md`,
`docs/TASK_STATE.md`.

**Валидация.** `sh -n install.sh` чисто; `make check` (gofmt, vet, test)
13/13 ok.

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
