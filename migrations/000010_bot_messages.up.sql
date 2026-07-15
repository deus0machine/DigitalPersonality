-- Phase 6.2: bot conversation log.
-- Records every message exchanged with the persona bot: incoming messages
-- from any interlocutor and the persona's replies.
--
-- user_id is the interlocutor's Telegram id on BOTH directions, so a whole
-- dialog is fetched by one user_id. These are conversations with the bot
-- (Bot API), fully separate from the personal-account memory in messages.

CREATE TABLE bot_messages (
    id           BIGSERIAL   PRIMARY KEY,
    chat_id      BIGINT      NOT NULL,
    user_id      BIGINT      NOT NULL,
    username     TEXT        NOT NULL DEFAULT '',
    from_persona BOOLEAN     NOT NULL,
    text         TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX bot_messages_user_time ON bot_messages (user_id, created_at);
CREATE INDEX bot_messages_time ON bot_messages (created_at);
