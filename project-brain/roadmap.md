# Roadmap

## Завершено

### Phase 1 — Domain + Storage Foundation
- Доменные сущности: Message, Chat, User, Episode, PersonalitySignal
- Интерфейсы репозиториев
- PostgreSQL реализации всех репозиториев
- Система миграций (golang-migrate)

### Phase 2 — Telegram MTProto Ingestion
- gotd/td клиент с session persistence
- ListDialogs, GetHistory с пагинацией
- Идемпотентный upsert, sync cursors (incremental backfill)
- Port pattern: Telegram-типы не выходят за infrastructure

### Phase 3 — Semantic + Personality Layers
- SemanticNormalizer: нормализация текста
- PersonalityExtractor: per-message сигналы (длина, emoji, caps, пунктуация)
- Два независимых слоя поверх raw storage

### Phase 4 — Episodic Memory
- EpisodeSegmenter: временные и контекстные разрывы → границы эпизодов
- EpisodeBuilder: полная оркестрация сегментации
- EpisodeRepository с LinkMessages (batch upsert)
- episode_semantic: нормализованный текст для поиска по эпизодам

### Phase 4.5 — Relevance Scoring + Personality Surfaces
- ChatRelevanceScorer (0.0–1.0) вместо бинарного фильтра
- PersonalitySurface: 5 поверхностей с разным весом
- Inspectability: все чаты сохраняются с оценками
- Собственные каналы: правильно определяются как self_expression

### Phase 4.6 — Data Quality
- Forward metadata: is_forwarded, forward_from_id, forward_date
- Edit tracking: edit_date
- Saved Messages: корректно обрабатываются как ChatTypeSavedMessages
- Боты: включены в sync с поверхностью tool_interaction

### Phase 4.7 — Retrieval Foundation (без embeddings)
- FTS с websearch_to_tsquery + ts_rank_cd
- Trigram similarity fallback (pg_trgm)
- SearchMessages, SearchEpisodes, FindSimilar
- PersonalityReport: per-chat аналитика

### Phase 4.8 — CLI Delivery Layer (Inspectability)
- `internal/interfaces/cli/`: Runner + 5 команд (search, episodes, similar, personality, chats)
- `config.LoadCLI()`: CLI-режим без Telegram credentials
- `cmd/server` роутинг: os.Args[1] → CLI или sync
- ADR-0005: CLI-first, inspectability > embeddings

### Phase 4.9 — Sender Integrity Fix
- FK violation `messages_sender_id_fkey` устранён для групп, личных чатов, каналов
- `HistoryPage.Participants`: извлекаем `v.Users` из каждого API response
- `upsertParticipants`: bulk upsert до обработки страницы сообщений
- `UserRepository.EnsureExists`: belt-and-suspenders fallback

### Phase 4.10 — Participation-Centered Memory Windows
- `in_memory_window BOOLEAN DEFAULT TRUE` на messages (migration 000006, zero breaking change)
- `WindowRepository`: `ComputeParticipationWindows` (3-step atomic SQL) + `ListPendingRebuild`
- `WindowExpander` use case: compute → retroactive Layer 2-3 rebuild (batched, idempotent)
- `WindowConfig`: `WINDOW_BEFORE=10`, `WINDOW_AFTER=10` (env vars)
- Sync engine: `needsWindowing(surface)` gate → `ComputeAndRebuild` после sync, до episodes
- Retrieval layer: `AND m.in_memory_window = TRUE` во всех message queries
- Episode builder: `AND m.in_memory_window = TRUE` в `ListUnepisodedMessages`
- Embedding pipeline: `ListPendingEmbedding` JOIN messages + window filter (no orphan embeddings)
- CLI: `windows` + `windows <chat-id>` — coverage table, anchor preview, pending rebuild indicator
- SQL toolkit: `docs/sql/inspect_windows.sql` — 8 inspection + 7 validation queries

---

## Текущее

Phase 4.11 завершена. Validation & Inspection CLI, media audit (`media-inspect`, `voice-stats`),
distributed anchor sampling, episode quality stats в `inspect-chat`.

Следующий шаг: Phase 5.1 — access_hash fix + Voice Transcription Pipeline.

---

## Phase 5.1 — Voice Transcription (Telegram Premium)

**Цель**: конвертировать 948 in-window голосовых сообщений в семантическую память.

**Почему сейчас**: 55% voice retention rate, Telegram Premium доступен, нет зависимости от OpenAI.
Спонтанная речь — высокоценный personality signal.

**Блокер (снимается за ~30 мин)**:
`access_hash` не хранится → `InputPeer` нельзя построить после перезапуска.
`port.DialogInfo.AccessHash` уже есть в sync engine — просто не пробрасывается в upsert.

Фикс — 4 изменения:
1. `migrations/000007`: `ALTER TABLE chats ADD COLUMN access_hash BIGINT NOT NULL DEFAULT 0`
2. `entity.Chat.AccessHash int64`
3. `ChatRepository.Upsert` → сохраняет `c.AccessHash`
4. `sync/engine.go` → `AccessHash: s.dialog.AccessHash` при `chatRepo.Upsert`

**Transcription Worker**:
- Очередь: `media_kind='voice' AND in_memory_window=TRUE AND ms.transcribed_at IS NULL`
- Вызов: `tg.MessagesTranscribeAudio{Peer, MsgID}`
- Pending=true → polling-retry через 30 s (не update stream)
- Результат: `ms.normalized_text = transcript`, `ms.skip_embedding = FALSE`, `ms.transcribed_at = NOW()`
- Throttle: 1 req / 5 s → 948 сообщений ≈ 1.5 ч
- Idempotent: `transcribed_at IS NULL` guard

**Важно**: `messages.transcribeAudio` НЕ поддерживает round video (`MSG_VOICE_MISSING`).
Round отложен в Phase 6 (требует Whisper + document metadata).

---

## Phase 5.5 — Sticker Analytics + Emotional Vocabulary

**Цель**: извлечь personality signals из уже хранящихся sticker_meta без API вызовов.

**Sticker emoticon aggregation** (данные уже в DB):
- `sticker_meta->>'Emoticon'` → `personality_signals` типа `sticker_emoticon`
- Aggregation: топ-N эмодзи по чатам, по времени суток, по контексту разговора
- Effort: ~2 часа (чистый SQL)

**Emotional vocabulary extraction**:
- Sticker как замена слову → "я использую 😭 вместо 'мне грустно'"
- Корреляция emoticon + preceding/following text → эмоциональный паттерн
- PersonalityReport: section "Sticker Communication Style"

**Sticker usage patterns**:
- Частота sticker vs text reply (показывает стиль коммуникации)
- Любимые паки по поверхностям (interpersonal vs social)
- Временные паттерны (вечерние sticker vs дневные text)

---

## Phase 5.3 — Embedding Pipeline

После Phase 5.1: транскрипты автоматически входят в embedding queue (`skip_embedding=FALSE`).

- `message_embeddings` таблица (pgvector, dim=1536)
- Batch embedding worker: `ListPendingEmbedding` → OpenAI API
- `SearchByVector` в retrieval
- Embeddings — infrastructure. Аддитивны к FTS.

---

## Phase 6 — Round Video + Photo + LLM Persona

**Round transcription**: document metadata migration + Whisper. 2107 in-window.

**Photo captions**: `text` уже хранит caption → использовать напрямую (zero effort).

**LLM Persona**:
- PromptBuilder: контекст из retrieved episodes + personality signals
- PersonaService: stateless, personality только через retrieval
- HTTP API или CLI интерфейс

---

## Backlog — Media Pipeline

### 🔜 Phase 5

| Фича | Ценность | Сложность | Зависимость |
|---|---|---|---|
| access_hash storage (migration 000007) | Blocker | S | — |
| Voice transcription worker | ⭐⭐⭐⭐⭐ | M | access_hash, Premium |
| Sticker emoticon aggregation | ⭐⭐⭐ | S | — (данные уже в DB) |

### 📋 Phase 6

| Фича | Ценность | Сложность | Описание |
|---|---|---|---|
| Round video transcription | ⭐⭐⭐⭐ | XL | document metadata + Whisper |
| Document text extraction | ⭐⭐⭐ | M | PDF/DOCX → text → embedding |
| Photo caption retrieval | ⭐⭐ | XS | text уже хранится |
| Relationship graph | ⭐⭐⭐⭐ | L | Кто с кем, как часто, тональность |
| Emotional modeling | ⭐⭐⭐ | L | Sentiment per episode arc |

### 🔭 Phase 7+

| Фича | Ценность | Сложность | Описание |
|---|---|---|---|
| Photo vision analysis | ⭐⭐ | XL | GPT-4V / Claude Vision, ~4438 in-window |
| Video vision/audio analysis | ⭐ | XXL | 12805 in-window, mostly consumed content |
| Multimodal embeddings | ⭐⭐⭐⭐ | XXL | CLIP / unified vector space |
| Temporal drift analysis | ⭐⭐⭐ | M | Как менялся стиль со временем |
| Autonomous memory consolidation | ⭐⭐⭐ | L | Периодическая перегенерация эпизодов |

---

## ROI матрица (media pipeline)

| Тип | In-window | Effort | Value | Phase |
|---|---|---|---|---|
| Voice transcription | 948 | M | ⭐⭐⭐⭐⭐ | **5.1** |
| Sticker emoticons | 8583 | S | ⭐⭐⭐ | **5.2** |
| Round transcription | 2107 | XL | ⭐⭐⭐⭐ | 6 |
| Photo captions | 4438 | XS | ⭐⭐ | 6 |
| Document text | 1224 | M | ⭐⭐⭐ | 6 |
| Photo vision | 4438 | XL | ⭐⭐ | 7+ |
| Video analysis | 12805 | XXL | ⭐ | 7+ |
