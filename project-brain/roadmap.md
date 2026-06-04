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
- `internal/interfaces/cli/`: Runner + команды (search, episodes, similar, personality, chats)
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
- CLI: `windows` + `windows <chat-id>` — coverage table, anchor preview

### Phase 4.11 — Validation & Inspection CLI
- `validate`: глобальный quality report + автоматические warnings (6 проверок) + top-20 чатов
- `inspect-chat <chat-id>`: детальный per-chat snapshot + sample participation windows
- `voice-stats`, `media-inspect`: медиа аудит

### Phase 5.1 — Voice Transcription Infrastructure
- `chats.access_hash BIGINT` (migration 000007) — для пересборки InputPeer после рестарта
- `message_semantic.transcribed_at TIMESTAMPTZ` — idempotent checkpoint
- `transcribe` command в `cmd/server/main.go`

### Phase 5.3 MVP — Utterance Embedding Infrastructure

**Корпусный аудит (2026-06-03)**:
- 207,779 utterances из ~480k raw messages
- P50=6 | P75=11 | P90=20 | **P95=29** | P99=57 | Max=1923 токенов
- 99% utterances < 64 токенов → chunking не нужен
- `utterance_id PK` без `chunk_index` — окончательное решение по схеме

**Что реализовано**:
- `utterance-stats` расширен: token length percentiles, buckets, top-10 по длине
- `Utterance.FirstMessageID` — стабильный embedding key (`group[0].ID`)
- Migration 000008: `utterance_embeddings(first_message_id PK, model_name, gap_seconds, embedded_at, vector(1536))` + HNSW индекс
- `application/utterance/embedding.go`: `Embedder`, `UtteranceEmbeddingRepository`, `EmbeddingCandidate`, `VectorHit`
- `application/utterance/vector.go`: `VectorScorer` — реализует `Scorer`, graceful orphan skip
- `infrastructure/openai/client.go`: HTTP клиент, 30s timeout, batch `EmbedTexts`, `EmbedQuery`
- `infrastructure/postgres/repository/utterance_embedding.go`: `FilterUnembedded`, `SaveBatch`, `SearchByVector`, `StoredGapSeconds`
- `CLIConfig.OpenAI OpenAIConfig` — API ключ опционален, команды деградируют без него
- `embed-utterances`: gap drift detection, min_tokens=10 filter, batch 100, идемпотентен
- `retrieve-vector`: pure vector search, similarity score, направление →/←

**Архитектурные решения**:
- `Embedder` интерфейс в application layer — I1 соблюдён
- Gap-change strategy: `DELETE FROM utterance_embeddings;` + re-run (no versioning)
- `HybridScorer` / RRF отложены до получения реальных данных о vector recall

---

## Текущее

**Phase 5.3 MVP реализована. Embedding infrastructure готова к запуску.**

Следующий шаг: запустить `embed-utterances`, затем сравнить `retrieve "запрос"` vs `retrieve-vector "запрос"` вручную на тестовых запросах.

По результатам решить:
- Достаточен ли vector recall → реализовать HybridScorer + RRF + retrieve-audit Hybrid колонку
- Недостаточен → рассмотреть episode embeddings как основную единицу (Phase 5.4)

---

## Phase 5.3.1 — Hybrid Retrieval (после аудита vector recall)

**Цель**: объединить BM25+Rerank и vector search через Reciprocal Rank Fusion.

**Триггер**: `retrieve-vector` показывает результаты, которых нет в `retrieve` (новые семантические совпадения).

**Что нужно реализовать**:
- `application/utterance/hybrid.go`: `HybridScorer` — RRF(BM25, vector), k=60
- `retrieve-audit` расширение: Hybrid колонка + NEW% метрика
- `CLIConfig.OpenAI` уже подключён — дополнительных config изменений нет

**Решение об отложенности**: если vector recall слабый (<15% NEW результатов) →
перейти к Phase 5.4 (episode embeddings) вместо hybrid на utterances.

---

## Phase 5.4 — Episode Embeddings (условная)

**Цель**: семантический retrieval на уровне эпизодов для persona simulation.

**Почему может быть нужна**: utterances короткие (P95=29 токенов) — episode_semantic
содержит агрегированный текст нескольких utterances, лучший embedding сигнал для тематических запросов.

**Что нужно**: `episode_embeddings(episode_id PK, model_name, embedded_at, vector(1536))` + worker + retrieval.

**Триггер**: retrieve-audit показывает слабый прирост от utterance vector search.

---

## Phase 5.5 — Sticker Analytics + Emotional Vocabulary (независимо)

**Цель**: извлечь personality signals из уже хранящихся sticker_meta без API вызовов.

- `sticker_meta->>'Emoticon'` → `personality_signals` типа `sticker_emoticon`
- Aggregation: топ-N эмодзи по чатам, по времени суток
- PersonalityReport: секция "Sticker Communication Style"
- Effort: ~2 часа (чистый SQL), нет зависимостей

---

## Phase 6 — LLM Persona + HTTP API

**Round transcription**: document metadata migration + Whisper (2107 in-window round videos).

**Photo captions**: `text` уже хранит caption → использовать напрямую (zero effort).

**LLM Persona**:
- PromptBuilder: контекст из retrieved episodes + personality signals
- PersonaService: stateless, personality только через retrieval
- HTTP API поверх того же retrieval.Service

---

## Backlog

### Phase 5 (ближайшие)

| Фича | Ценность | Сложность | Статус |
|---|---|---|---|
| embed-utterances run + аудит | ⭐⭐⭐⭐⭐ | XS | **Следующий шаг** |
| HybridScorer + RRF | ⭐⭐⭐⭐ | S | После аудита vector |
| retrieve-audit Hybrid колонка | ⭐⭐⭐⭐ | S | После HybridScorer |
| Voice transcription worker | ⭐⭐⭐⭐⭐ | M | Инфраструктура готова |
| Episode embeddings | ⭐⭐⭐⭐ | M | Условно (Phase 5.4) |
| Sticker emoticon aggregation | ⭐⭐⭐ | S | Независимо от embedding |

### Phase 6+

| Фича | Ценность | Сложность | Описание |
|---|---|---|---|
| Round video transcription | ⭐⭐⭐⭐ | XL | document metadata + Whisper |
| Document text extraction | ⭐⭐⭐ | M | PDF/DOCX → text → embedding |
| Relationship graph | ⭐⭐⭐⭐ | L | Кто с кем, как часто, тональность |
| Emotional modeling | ⭐⭐⭐ | L | Sentiment per episode arc |
| HTTP API | ⭐⭐⭐⭐ | M | Поверх существующего retrieval.Service |
| LLM Persona CLI/API | ⭐⭐⭐⭐⭐ | L | PromptBuilder + PersonaService |

### Phase 7+

| Фича | Ценность | Сложность | Описание |
|---|---|---|---|
| Photo vision analysis | ⭐⭐ | XL | GPT-4V / Claude Vision |
| Multimodal embeddings | ⭐⭐⭐⭐ | XXL | CLIP / unified vector space |
| Temporal drift analysis | ⭐⭐⭐ | M | Как менялся стиль со временем |
| Autonomous memory consolidation | ⭐⭐⭐ | L | Периодическая перегенерация эпизодов |

---

## Известные технические долги (сознательно отложены)

| ID | Описание | Триггер для решения |
|---|---|---|
| TD-01 | Utterance embeddings staleness при sync (граничные utterances) | Доля stale > 5% при аудите |
| TD-02 | Full corpus rebuild в embed-utterances (не инкрементален) | Corpus > 1M сообщений |
| TD-03 | HNSW без filter по model/gap | Второй gap или вторая embedding модель |
| TD-04 | Utterances не материализованы в БД (нет stable FK) | Нужны join'ы или cross-source queries |
| TD-05 | TopEmoji, TopSlang в PersonalityReport всегда пусты (KI-013) | При запросе personality аналитики |
| TD-06 | Query embedding latency для HTTP API | Phase 6 с interactive SLA |
