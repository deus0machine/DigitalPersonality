# ADR-0005 — CLI Delivery Layer (Inspectability First)

**Date**: 2026-05-22
**Status**: Accepted

---

## Context

Phase 4.7 completed the retrieval foundation: FTS, trigram similarity, episode search,
and per-chat personality analytics. However `retrieval.Service` had no interface —
it was dead code from a runtime perspective.

Before proceeding to Phase 5 (embedding pipeline), three questions needed validation:

1. Does FTS + trigram find semantically meaningful messages?
2. Does episode segmentation produce coherent conversation units?
3. Does personality signal extraction capture real style markers?

These questions cannot be answered by reading code. They require running queries
against real synchronized data and inspecting the results.

Two paths were available:
- **Path A**: Add embeddings first, then build a delivery layer.
- **Path B**: Build a CLI delivery layer first, validate existing retrieval, then add embeddings.

---

## Decision

**Path B**: Build a minimal CLI delivery layer before embeddings.

Location: `internal/interfaces/cli/`

Commands exposed via `cmd/server`:

| Command | What it does |
|---|---|
| `search <query>` | FTS + trigram search over messages; shows MatchType + Rank |
| `episodes <query>` | FTS over episode_semantic text |
| `similar <text>` | Trigram similarity for speech pattern discovery |
| `personality [chat-id]` | Per-chat personality analytics (active hours, length dist) |
| `chats` | List all synced chats with relevance scores |

---

## Why CLI-first

**1. Zero infrastructure overhead.**
No HTTP port, no CORS, no auth, no daemon. Just `go run ./cmd/server search "query"`.
The fastest possible feedback loop.

**2. CLI is the right tool for validation.**
The goal is inspectability — understanding what the system knows.
An HTTP API would add layers (JSON, status codes, request/response) without adding insight.
HTTP belongs to Phase 6 when external callers are expected (LLM persona simulation).

**3. Iteration speed.**
Change output format → re-run → see result. No redeploy, no client changes.

**4. No Telegram credentials needed.**
`config.LoadCLI()` parses only `AppConfig` + `PostgresConfig`.
Telegram session is not touched. The read path is completely isolated from the sync path.

---

## Why Inspectability > Premature AI Features

> Invariant I4: "Inspectability matters more than premature optimization."

The CLI makes the system's reasoning explicit in every result:

- `[fts]` vs `[trigram]` match type is shown for every message hit.
- Rank/score is numeric and visible.
- Episode linkage, personality surface, direction — all shown.

If retrieval is wrong, the CLI makes it immediately visible.

If embeddings were added first, wrong retrieval would be masked behind
opaque cosine similarity scores. Problems would be harder to diagnose.

---

## Why Embeddings Are Intentionally Postponed

Phase 5 (embeddings) introduces:

- OpenAI API calls (cost, latency, external dependency, rate limits)
- Async background worker pool
- pgvector index (new infrastructure)
- New retrieval path that needs to be composed with FTS + trigram

None of that is needed to validate whether the **existing** retrieval is useful.

If FTS misses important matches, the fix might be upstream of embeddings:
- Russian stemming (switch from `simple` to `pg_catalog.russian` in FTS)
- Better episode segmentation
- Better personality signal extraction

Adding embeddings before validating FTS would be premature optimization.
Embeddings are additive to FTS — not a replacement. They should be added
after the base retrieval quality is confirmed through real usage.

---

## Architectural Consequences

- `internal/interfaces/cli/` is the first and only delivery layer until Phase 6.
- No business logic in CLI handlers: parse args → call service → format output.
- `cmd/server` routes on `os.Args[1]`: CLI commands go to `Runner`, no args / "sync" → backfill.
- `TopEmoji` and `TopSlang` in `PersonalityReport` are empty until KI-013 is resolved.
- Future HTTP/gRPC delivery will follow the same pattern: thin handlers, use case in application layer.
