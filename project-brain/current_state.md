# Current State

_Последнее обновление: 2026-05-31 (Phase 4.11 — Validation & Inspection CLI)_

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

### Participation-Centered Memory Windows (Phase 4.10)

Architecture: group/channel dialogs generate huge volumes of noise — only messages near
user participation (outgoing anchors) are relevant to personality/semantic/episodic pipelines.

**`in_memory_window` column** (`migration 000006`):
- `BOOLEAN NOT NULL DEFAULT TRUE` on `messages` — zero breaking change; all existing messages stay active
- `TRUE`: message flows through Layers 2-4 (semantic, personality, episodic)
- `FALSE`: stored in Layer 1 only — inspectable, not processed

**WindowRepository** (`domain/repository/window.go` + `infrastructure/postgres/repository/window.go`):
- `ComputeParticipationWindows(chatID, before, after)` — 3-step atomic SQL transaction:
  1. Reset non-outgoing messages → `in_memory_window = FALSE`
  2. CTE with `ROW_NUMBER()` expands ±before/after rows around each outgoing anchor → `TRUE`
  3. Direct reply targets of outgoing messages → `TRUE`
- `ListPendingRebuild(chatID, limit)` — messages with `in_memory_window=TRUE` but no semantic doc

**WindowExpander use case** (`application/window/expander.go`):
- `ComputeAndRebuild(chatID)` — runs computation then retroactive Layer 2-3 rebuild
- `rebuildLayers` — batched (100/batch): `ListPendingRebuild` → normalize → semantic upsert → personality signals
- Layer 4 (episodes) runs after via existing `episode.Builder`

**WindowConfig** (`config.go`): `WINDOW_BEFORE=10`, `WINDOW_AFTER=10` (env vars)

**Surfaces and windowing**:
- `social`, `passive_consumption` → windowed (noise-heavy group chats)
- `interpersonal`, `self_expression`, `tool_interaction` → full-sync (flag stays TRUE)
- `needsWindowing(surface)` predicate in `sync/engine.go` gates the decision

**Retrieval + episode layer integration**:
- `messageWhereClause`: `AND m.in_memory_window = TRUE` — all search queries respect window
- `fetchReports`: counts only windowed messages (personality analytics exclude noise)
- `ListUnepisodedMessages`: `AND m.in_memory_window = TRUE` — episodes built only from windowed messages
- `ListPendingEmbedding`: `JOIN messages … WHERE m.in_memory_window = TRUE` — embedding pipeline skips orphan docs

**Sync engine integration** (`sync/engine.go`):
- `toSync []scored` carries surface through the loop (was `[]port.DialogInfo`)
- After sync: `needsWindowing` → `windowExpander.ComputeAndRebuild` → `episodeBuilder.BuildForDialog`

**Window inspection CLI** (`interfaces/cli/windows.go`):
- `windows` → coverage table: chat, surface, total, in-window, anchors, %retained, pending
- `windows <chat-id>` → detail view + sample anchor windows (±5 context, 3 anchors preview)

**SQL inspection toolkit** (`docs/sql/inspect_windows.sql`):
- 8 inspection queries + 7 validation (sanity) queries
- Validates: retained ratio anomalies, orphan anchors, episodes from non-window messages,
  personality signals outside window, orphan semantic docs, embedding pipeline health

### Sender Integrity Fix (Phase 4.9)
- `port.HistoryPage.Participants []UserInfo` — участники каждой страницы из `v.Users` API response
- `mapParticipants()` (telegram/mapper.go) — фильтрует `*tg.UserEmpty`, возвращает `[]UserInfo`
- `upsertParticipants()` (sync/engine.go) — bulk upsert перед каждой страницей сообщений
- `UserRepository.EnsureExists()` — `INSERT ... ON CONFLICT DO NOTHING` fallback для deleted accounts
- `ingestMessage` — вызывает `EnsureExists` как belt-and-suspenders перед каждым message upsert
- Устранён FK violation `messages_sender_id_fkey` для всех случаев: группы, личные чаты, подписанные посты каналов

### CLI Delivery Layer
- `internal/interfaces/cli/`: `Runner`, 5 команд, форматирование вывода
- `runner.go`: Runner struct — только DB + retrieval service, без Telegram
- `search.go`: FTS + trigram search, показывает MatchType + Rank
- `episodes.go`: поиск по episode_semantic
- `similar.go`: trigram similarity для поиска речевых паттернов
- `personality.go`: обзорная таблица (без аргументов) и детальный отчёт (с chat-id)
- `chats.go`: список всех синхронизированных чатов
- `windows.go`: window coverage summary + detail с sample anchor windows
- `validate.go`: `validate` + `inspect-chat` — качество памяти и per-chat диагностика
- `config.LoadCLI()`: парсит только AppConfig + PostgresConfig, Telegram не требуется
- `cmd/server/main.go`: роутинг на os.Args[1] — CLI команды или sync
- ADR-0005: объяснение выбора CLI-first и отложенных embeddings

### Validation & Inspection (Phase 4.11)

**`validate`** — глобальный quality report:
- Messages: total / in-window / outgoing counts + проценты
- Episodes: count + avg size
- Personality signals: count
- Chats by surface: breakdown по PersonalitySurface
- Автоматические warnings (5 проверок — см. ниже)
- Top-20 чатов по объёму: score, surface, total, in-window, out%, episodes

**Автоматические warnings**:
- `< 10%` сообщений в memory windows → window computation не запускалась
- `> 95%` сообщений в memory windows → windowing для social/passive_consumption не активно
- 0 personality signals → Layer 3 не запускался
- 0 episodes при >100 сообщениях → Layer 4 не запускался
- Чаты с score >0.8 и 0 сообщений → возможный sync gap
- Suspiciously low episode ratio (`< 0.5%`) при >500 сообщениях

**`inspect-chat <chat-id>`** — детальный per-chat snapshot:
- Chat ID, surface, score
- Messages total / outgoing / in-window (с процентами)
- Episode count
- Sample participation windows (3 anchors × ±5 context messages, reusing WindowAnchors)

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
| 000006_memory_window.up.sql | in_memory_window BOOLEAN DEFAULT TRUE на messages; два partial index |

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
| `windows` | Memory window coverage: все чаты |
| `windows <chat-id>` | Window detail + sample anchor preview |
| `validate` | Глобальный quality report + автоматические warnings + top-20 чатов |
| `inspect-chat <chat-id>` | Детальный диагностический отчёт по одному чату + sample windows |

Sync — batch job, не daemon. CLI команды — read-only, не требуют Telegram сессии.

## Что НЕ реализовано

- Embedding pipeline (Phase 5): генерация векторов, pgvector запросы
- HTTP API (Phase 6)
- LLM persona simulation (Phase 6)
- Real-time updates (webhook / long-poll)
- TopEmoji, TopSlang в PersonalityReport (агрегация не написана, поля есть — KI-013)
