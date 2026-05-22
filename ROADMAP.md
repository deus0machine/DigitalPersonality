# Digital Personality — Development Roadmap

## Phase 1: Foundation (DONE)
- [x] Project structure (Clean Architecture)
- [x] docker-compose with pgvector/pgvector:pg16
- [x] PostgreSQL connection pool (pgx/v5)
- [x] Migration system (golang-migrate)
- [x] Structured logging (log/slog)
- [x] Config from environment (caarlos0/env)
- [x] Graceful shutdown (signal.NotifyContext + errgroup)
- [x] Domain entities: User, Chat, Message, Embedding
- [x] Repository interfaces (domain/repository)
- [x] Postgres repository implementations
- [x] pgvector cosine similarity search
- [x] CLAUDE.md architecture rules

---

## Phase 2: Telegram Sync Engine ✅
Goal: stream and persist all personal Telegram history.

Tasks:
- [x] Integrate `gotd/td` v0.144.0 (MTProto client)
- [x] Auth flow: phone → code → 2FA → session persistence (consoleAuthenticator)
- [x] `TelegramGateway` port interface (application/port/telegram.go)
- [x] Infrastructure implementation (infrastructure/telegram/)
- [x] File-based session storage (atomic rename, mode 0600)
- [x] Dialog list sync with pagination
- [x] Message history backfill with cursor-based incremental pagination
- [x] Idempotent ingestion (ON CONFLICT upsert)
- [x] sync_cursors persisted per-dialog
- [x] mapper.go: gotd/td types → domain-neutral DTOs (no leakage)
- [x] BackfillEngine orchestrator (application/sync/engine.go)
- [ ] Real-time update handler (new messages, edits, deletes) → Phase 3
- [ ] Media metadata ingestion → Phase 3
- [ ] Reconnect with exponential backoff for real-time → Phase 3

---

## Phase 3: Embedding Pipeline
Goal: generate and store vector embeddings for all messages.

Tasks:
- [ ] OpenAI client wrapper (retries, rate limiting, batching)
- [ ] EmbeddingWorker pool (configurable concurrency)
- [ ] Queue: channel-based in-process queue → later: persistent queue
- [ ] Backfill job: process all messages without embeddings
- [ ] Incremental job: embed new messages as they arrive
- [ ] Model abstraction: swap OpenAI for local model (Ollama, etc.)
- [ ] Embedding cost tracking / token counting

---

## Phase 4: Memory Engine
Goal: structured retrieval of relevant memories for context injection.

Tasks:
- [ ] MemoryRetriever interface in domain
- [ ] Semantic search usecase: query → embedding → top-k search
- [ ] Time-weighted retrieval (recent messages score higher)
- [ ] Keyword search fallback (pg_trgm)
- [ ] Memory clustering / deduplication
- [ ] Context window builder: assemble memories into LLM prompt context

---

## Phase 5: LLM Personality Layer
Goal: generate responses that mimic user communication style.

Tasks:
- [ ] PersonaEngine interface in domain
- [ ] OpenAI Chat Completions client
- [ ] System prompt builder from memory context
- [ ] Style analyzer: extract vocabulary, tone, response patterns
- [ ] Conversation session management (stateful context)
- [ ] Response quality evaluation (human-in-loop)

---

## Phase 6: API / Interface Layer
Goal: expose the personality engine via a usable interface.

Options (decide later):
- gRPC service for internal tool integration
- REST API with HTTP/JSON
- Telegram bot responding as the user

Tasks:
- [ ] Interface layer design (delivery/grpc or delivery/http)
- [ ] Auth/security for API endpoints
- [ ] Rate limiting, timeouts
- [ ] Health check endpoint (/healthz, /readyz)
- [ ] Metrics (Prometheus)
- [ ] Tracing (OpenTelemetry)

---

## Phase 7: Observability & Production Hardening
- [ ] Prometheus metrics for all critical paths
- [ ] OpenTelemetry traces through all layers
- [ ] Alerting rules for sync failures, embedding backlogs
- [ ] Backup strategy for postgres + session files
- [ ] Secret rotation support
- [ ] Deploy to Linux VPS via docker-compose

---

## Architecture Decisions Log

| Date | Decision | Reason |
|------|----------|--------|
| 2026-05 | pgvector/pgvector:pg16 Docker image | Provides vector extension pre-installed |
| 2026-05 | log/slog over zap/zerolog | Standard library, zero deps, sufficient for structured logging |
| 2026-05 | caarlos0/env for config | Clean struct-tag config, no runtime reflection overhead |
| 2026-05 | golang-migrate for schema | SQL-first, audit-friendly, no ORM magic |
| 2026-05 | errgroup for lifecycle | Composable cancellation without a custom supervisor |
| 2026-05 | IVFFlat index for vectors | Good ANN performance; HNSW can replace it after benchmarking |
| 2026-05 | TelegramGateway port pattern | application defines interface, infrastructure implements; gotd/td Run() pattern encapsulated |
| 2026-05 | atomic.Pointer[T] for td/api | Run() is reentrant boundary; atomics avoid mutex on read path |
| 2026-05 | consoleAuthenticator for auth | Interactive CLI auth for first run; session file reused on restarts |
| 2026-05 | Cursor-based sync strategy | Newest→oldest pagination, stop at saved cursor; resumable and idempotent |
