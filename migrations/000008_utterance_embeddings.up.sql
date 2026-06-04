-- Phase 5.3: Utterance Embeddings
--
-- Stores one vector per utterance, keyed by the first message's DB id.
-- first_message_id is a stable identifier because:
--   1. Build() is deterministic for a fixed UTTERANCE_GAP_SECONDS.
--   2. New messages only extend or follow existing utterances — they do not
--      insert between existing messages of the same author within a gap.
--
-- If UTTERANCE_GAP_SECONDS changes, all stored embeddings become stale.
-- Remedy: TRUNCATE utterance_embeddings; then re-run embed-utterances.
-- The gap_seconds column allows workers to detect this mismatch.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE utterance_embeddings (
    first_message_id BIGINT       PRIMARY KEY REFERENCES messages(id),
    model_name       VARCHAR(100) NOT NULL,
    gap_seconds      INT          NOT NULL,
    embedded_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    vector           vector(1536)
);

CREATE INDEX utterance_embeddings_hnsw
    ON utterance_embeddings
    USING hnsw (vector vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
