DROP INDEX IF EXISTS idx_messages_edited;
DROP INDEX IF EXISTS idx_messages_forwarded;
DROP INDEX IF EXISTS idx_messages_sent_outgoing;
DROP INDEX IF EXISTS idx_episode_semantic_fts;
ALTER TABLE episode_semantic DROP COLUMN IF EXISTS text_search;
DROP INDEX IF EXISTS idx_messages_trgm;
DROP INDEX IF EXISTS idx_messages_fts;
ALTER TABLE messages DROP COLUMN IF EXISTS text_search;
ALTER TABLE messages
    DROP COLUMN IF EXISTS edit_date,
    DROP COLUMN IF EXISTS forward_date,
    DROP COLUMN IF EXISTS forward_from_id,
    DROP COLUMN IF EXISTS is_forwarded;
