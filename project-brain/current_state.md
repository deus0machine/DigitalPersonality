# Current State

_Последнее обновление: 2026-05-22 (Phase 4.8 — CLI Delivery Layer)_

## Реализованные подсистемы

### Layer 1 — Raw Storage
- Таблица `messages`: все поля включая `is_forwarded`, `forward_from_id`, `forward_date`, `edit_date`
- Таблица `chats`: `relevance_score`, `relevance_reason`, `personality_surface`
- Таблица `users`, `sync_cursors`
- Upsert по `(telegram_id, chat_id)` — идемпотентная запись
- `MessageRepository`: Upsert, GetByID, GetByTelegramID, List, GetCursor, SaveCursor, MarkDeleted

### Layer 2 — Semantic Normalization
- Таблица `message_semantic`: нормализованный текст без служебных символов
- `SemanticNormalizer` (infrastructure/normalizer): чистит текст для поиска
- `SemanticRepository`: Upsert, GetByMessageID
- FTS-индекс: `text_search tsvector GENERATED ALWAYS AS (to_tsvector('simple', ...)) STORED`
- Trigram-индекс: `GIN (text gin_trgm_ops)` на `messages.text`

### Layer 3 — Personality Signals
- Таблица `personality_signals`: per-message фичи
- `PersonalityExtractor` (infrastructure/personality): извлекает сигналы из entity.Message
- `PersonalityRepository`: SaveSignals, GetByMessageID

### Layer 4 — Episodic Memory
- Таблица `episodes` + `episode_messages` + `episode_semantic`
- `EpisodeSegmenter` (infrastructure/episode): временные/контекстные разрывы → границы эпизодов
- `EpisodeBuilder` (application/episode): оркестрирует сегментацию и хранение
- `EpisodeRepository`: Create, LinkMessages, ListUnepisodedMessages, UpsertSemantic, GetSemantic, ListPendingEmbedding
- FTS-индекс на `episode_semantic.text_search`

### Telegram Gateway
- `internal/infrastructure/telegram/`: gotd/td клиент
- `client.go`: Run/Self/ListDialogs/GetHistory, session persistence (файл mode 0600)
- `mapper.go`: `mapMessage`, `mapDialogInfo`, `resolveForward`, `resolveEditDate`
- `auth.go`: интерактивная авторизация через терминал

### Dialog Relevance Scoring
- `ChatRelevanceScorer` (application/sync/scorer.go): pure function, 0.0–1.0
- `SyncThreshold = 0.35`: ниже — не синхронизировать
- `PersonalitySurface`: self_expression / interpersonal / social / tool_interaction / passive_consumption
- ВСЕ чаты апсертируются с оценками, даже исключённые — inspectability

### CLI Delivery Layer
- `internal/interfaces/cli/`: `Runner`, 5 команд, форматирование вывода
- `runner.go`: Runner struct — только DB + retrieval service, без Telegram
- `search.go`: FTS + trigram search, показывает MatchType + Rank
- `episodes.go`: поиск по episode_semantic
- `similar.go`: trigram similarity для поиска речевых паттернов
- `personality.go`: обзорная таблица (без аргументов) и детальный отчёт (с chat-id)
- `chats.go`: список всех синхронизированных чатов
- `config.LoadCLI()`: парсит только AppConfig + PostgresConfig, Telegram не требуется
- `cmd/server/main.go`: роутинг на os.Args[1] — CLI команды или sync
- ADR-0005: объяснение выбора CLI-first и отложенных embeddings

### Retrieval Foundation (без embeddings)
- `internal/application/retrieval/`: Query, MessageHit, EpisodeHit, PersonalityReport, Repository interface, Service
- `internal/infrastructure/postgres/repository/retrieval.go`: полная реализация
- SearchMessages: FTS (`websearch_to_tsquery`) → trigram fallback → filter-only
- SearchEpisodes: FTS по episode_semantic
- FindSimilar: trigram similarity для style clustering
- PersonalityReport: hour distribution, length classification, episode count per chat

### App Assembly
- `internal/app/app.go`: точка сборки всех зависимостей
- `cmd/server/main.go`: godotenv.Load() → config.Load() → app.Run()
- `docker-compose.yml`: postgres + pgvector, порт 5432

## Миграции

| Файл | Что добавляет |
|------|---------------|
| 000001_init.up.sql | messages, chats, users, sync_cursors |
| 000002_semantic.up.sql | message_semantic, personality_signals |
| 000003_episodes.up.sql | episodes, episode_messages, episode_semantic |
| 000004_chat_relevance.up.sql | relevance_score, relevance_reason, personality_surface на chats |
| 000005_message_richness.up.sql | is_forwarded, forward_*, edit_date на messages; FTS + trigram индексы |

## Текущий entry point

`cmd/server/main.go` диспетчеризует по `os.Args[1]`:

| Команда | Что делает |
|---|---|
| _(нет аргументов)_ / `sync` | Telegram backfill → `app.Run()` → `engine.RunBackfill()` |
| `search <query>` | Поиск сообщений (FTS + trigram) |
| `episodes <query>` | Поиск эпизодов |
| `similar <text>` | Похожие сообщения по trigram |
| `personality [chat-id]` | Personality аналитика |
| `chats` | Список всех чатов |

Sync — batch job, не daemon. CLI команды — read-only, не требуют Telegram сессии.

## Что НЕ реализовано

- Embedding pipeline (Phase 5): генерация векторов, pgvector запросы
- HTTP API (Phase 6)
- LLM persona simulation (Phase 6)
- Real-time updates (webhook / long-poll)
- TopEmoji, TopSlang в PersonalityReport (агрегация не написана, поля есть — KI-013)
