DROP INDEX IF EXISTS idx_chats_surface;
DROP INDEX IF EXISTS idx_chats_relevance;
ALTER TABLE chats
    DROP COLUMN IF EXISTS personality_surface,
    DROP COLUMN IF EXISTS relevance_reason,
    DROP COLUMN IF EXISTS relevance_score;
