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

---

## Текущее

Phase 4.8 завершена. CLI delivery layer позволяет инспектировать память через консоль.
Следующий шаг: валидировать retrieval на реальных данных, затем Phase 5.

---

## Следующее (Phase 5 — Embedding Pipeline)

**Цель**: векторный поиск поверх существующего FTS+trigram.

Что нужно:
- `message_embeddings` таблица (pgvector, dim=1536 или dim=3072)
- Batch embedding worker: читает `ListPendingEmbedding`, вызывает OpenAI API
- Retry + backoff, дедупликация
- `EmbeddingRepository`: SaveEmbedding, ListPending, MarkDone
- Расширить `retrieval.Repository`: `SearchByVector(ctx, vec []float32, q Query)`
- Обновить `OPENAI_API_KEY` как required

**Инвариант**: embeddings — infrastructure, не business logic. Векторный поиск аддитивен к FTS, не заменяет.

---

## Следующее (Phase 6 — LLM Persona)

**Цель**: симуляция стиля общения через LLM + memory retrieval.

Что нужно:
- PromptBuilder: собирает контекст из retrieved episodes + personality signals
- PersonaService: stateless генератор, personality из памяти
- HTTP API или CLI: интерфейс для запросов к персоне

**Инвариант**: LLM не знает personality напрямую — только через retrieval.

---

## Идеи на будущее

- Relationship graph: кто с кем, как часто, тональность
- Emotional modeling: sentiment per conversation arc
- Multimodal memory: voice messages, stickers как personality markers
- Temporal drift: как менялся стиль со временем
- Autonomous memory consolidation: периодическая перегенерация эпизодов
