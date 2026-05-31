DROP INDEX IF EXISTS idx_messages_window_active;
DROP INDEX IF EXISTS idx_messages_not_in_window;
ALTER TABLE messages DROP COLUMN IF EXISTS in_memory_window;
