# ADR-0006 — Production-Grade FLOOD_WAIT Handling

**Status**: Accepted  
**Date**: 2026-05-30

---

## Context

Telegram's MTProto API enforces rate limits by returning `FLOOD_WAIT_N` errors (RPC code 420)
when a client sends too many requests in a short window. N is the number of seconds the client
must pause before retrying the same method.

The previous implementation treated any error from `MessagesGetHistory` as a non-recoverable
dialog failure — logging at ERROR level and skipping the entire dialog. This caused:

- Large, high-value dialogs (groups with thousands of messages) to be skipped entirely
- Incomplete backfills that silently looked successful
- False ERROR log noise for an expected operational condition

FLOOD_WAIT is **not a bug** — it is Telegram's normal demand-management mechanism.
Production clients are expected to respect it and retry.

---

## Decision

### Retry logic in infrastructure layer (telegram/client.go)

FLOOD_WAIT handling lives entirely in `Client.fetchHistoryWithRetry` — the infrastructure
layer that owns all Telegram API interactions. The application layer (`sync/Engine`) continues
to call `gateway.GetHistory` with no knowledge of retry mechanics.

```
GetHistory call → FLOOD_WAIT → sleep(flood_wait × multiplier^(attempt-1) + jitter) → retry
```

**Parameters** (configurable via env vars, sensible defaults):

| Parameter | Env var | Default | Description |
|---|---|---|---|
| `FloodMaxRetries` | `SYNC_FLOOD_MAX_RETRIES` | 5 | Max retries before dialog fails |
| `FloodJitter` | `SYNC_FLOOD_JITTER` | 1s | Added to every sleep to spread retries |
| `FloodBackoffMultiplier` | `SYNC_FLOOD_BACKOFF_MULT` | 1.5 | Multiplies sleep duration each attempt |

**Sleep formula**:
```
sleep = floodWaitDuration × multiplier^(attempt-1) + jitter
```

Example with FLOOD_WAIT 3, multiplier=1.5, jitter=1s:
- attempt 1: 3 × 1.0 + 1 = **4s**
- attempt 2: 3 × 1.5 + 1 = **5.5s**  
- attempt 3: 3 × 2.25 + 1 = **7.75s**

### Proactive throttle in application layer (sync/engine.go)

A configurable inter-page delay (`SYNC_HISTORY_REQUEST_DELAY`, default 200ms) is inserted
between consecutive `GetHistory` calls for the same dialog. This reduces the probability of
hitting FLOOD_WAIT in the first place, without needing infrastructure knowledge at the
application layer.

### Log levels

- FLOOD_WAIT before max retries → `WARN` (expected operational condition)
- Retry succeeded → `INFO`
- Max retries exhausted → dialog sync marked `failed` in engine, logged at `ERROR`

---

## Error type used

`tgerr.AsFloodWait(err)` from `github.com/gotd/td/tgerr` — returns the wait duration
and a boolean. Handles both `FLOOD_WAIT` and `FLOOD_PREMIUM_WAIT` variants.

```go
// tgerr.AsFloodWait signature:
func AsFloodWait(err error) (d time.Duration, ok bool)
```

The underlying `*tgerr.Error` has fields:
- `Code int` — always 420 for flood wait
- `Type string` — "FLOOD_WAIT" or "FLOOD_PREMIUM_WAIT"
- `Argument int` — the N seconds to wait

---

## Example log output

```
level=WARN msg="history flood wait"
  chat_id=1234567890 offset=95000 wait_seconds=3 sleep=4s attempt=1

level=WARN msg="history flood wait"
  chat_id=1234567890 offset=95000 wait_seconds=3 sleep=5.5s attempt=2

level=INFO msg="history retry succeeded"
  chat_id=1234567890 offset=95000 attempt=3
```

---

## Consequences

**Positive**:
- Large dialogs are no longer silently dropped on FLOOD_WAIT
- Backfills complete reliably even on accounts with many active dialogs
- FLOOD_WAIT is correctly treated as an operational condition, not an error

**Negative / trade-offs**:
- A dialog with persistent FLOOD_WAIT (all 5 retries hit it) can delay the backfill
  significantly. With default settings and 3s flood waits, worst case per page: ~27s.
  Across 10 pages this is ~270s extra for one dialog.

**Mitigation**: `SYNC_FLOOD_MAX_RETRIES` and `SYNC_HISTORY_REQUEST_DELAY` are env-var
configurable so operators can tune for their account's rate limit profile.

---

## Alternatives considered

**Gotd/td built-in flood handling**: gotd/td handles FLOOD_WAIT at the connection level
for some methods, but not for `MessagesGetHistory` at the application-response level. We
cannot rely on it for pagination retries.

**Global backoff / token bucket**: A shared rate limiter across all dialogs would be more
efficient but adds shared state, complexity, and makes testing harder. The per-request retry
loop is simpler and good enough for sequential dialog processing.
