DROP TABLE IF EXISTS personality_profiles;
DROP INDEX IF EXISTS idx_personality_signals_message;
DROP INDEX IF EXISTS idx_personality_signals_type;
DROP TABLE IF EXISTS personality_signals;
DROP INDEX IF EXISTS idx_message_semantic_skip;
DROP TABLE IF EXISTS message_semantic;
DROP INDEX IF EXISTS idx_messages_outgoing_chat;
DROP INDEX IF EXISTS idx_messages_media_kind;
ALTER TABLE messages
    DROP COLUMN IF EXISTS media_kind,
    DROP COLUMN IF EXISTS sticker_meta,
    DROP COLUMN IF EXISTS reactions,
    DROP COLUMN IF EXISTS entities;
