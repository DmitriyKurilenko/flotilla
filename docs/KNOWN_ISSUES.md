# Известные проблемы

## Открытые

1. **install.sh шаг 8 требует опубликованного GitHub Release.**
   Код уже на GitHub; Traefik bundle (step 7) скачивается. Шаг 8
   падает потому что `releases/latest` не найден — тег `v0.1.0` ещё
   не запушен, GoReleaser не собирал артефакты.
   - Временный workaround: `make build` локально + `scp bin/flotilla root@vps:/usr/local/bin/`
   - Закрытие: `git tag v0.1.0 && git push --tags` → release.yml соберёт
     4 бинаря + checksums.txt, и `curl … | sh` пройдёт end-to-end.

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

- **Traefik: Docker API v1.24 rejected by modern daemon.**
  Traefik v3.1 внутри контейнера использует Docker SDK с API v1.24
  по умолчанию. Docker daemon 25.0+ требует минимум v1.40 и
  отклоняет запросы. В результате Docker provider в Traefik не
  работает — не видит labels, не создаёт routers.
  `flotilla deploy` падает на `traefik-discover` («no router for
  Host(...)»). Логи Traefik: «client version 1.24 is too old».
  Закрыто: `DOCKER_API_VERSION=1.40` добавлено в
  `embed/traefik/compose.yml`. 2026-05-21.
- **install.sh step 6: порт 8080 не проверялся, падало на step 7.**
  `docker compose up` для Traefik падало с «Bind for 0.0.0.0:8080
  failed: port is already allocated», потому что step 6 проверял
  только 80/443. Закрыто: добавлен `:8080->` в `detect_ingress()`,
  fail fast с понятным сообщением. 2026-05-21.
- **install.sh step 6: pre-existing Traefik без Docker provider**
  молча reused, и `flotilla deploy` падал на `traefik-discover`
  («no router for Host(...)»). Закрыто: добавлена проверка
  `providers.docker=true` в `detect_ingress()`, fail fast с понятным
  сообщением. 2026-05-21.
- **Расхождение локальной проверки и CI: `make check` не запускал
  `golangci-lint`.** CI падал с `gosimple S1016`, хотя локально всё
  было зелёным. Закрыто: `lint` добавлен в target `check` в Makefile,
  `AGENTS.md` validation baseline обновлён. 2026-05-21.
- **`--env-file` форсился даже при отсутствии `.env`** → `docker
  compose config` падал «couldn't find env file» на проектах без
  env-переменных. Закрыто DEC-010 (`compose.envFileArgs` +
  `TestEnvFileArgs`), 2026-05-16.
- **CLI: флаги после позиционного пути игнорировались** (stdlib `flag`
  останавливается на первом не-флаге). Закрыто `splitArgs` в
  `cmd/flotilla`, 2026-05-16.
