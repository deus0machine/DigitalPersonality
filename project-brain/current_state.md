# Current State

_Последнее обновление: 2026-07-15 (Phase 5.3.1 — Hybrid Retrieval, RRF)_

> **Примечание (2026-07-15):** embedding-провайдер — **Ollama (bge-m3, 1024 dim)**, не OpenAI.
> Migration 000009 сменила `vector(1536)` → `vector(1024)`. Упоминания OpenAI/1536 ниже — исторические.
> `infrastructure/openai/client.go` больше не подключён в runner (кандидат на удаление).
>
> **Fix (2026-07-15):** `schema_migrations` была dirty на версии 8 (миграцию 9 применяли вручную),
> из-за чего `sync`/`transcribe` падали на старте. Схема сверена с финалом 000009,
> выполнено `UPDATE schema_migrations SET version=9, dirty=false`.

## Реализованные подсистемы

### Layer 1 — Raw Storage
- Таблица `messages`: все поля включая `is_forwarded`, `forward_from_id`, `forward_date`, `edit_date`
- Таблица `chats`: `relevance_score`, `relevance_reason`, `personality_surface`, `access_hash`
- Таблица `users`, `sync_cursors`
- Upsert по `(telegram_id, chat_id)` — идемпотентная запись
- `MessageRepository`: Upsert, GetByID, GetByTelegramID, List, GetCursor, SaveCursor, MarkDeleted

### Layer 2 — Semantic Normalization
- Таблица `message_semantic`: нормализованный текст, `transcribed_at` (voice checkpoint)
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
- `client.go`: Run/Self/ListDialogs/GetHistory, session persistence (файл mode 0600), flood-wait retry с exponential backoff
- `mapper.go`: `mapMessage`, `mapDialogInfo`, `resolveForward`, `resolveEditDate`
- `auth.go`: интерактивная авторизация через терминал

### Dialog Relevance Scoring
- `ChatRelevanceScorer` (application/sync/scorer.go): pure function, 0.0–1.0
- `SyncThreshold = 0.35`: ниже — не синхронизировать
- `PersonalitySurface`: self_expression / interpersonal / social / tool_interaction / passive_consumption
- ВСЕ чаты апсертируются с оценками, даже исключённые — inspectability

### Participation-Centered Memory Windows (Phase 4.10)
- `in_memory_window BOOLEAN DEFAULT TRUE` на `messages` (migration 000006)
- `WindowRepository`: `ComputeParticipationWindows` (3-step atomic SQL) + `ListPendingRebuild`
- `WindowExpander` use case: compute → retroactive Layer 2-3 rebuild (batched, idempotent)
- `WindowConfig`: `WINDOW_BEFORE=10`, `WINDOW_AFTER=10` (env vars)
- `needsWindowing(surface)`: social/passive_consumption → windowed; остальные → full-sync
- Retrieval layer: `AND m.in_memory_window = TRUE` во всех message queries

### Utterance Pipeline (Phase 4.x + 5.3)

**Runtime группировка** (`application/utterance/builder.go`):
- `Build(msgs, gap)` — детерминированная группировка по автору + временному gap
- `Utterance` struct: `FirstMessageID`, `Text`, `MessageCount`, `IsOutgoing`, `HasVoice`, `VoiceCount`
- `FirstMessageID = group[0].ID` — стабильный ключ для embedding storage
- Utterances — in-memory DTOs, не материализованы в БД

**BM25 Retrieval** (`application/utterance/scorer.go`, `rerank.go`):
- `BM25Scorer`: term-frequency ranking по всем in-window utterances
- `RerankScorer`: length-sigmoid reranker поверх BM25 (K=10, Cap=100)
- `RetrievalService`: fetch → build → score → limit
- `RetrieveWithContext`: adds surrounding utterances per hit (window N)

**Utterance Stats CLI** (`interfaces/cli/utterance_stats.go`):
- `utterance-stats [chat-id]`: message count percentiles, size buckets, voice stats
- **Token length audit** (добавлено Phase 5.3): P50/P75/P90/P95/P99/Max в аппрокс. токенах,
  bucket distribution, %>256/>512/>1024, top-10 longest utterances с span и preview
- `compare-gaps <chat-id>`: сравнение 4 gap-значений (30/60/120/300s)
- `inspect-bursts <chat-id>`: top-50 по числу сообщений

**Корпусный аудит (проведён 2026-06-03)**:
- 207,779 utterances из ~480k raw messages
- P50=6 | P75=11 | P90=20 | P95=29 | P99=57 | Max=1923 токенов (approx runes/4)
- 99% utterances < 64 токенов — chunking не нужен
- Mean/Median messages per utterance = 2; P90 = 4
- Вывод: `utterance_id PK` без `chunk_index`; min_tokens фильтр = 10

### Utterance Embedding Infrastructure (Phase 5.3 MVP)

**Схема** (migration 000008):
- `utterance_embeddings(first_message_id PK FK→messages, model_name, gap_seconds, embedded_at, vector(1536))`
- HNSW индекс: `m=16, ef_construction=64`, cosine ops
- `first_message_id` — стабильный ключ при фиксированном `UTTERANCE_GAP_SECONDS`
- При смене gap: `DELETE FROM utterance_embeddings;` + re-run worker

**Application interfaces** (`application/utterance/embedding.go`):
- `Embedder`: `EmbedTexts(ctx, []string) ([][]float32, error)` + `EmbedQuery(ctx, string) ([]float32, error)`
- `UtteranceEmbeddingRepository`: `FilterUnembedded`, `SaveBatch`, `SearchByVector`, `StoredGapSeconds`
- `EmbeddingCandidate`, `VectorHit` — чистые Go-структуры без инфраструктурных зависимостей
- Инвариант I1 соблюдён: application не импортирует `openai` или `pgx`

**VectorScorer** (`application/utterance/vector.go`):
- Реализует `Scorer` — совместим с `RetrievalService` без изменений
- Embed query → `SearchByVector` → map по `FirstMessageID` → `[]SearchResult`
- Orphan embeddings (message вышел из in_memory_window) — silently skipped
- `topK=50` candidates из pgvector, финальный limit применяет `RetrievalService`
- Graceful: используется только при наличии `OPENAI_API_KEY`

**OpenAI client** (`infrastructure/openai/client.go`):
- HTTP клиент без внешних SDK, `Timeout: 30s`
- `EmbedTexts`: batch POST `/v1/embeddings`, `encoding_format=float`
- `EmbedQuery`: single-text wrapper
- Реализует `utterance.Embedder` — application layer зависит только от интерфейса

**Postgres реализация** (`infrastructure/postgres/repository/utterance_embedding.go`):
- `FilterUnembedded`: `SELECT ... WHERE first_message_id = ANY($1)` → diff со входом
- `SaveBatch`: транзакция с individual INSERT ON CONFLICT DO NOTHING
- `SearchByVector`: `ORDER BY vector <=> $1 LIMIT $2` (HNSW ANN)
- `StoredGapSeconds`: gap drift detection для worker

**embed-utterances CLI** (`interfaces/cli/embed.go`):
- Gap drift detection: если stored gap ≠ current gap → ошибка с инструкцией
- Token filter: skip utterances с `len([]rune(text))/4 < 10`
- `FilterUnembedded` → batch embed (100/batch, 200ms delay) → `SaveBatch`
- Идемпотентен: повторный запуск пропускает уже эмбеддированные

**retrieve-vector CLI** (`interfaces/cli/retrieve_vector.go`):
- Semantic retrieval через pgvector ANN
- Показывает `similarity=` (1 - cosine distance), направление →/←, временной диапазон
- Требует `OPENAI_API_KEY` и заполненной `utterance_embeddings`

**Runner wire-up** (`interfaces/cli/runner.go`):
- `embeddingRepo`: всегда инициализирован (без API-ключа используется только для StoredGapSeconds)
- `embedder`, `vectorSvc`: `nil` если `OPENAI_API_KEY` не задан — graceful degradation
- `CLIConfig.OpenAI OpenAIConfig` добавлен

### Sender Integrity Fix (Phase 4.9)
- `port.HistoryPage.Participants []UserInfo` — из `v.Users` каждого API response
- `upsertParticipants()` — bulk upsert до обработки страницы
- `UserRepository.EnsureExists()` — fallback для deleted accounts

### Voice Transcription Infrastructure (Phase 5.1)
- `chats.access_hash BIGINT` (migration 000007) — для пересборки InputPeer после рестарта
- `message_semantic.transcribed_at TIMESTAMPTZ` — idempotent checkpoint worker'а
- `runTranscribe()` в `cmd/server/main.go` — отдельный entry point

### CLI Delivery Layer
- `runner.go`: Runner — только DB + services, без Telegram
- `search.go`: FTS + trigram, MatchType + Rank
- `episodes.go`: поиск по episode_semantic
- `similar.go`: trigram similarity для речевых паттернов
- `personality.go`: обзорная таблица / детальный отчёт
- `chats.go`: список чатов с relevance scores
- `windows.go`: window coverage + detail + anchor preview
- `validate.go`: глобальный quality report + автоматические warnings + top-20 чатов
- `audit.go`: `retrieve-audit` — BM25 vs BM25+Rerank (10 test queries, LONG%, MULTI%, GAP)
- `retrieve.go`: `retrieve`, `retrieve-debug`
- `retrieve_context.go`: `retrieve-context`, `retrieve-context-debug`
- `utterances.go`: `inspect-utterances`
- `utterance_stats.go`: `utterance-stats`, `compare-gaps`, `inspect-bursts`
- `embed.go`: `embed-utterances` — batch embedding worker
- `retrieve_vector.go`: `retrieve-vector` — pure vector search

### App Assembly
- `internal/app/app.go`: точка сборки всех зависимостей
- `cmd/server/main.go`: godotenv.Load() → config.Load() → dispatch
- `docker-compose.yml`: postgres + pgvector, порт 5432

## Миграции

| Файл | Что добавляет |
|------|---------------|
| 000001_init_schema | messages, chats, users, sync_cursors |
| 000002_multi_layer_memory | message_semantic, personality_signals |
| 000003_episodes | episodes, episode_messages, episode_semantic |
| 000004_chat_relevance | relevance_score, relevance_reason, personality_surface на chats |
| 000005_message_richness | is_forwarded, forward_*, edit_date; FTS + trigram индексы |
| 000006_memory_window | in_memory_window BOOLEAN DEFAULT TRUE; два partial index |
| 000007_voice_transcription | chats.access_hash BIGINT; message_semantic.transcribed_at |
| 000008_utterance_embeddings | utterance_embeddings + HNSW индекс |

## Текущий entry point

`cmd/server/main.go` диспетчеризует по `os.Args[1]`:

| Команда | Что делает |
|---|---|
| _(нет аргументов)_ / `sync` | Telegram backfill |
| `transcribe` | Voice transcription worker (Telegram Premium) |
| `search <query>` | FTS + trigram поиск сообщений |
| `episodes <query>` | Поиск эпизодов |
| `similar <text>` | Trigram similarity |
| `personality [chat-id]` | Personality аналитика |
| `chats` | Список чатов |
| `windows [chat-id]` | Memory window coverage |
| `validate` | Глобальный quality report |
| `inspect-chat <chat-id>` | Детальный per-chat диагностический отчёт |
| `voice-stats` | Статистика голосовых сообщений |
| `media-inspect` | Полный медиа аудит |
| `inspect-utterances <chat-id>` | Preview utterances для чата |
| `utterance-stats [chat-id]` | Quality metrics + token length audit |
| `compare-gaps <chat-id>` | Сравнение gap=30/60/120/300s |
| `inspect-bursts <chat-id>` | Top-50 длинных utterances |
| `retrieve <query>` | BM25+Rerank retrieval |
| `retrieve-debug <query>` | BM25+Rerank + pipeline timing |
| `retrieve-context <query>` | Retrieval с контекстным окном |
| `retrieve-context-debug <query>` | То же + метрики |
| `retrieve-audit` | BM25 vs BM25+Rerank сравнение (10 queries) |
| `embed-utterances` | Batch embedding worker (требует OLLAMA_EMBEDDING_MODEL) |
| `retrieve-vector <query>` | Semantic retrieval через pgvector (требует OLLAMA_EMBEDDING_MODEL) |
| `retrieve-hybrid <query>` | BM25+Rerank + vector через RRF k=60 (требует OLLAMA_EMBEDDING_MODEL) |
| `retrieve-audit-vector` | BM25 vs Vector vs Hybrid аудит: OVERLAP, NEW%, hybrid состав |
| `ask <сообщение>` | Разговор с цифровой личностью: бёрст сообщений с паузами (требует OLLAMA_CHAT_MODEL) |
| `bot` | Telegram-бот @FutureBond_Bot: персона отвечает на входящие (требует TELEGRAM_BOT_TOKEN) |

### LLM Persona (Phase 6 MVP, 2026-07-15)
- `application/persona/`: Service + порты Generator/StyleRepository/Retriever
- StyleProfile из живых данных: burst avg 1.89 (P90=3), паузы P50=5s/P90=13s
- Политика знаний: утечка эрудиции разрешена, но строго в манере персоны
- `ollama.ChatClient` (gemma3:4b, structured JSON output), `OLLAMA_CHAT_MODEL` в .env
- Smoke-тесты пройдены: бёрст-формат, опора на память, встречные вопросы работают

### Hybrid Retrieval (Phase 5.3.1, 2026-07-15)
- `application/utterance/hybrid.go`: `HybridScorer` — RRF по рангам, k=60
- Аудит 2026-07-15: NEW% = 83% — vector даёт морфологические/семантические совпадения,
  недоступные FTS 'simple' (KI-012)
- Эмбеддинги: 72,385 utterances (все ≥10 approx tokens из 231,719), bge-m3, gap=120s

## Что НЕ реализовано

- **Episode embeddings** — Phase 5.4, опционально (hybrid на utterances подтверждён)
- **HTTP API** — Phase 6
- **LLM persona simulation** — Phase 6
- **Real-time updates** — не планируется в текущей архитектуре
- **TopEmoji, TopSlang в PersonalityReport** — KI-013, SQL агрегация не написана
