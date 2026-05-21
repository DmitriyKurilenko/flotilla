# Task State

## Текущие задачи
| # | Задача | Статус | Заметки |
|---|--------|--------|---------|
| 1 | v0.1 MVP: дизайн (ARCHITECTURE.md) | done | Схема v1, pipeline, L001-L008, install.sh-контракт зафиксированы |
| 2 | Реализация v0.1 (11 волн) | done | cmd/flotilla + internal/* + install.sh + examples + integration-тесты |
| 3 | Dogfood examples | done | hello-world + crm-prvms проходят `deploy --dry` на реальном docker |
| 4 | Testbed (ft-static/ft-echo/ft-multi) | done | ≤1cpu/1gb каждый; вскрыли и закрыли DEC-010 |
| 5 | Доки актуализированы под flotilla | done | DECISIONS/DEV_LOG/KNOWN_ISSUES/TASK_STATE/RELEASE_NOTES/VERSIONING/AGENTS |
| 6 | Init-commit v0.1.0 подготовлен | done | staged + доки актуализированы под новые examples |
| 7 | Тег `v0.1.0` + GitHub Release | in progress | код на GitHub; нужен `git tag v0.1.0 && git push --tags` → GoReleaser соберёт 4 бинаря |
| 8 | Прод-обкатка testbed на VPS | in progress | обнаружены и закрыты: pre-existing Traefik без Docker provider; порт 8080 не проверялся в step 6; `make check` не запускал `golangci-lint`; Docker API version mismatch (Traefik SDK v1.24 vs daemon v1.40+). Сейчас на сервере нужно пересоздать Traefik с обновлённым compose.yml |
| 9 | Обкатка 5 examples на сервере | pending | после корректного install.sh — deploy hello-world, echo-server, filebrowser, miniflux, crm-prvms |

## Следующее
1. Человек ревьюит staged-набор и делает init-commit (сообщение — ниже в диалоге / по VERSIONING.md формату).
2. `git tag v0.1.0 && git push --tags` — GoReleaser соберёт 4 бинаря, install.sh шаг 8 заработает (закрывает KNOWN_ISSUES #1).
3. Обкатка 3 testbed-проектов на сервере, наблюдение `flotilla status --all`.
4. По итогам обкатки — фидбэк в DEV_LOG, при необходимости патч → v0.1.1.

## v0.2 (отложено, ARCHITECTURE §12)
`flotilla logs`, `restart-traefik`, `--host` (SSH), конфигурируемый
`smoke:`, конфигурируемые discovery-paths для `--all`.
