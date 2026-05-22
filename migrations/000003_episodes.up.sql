-- Episodic Memory Architecture.
--
-- Episodes are coherent conversational memory units — the chunks a human would
-- recall as "scenes" or "events", not individual messages.
--
-- Retrieval hierarchy:
--   message_semantic  → fine-grained semantic search
--   episode_semantic  → autobiographical / contextual memory retrieval
--   personality_signals → communication style reconstruction
--
-- An episode contains 1..N messages from the same chat, segmented by:
--   time gaps, reply chain continuity, participant patterns, size limits.

-- ─── Core episode table ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS episodes (
    id              BIGSERIAL PRIMARY KEY,
    chat_id         BIGINT NOT NULL REFERENCES chats(id),

    -- Temporal span of the episode
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ NOT NULL,
    duration_secs   INT GENERATED ALWAYS AS
                        (EXTRACT(EPOCH FROM (ended_at - started_at))::INT) STORED,

    -- Classification
    episode_type    TEXT NOT NULL,           -- dialogue|monologue|burst|thread|async
    message_count   INT NOT NULL DEFAULT 0,
    participant_ids BIGINT[] NOT NULL DEFAULT '{}',

    -- Segmentation provenance — why was this boundary drawn?
    segmented_by    TEXT NOT NULL,           -- time_gap_hard|time_gap_medium|reply_chain|size_limit|...
    confidence      REAL NOT NULL DEFAULT 1.0 CHECK (confidence BETWEEN 0 AND 1),

    -- Future: importance / emotional weighting (null until computed)
    importance      REAL CHECK (importance BETWEEN 0 AND 1),
    emotional_valence TEXT,                  -- positive|negative|neutral|ambiguous

    -- Future: summarization (null until a summarizer runs)
    summary         TEXT,
    summary_model   TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_episodes_chat_time
    ON episodes (chat_id, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_episodes_type
    ON episodes (episode_type, chat_id);

CREATE INDEX IF NOT EXISTS idx_episodes_pending_importance
    ON episodes (importance NULLS FIRST)
    WHERE importance IS NULL;

-- ─── Message → Episode linking ────────────────────────────────────────────────
-- Each message belongs to at most one episode.
-- position preserves intra-episode ordering without relying on DB row order.

CREATE TABLE IF NOT EXISTS episode_messages (
    episode_id  BIGINT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    message_id  BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    position    INT NOT NULL,
    PRIMARY KEY (episode_id, message_id)
);

-- Fast lookup: "which episode does message X belong to?"
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_messages_msg
    ON episode_messages (message_id);

-- Fast lookup: "find messages not yet episoded in chat X"
-- Implemented as an anti-join in the application; this index supports it.
CREATE INDEX IF NOT EXISTS idx_episode_messages_episode
    ON episode_messages (episode_id, position);

-- ─── Semantic layer for episodes ─────────────────────────────────────────────
-- episode_semantic holds the composed text used to generate an episode embedding.
-- The semantic text is a direction-annotated concatenation of member messages:
--     → outgoing message text
--     ← incoming message text
--     ...
-- skip_embedding = TRUE for single-message or very-short episodes.

CREATE TABLE IF NOT EXISTS episode_semantic (
    episode_id      BIGINT PRIMARY KEY REFERENCES episodes(id) ON DELETE CASCADE,
    semantic_text   TEXT NOT NULL DEFAULT '',
    token_count     INT NOT NULL DEFAULT 0,
    skip_embedding  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_episode_semantic_pending
    ON episode_semantic (episode_id)
    WHERE skip_embedding = FALSE;

-- ─── Episode vector embeddings ────────────────────────────────────────────────
-- Kept separate from message-level embeddings to allow independent retrieval
-- strategies. Both tables are queried with pgvector cosine similarity.

CREATE TABLE IF NOT EXISTS episode_embeddings (
    id          BIGSERIAL PRIMARY KEY,
    episode_id  BIGINT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    model       TEXT NOT NULL,
    vector      vector(1536) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (episode_id, model)
);

-- IVFFlat ANN index — rebuild after bulk ingest with REINDEX.
CREATE INDEX IF NOT EXISTS idx_episode_embeddings_vector
    ON episode_embeddings USING ivfflat (vector vector_cosine_ops)
    WITH (lists = 50);
