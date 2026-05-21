# Известные проблемы

## Открытые

1. **install.sh шаг 8 требует опубликованного GitHub Release.**
   Скачивание бинаря идёт с `releases/latest`. Пока нет тега `v0.1.0`
   и собранных GoReleaser-артефактов — собирать из исходников
   (`make build`) и копировать бинарь на VPS вручную.
   - Закрытие: `git tag v0.1.0 && git push --tags` → release.yml.

2. **Полный Let's Encrypt round-trip не покрыт авто-тестами.**
   Integration-тесты доводят пайплайн до `traefik-discover` на реальном
   `docker compose`, но реальную выдачу ACME-сертификата не проверяют
   (нужен публичный домен + DNS). ARCHITECTURE §11.
   - Закрытие: ручной/staging-job на домене с DNS → VPS.

3. **`--all` сканирует только `/opt/*/project.yml`.**
   Путь захардкожен (`discover.DefaultGlob`). Конфигурируемые
   search-paths отложены в v0.2 (ARCHITECTURE §12, §13).

4. **`traefik-discover` зависит от Traefik API на `127.0.0.1:8080`.**
   `embed/traefik/compose.yml` биндит именно это. Если на сервере уже
   крутится свой Traefik без доступного API на 8080 — шаг 7 не пройдёт.
   На свежей VPS через install.sh проблемы нет.

## Закрытые

- **`--env-file` форсился даже при отсутствии `.env`** → `docker
  compose config` падал «couldn't find env file» на проектах без
  env-переменных. Закрыто DEC-010 (`compose.envFileArgs` +
  `TestEnvFileArgs`), 2026-05-16.
- **CLI: флаги после позиционного пути игнорировались** (stdlib `flag`
  останавливается на первом не-флаге). Закрыто `splitArgs` в
  `cmd/flotilla`, 2026-05-16.
