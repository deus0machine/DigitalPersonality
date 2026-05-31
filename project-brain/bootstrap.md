# Bootstrap — Digital Personality

Прочитай этот файл в начале каждой сессии, затем `current_state.md`.

## Что это за проект

Go-система для построения цифровой личности на основе Telegram-данных.
Синхронизирует сообщения через MTProto → строит многоуровневую память →
будет симулировать стиль общения через LLM.

## Быстрый старт сессии

1. Прочитай `project-brain/current_state.md` — что реализовано, какая архитектура.
2. Прочитай `project-brain/roadmap.md` — где мы на карте.
3. Если задача связана с конкретной подсистемой — найди её ADR в `project-brain/adr/`.
4. Если нужно понять инварианты — `project-brain/invariants.md`.
5. Если задача может задеть известные проблемы — `project-brain/known_issues.md`.

## Ключевые факты

- **Go module**: `github.com/digital-personality`, Go 1.25+
- **MTProto**: `github.com/gotd/td v0.144.0`
- **БД**: PostgreSQL 16 + pgvector, драйвер `pgx/v5`
- **Миграции**: `golang-migrate`, папка `migrations/`
- **Env**: `godotenv` читает `.env` при запуске, caarlos0/env парсит переменные
- **Структура**: `cmd/` → `internal/{domain,application,infrastructure,interfaces}`
- **Запуск sync**: `docker compose up -d && go run ./cmd/server`
- **CLI inspect**: `go run ./cmd/server search "текст"` (без Telegram creds)
- **Все команды**: `search`, `episodes`, `similar`, `personality [id]`, `chats`, `windows [id]`

## Memory Window Architecture (Phase 4.10)

Группы и каналы содержат много шума — только сообщения около точек участия нужны.
Решение: `in_memory_window BOOLEAN DEFAULT TRUE` на `messages`.

- **social / passive_consumption** → windowed: только ±10 сообщений вокруг исходящих anchor-ов
- **interpersonal / self_expression / tool_interaction** → full-sync: флаг всегда TRUE
- После sync: `WindowExpander.ComputeAndRebuild` → retroactive Layer 2-3 rebuild
- SQL validation toolkit: `docs/sql/inspect_windows.sql`
- CLI валидация: `go run ./cmd/server windows` / `windows <chat-id>`

## Архитектурный принцип

Зависимости только внутрь: infrastructure → application → domain.
Telegram-типы (gotd/td) не выходят за границу `internal/infrastructure/telegram/`.
Всё что пересекает границы — через порты (`internal/application/port/`).

## Файлы, которые чаще всего меняются

- `internal/app/app.go` — точка сборки всех зависимостей
- `internal/application/sync/engine.go` — оркестрация backfill pipeline
- `internal/interfaces/cli/` — CLI команды и форматирование
- `migrations/` — каждое изменение схемы = новый файл
- `project-brain/current_state.md` — обновлять после каждой крупной фазы

## Критические ограничения

- Никогда не логировать session-данные Telegram, API ключи, тексты сообщений
- `OPENAI_API_KEY` не `required` до Phase 5
- Embedding-инфраструктура не реализована — не интегрировать LLM до Phase 5
- CLI команды требуют только `POSTGRES_PASSWORD` (используют `config.LoadCLI()`)
- Telegram credentials (`TELEGRAM_APP_ID` и т.д.) нужны только для `sync`
