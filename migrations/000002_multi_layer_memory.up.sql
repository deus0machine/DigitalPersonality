-- Multi-layer memory architecture.
--
-- Layer 1 (Raw):     messages + enriched personality-relevant columns
-- Layer 2 (Semantic): message_semantic — normalized text for embedding
-- Layer 3 (Personality): personality_signals — per-message extracted features
--
-- Normalization NEVER modifies raw data.
-- Short/emoji-only messages are stored raw and get personality signals
-- but may be skipped for embedding (skip_embedding = TRUE).

-- ─── Layer 1 additions: enrich messages with personality metadata ────────────

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS entities    JSONB,          -- text formatting: bold/italic/links/mentions
    ADD COLUMN IF NOT EXISTS reactions   JSONB,          -- emoji reactions + counts
    ADD COLUMN IF NOT EXISTS sticker_meta JSONB,         -- sticker set + associated emoticon
    ADD COLUMN IF NOT EXISTS media_kind  TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_messages_media_kind ON messages (media_kind)
    WHERE media_kind != '';

CREATE INDEX IF NOT EXISTS idx_messages_outgoing_chat
    ON messages (chat_id, sent_at DESC)
    WHERE is_outgoing = TRUE;  -- fast query for personality analysis on self messages

-- ─── Layer 2: Semantic documents ─────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS message_semantic (
    message_id      BIGINT PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    -- normalized_text may differ significantly from raw text:
    -- emoji stripped, lowercased, whitespace normalized.
    -- Used exclusively for embedding generation and semantic search.
    normalized_text TEXT NOT NULL DEFAULT '',
    language        TEXT,                                -- 'ru' | 'en' | 'mixed' | null
    token_count     INT NOT NULL DEFAULT 0,             -- whitespace-tokenized word count
    -- skip_embedding = TRUE for stickers, very short msgs, pure-emoji, etc.
    -- These are valuable for personality but not for semantic retrieval.
    skip_embedding  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_message_semantic_skip
    ON message_semantic (skip_embedding, message_id)
    WHERE skip_embedding = FALSE;  -- fast lookup of messages pending embedding

-- ─── Layer 3: Personality signals ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS personality_signals (
    id              BIGSERIAL PRIMARY KEY,
    message_id      BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    -- signal_type is an extensible enum stored as text.
    -- Values: emoji_usage | punctuation_style | capitalization |
    --         length_class | media_kind | sticker_usage | slang_markers
    signal_type     TEXT NOT NULL,
    value_json      JSONB NOT NULL,
    extracted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (message_id, signal_type)
);

CREATE INDEX IF NOT EXISTS idx_personality_signals_type
    ON personality_signals (signal_type, extracted_at DESC);

CREATE INDEX IF NOT EXISTS idx_personality_signals_message
    ON personality_signals (message_id);

-- ─── Personality profiles: aggregated per user ───────────────────────────────

CREATE TABLE IF NOT EXISTS personality_profiles (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    -- features is a flexible JSONB blob that grows as we add more signals.
    -- Schema: {
    --   "top_emoji": [{"emoji": "😂", "count": 42}],
    --   "avg_message_length": 24.3,
    --   "short_reply_rate": 0.45,
    --   "emoji_density": 1.2,
    --   "uppercase_rate": 0.03,
    --   "favorite_phrases": ["ахах", "ок", "да"],
    --   "media_breakdown": {"sticker": 120, "photo": 30}
    -- }
    features        JSONB NOT NULL DEFAULT '{}',
    signal_count    BIGINT NOT NULL DEFAULT 0,          -- total signals processed
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
