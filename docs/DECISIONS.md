# Архитектурные решения

Журнал решений (ADR-lite). Полное обоснование и актуальная форма — в
[`ARCHITECTURE.md`](ARCHITECTURE.md); здесь — что решили и почему, чтобы
не переоткрывать споры. Код, противоречащий решению, — баг в коде или в
доке; молча не расходиться.

## DEC-001: project.yml несёт только то, чего нет в compose (2026-05-15)
**Контекст:** соблазн описать в project.yml сервисы/kind/hooks/env.
**Решение:** схема v1 — 6 полей (`version`, `name`, `domain`, `path`,
`description?`, `https_entrypoint?`), 4 обязательных. Список сервисов,
healthcheck'и, env-переменные, labels выводятся из `compose.yml`.
**Последствия:** compose — единственный источник истины про рантайм;
project.yml его не дублирует (ARCHITECTURE §3, §3.2).

## DEC-002: `version: 1` скаляром, без apiVersion/kind (2026-05-15)
**Контекст:** k8s-стиль `apiVersion: group/v1` + `kind: Project`.
**Решение:** один вид ресурса → `kind` бессмыслен; группы-домена нет →
`apiVersion` тоже. Оставлен один `version: 1` для миграций схемы.

## DEC-003: install.sh — полный VPS-bootstrap, без флагов (2026-05-15)
**Контекст:** спор «тонкий загрузчик + `flotilla init`» vs «всё в
install.sh».
**Решение:** install.sh ставит Docker, сеть `proxy`, Traefik+ACME и сам
бинарь. Интерактивный (tty обязателен, спрашивает только ACME-email),
без флагов, идемпотентный. Команд `init`/`new` нет — `init` семантически
про инициализацию проекта, не сервера.
**Последствия:** ровно два шага: `curl … | sh`, затем `flotilla deploy`.

## DEC-004: CLI — две команды (2026-05-15)
**Решение:** `status` и `deploy` (+ `--all/--keep-going/--dry/--quiet/
--json`). Отдельной `lint` нет — валидация это `deploy --dry` (pipeline
шаги 0-3). Пользователь сам пишет project.yml/compose.yml, скаффолд-
шаблонов нет.

## DEC-005: lint правит авторский compose.yml, не merged override (2026-05-15)
**Контекст:** §5 step 2 изначально говорил «lint reads compose +
override».
**Решение:** lint валидирует **авторский** compose.yml. Линтить
сгенерированный flotilla override бессмысленно и ломает L008 на каждом
auto-cert проекте. ARCHITECTURE §5 step 2 исправлен.

## DEC-006: lint-правила L001-L008 — закрытый набор (2026-05-15)
**Решение:** новое правило требует задокументированного инцидента в
CHANGELOG, реализацию + позитив/негатив фикстуры, строку в §7. «Best
practice с HN» — не мотивация.

## DEC-007: pure Go для CLI, bash только для install.sh (2026-05-15)
**Решение:** CLI — чистый Go. Bash оправдан только для install.sh (на
свежей VPS Go-бинаря ещё нет). `docker`/`docker compose` — через
`os/exec`, без Docker Engine SDK (тяжёлый граф зависимостей, хрупкий
ABI). Это оркестрация, не «bash-в-Go».

## DEC-008: https_entrypoint только для simple-case (2026-05-15)
**Решение:** одно поле `https_entrypoint` → flotilla генерит Traefik
labels для одного сервиса/одного домена. Multi-router (path-prefix,
несколько сервисов на хосте) остаётся на ручных labels в compose.

## DEC-009: схема — `internal/project/v1.schema.json`, `//go:embed` (2026-05-16)
**Контекст:** go:embed не разрешает `..`; дубль top-level `schemas/` +
embed-копия хуже переноса.
**Решение:** единственный файл схемы в `internal/project/`, встроен в
бинарь. Внешние ссылки — на raw.githubusercontent.com.

## DEC-010: `--env-file` только если файл существует (2026-05-16)
**Контекст:** testbed-проекты без env-переменных (ft-static, ft-multi)
без `.env` падали на `docker compose config`: «couldn't find env file».
**Решение:** `compose.envFileArgs` добавляет `--env-file` лишь когда
файл реально есть; compose-дефолтный поиск `.env` остаётся. Покрыто
`TestEnvFileArgs`.
