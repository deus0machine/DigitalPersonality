-- Users: Telegram accounts we track
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT PRIMARY KEY,           -- Telegram user ID
    username    TEXT,
    first_name  TEXT NOT NULL DEFAULT '',
    last_name   TEXT NOT NULL DEFAULT '',
    phone       TEXT,
    is_self     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users (username) WHERE username IS NOT NULL;

-- Chats: Telegram chats/channels/dialogs
CREATE TABLE IF NOT EXISTS chats (
    id          BIGINT PRIMARY KEY,           -- Telegram chat ID
    type        TEXT NOT NULL,               -- 'private' | 'group' | 'supergroup' | 'channel'
    title       TEXT,
    username    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Messages: raw synchronized messages
CREATE TABLE IF NOT EXISTS messages (
    id              BIGSERIAL PRIMARY KEY,
    telegram_id     BIGINT NOT NULL,
    chat_id         BIGINT NOT NULL REFERENCES chats(id),
    sender_id       BIGINT REFERENCES users(id),
    reply_to_id     BIGINT,                  -- telegram_id of parent message
    text            TEXT NOT NULL DEFAULT '',
    raw_data        JSONB,                   -- full telegram message payload
    sent_at         TIMESTAMPTZ NOT NULL,
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_outgoing     BOOLEAN NOT NULL DEFAULT FALSE,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (telegram_id, chat_id)
);

CREATE INDEX IF NOT EXISTS idx_messages_chat_id     ON messages (chat_id);
CREATE INDEX IF NOT EXISTS idx_messages_sender_id   ON messages (sender_id);
CREATE INDEX IF NOT EXISTS idx_messages_sent_at     ON messages (sent_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_text_trgm   ON messages USING gin (text gin_trgm_ops);

-- Sync cursors: tracks last synced position per chat
CREATE TABLE IF NOT EXISTS sync_cursors (
    chat_id         BIGINT PRIMARY KEY REFERENCES chats(id),
    last_message_id BIGINT NOT NULL DEFAULT 0,
    synced_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Embeddings: vector representations of messages
CREATE TABLE IF NOT EXISTS embeddings (
    id          BIGSERIAL PRIMARY KEY,
    message_id  BIGINT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    model       TEXT NOT NULL,              -- embedding model identifier
    vector      vector(1536) NOT NULL,      -- OpenAI text-embedding-3-small dimension
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (message_id, model)
);

-- IVFFlat index for approximate nearest-neighbor search.
-- Lists = sqrt(rows). Rebuild after bulk ingest.
CREATE INDEX IF NOT EXISTS idx_embeddings_vector
    ON embeddings USING ivfflat (vector vector_cosine_ops)
    WITH (lists = 100);
