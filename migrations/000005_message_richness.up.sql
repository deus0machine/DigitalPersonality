-- Message richness: forward metadata, edit tracking, and retrieval indexes.
--
-- Forward metadata tells us what content the user curates and shares —
-- a strong personality signal (worldview, taste, affiliations).
--
-- FTS + trigram indexes enable PostgreSQL-native retrieval without OpenAI:
--   - Full-text search: find episodes/messages containing specific words/phrases
--   - Trigram similarity: fuzzy-match characteristic phrases and writing patterns
--
-- This is the retrieval foundation layer. Embedding-based retrieval comes later
-- and will be additive — these indexes remain useful for metadata filtering.

-- ─── Forward and edit metadata ────────────────────────────────────────────────

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS is_forwarded    BOOLEAN      NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS forward_from_id BIGINT,              -- original sender; NULL = anonymous/channel
    ADD COLUMN IF NOT EXISTS forward_date    TIMESTAMPTZ,         -- original send timestamp
    ADD COLUMN IF NOT EXISTS edit_date       TIMESTAMPTZ;         -- NULL = never edited

-- ─── Full-text search on messages ─────────────────────────────────────────────
-- Generated tsvector column: updated automatically on any text change.
-- 'simple' config: tokenizes and lowercases without stemming —
--   works for both Russian and English without language detection.
--   Trigram handles fuzzy matching; FTS handles exact word/phrase retrieval.

ALTER TABLE messages ADD COLUMN IF NOT EXISTS text_search tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple', COALESCE(text, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_messages_fts
    ON messages USING GIN (text_search);

-- ─── Trigram similarity on messages ───────────────────────────────────────────
-- Enables: similarity('запрос', text) > threshold
-- Use case: find characteristic phrases, detect recurring language patterns.

CREATE INDEX IF NOT EXISTS idx_messages_trgm
    ON messages USING GIN (text gin_trgm_ops)
    WHERE text IS NOT NULL AND length(text) > 0;

-- ─── Full-text search on episode semantic documents ───────────────────────────
-- Episode-level FTS enables retrieval of whole conversational contexts
-- rather than individual messages — better for autobiographical memory retrieval.

ALTER TABLE episode_semantic ADD COLUMN IF NOT EXISTS text_search tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple', COALESCE(semantic_text, ''))
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_episode_semantic_fts
    ON episode_semantic USING GIN (text_search);

-- ─── Supporting indexes for retrieval filtering ───────────────────────────────

-- Temporal window queries: "what did the user write in the evenings?"
CREATE INDEX IF NOT EXISTS idx_messages_sent_outgoing
    ON messages (sent_at DESC, is_outgoing)
    WHERE is_deleted = FALSE;

-- Forward analysis: "what content does the user forward?"
CREATE INDEX IF NOT EXISTS idx_messages_forwarded
    ON messages (chat_id, forward_date DESC)
    WHERE is_forwarded = TRUE;

-- Edited message analysis: "how often does the user edit messages?"
CREATE INDEX IF NOT EXISTS idx_messages_edited
    ON messages (edit_date DESC)
    WHERE edit_date IS NOT NULL;
