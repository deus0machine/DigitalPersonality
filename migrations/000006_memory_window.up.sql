-- Participation-centered memory windows for group/channel dialogs.
--
-- in_memory_window = TRUE  → message is within a participation window:
--   outgoing anchor ±N neighbours, or direct reply target of an outgoing message.
--   These messages flow through Layers 2-4 (semantic, personality, episodic).
--
-- in_memory_window = FALSE → distant group chatter outside any participation window.
--   Stored in Layer 1 (raw) for inspectability and future reprocessing,
--   but excluded from semantic/personality/episodic pipelines.
--
-- DEFAULT TRUE ensures zero breaking change:
--   - existing messages remain fully active in all layers
--   - full-sync surfaces (interpersonal, self_expression, tool_interaction)
--     never have their flag set to FALSE

ALTER TABLE messages
    ADD COLUMN in_memory_window BOOLEAN NOT NULL DEFAULT TRUE;

-- Partial index for window computation queries (scanning outside-window messages).
CREATE INDEX IF NOT EXISTS idx_messages_not_in_window
    ON messages (chat_id, sent_at ASC)
    WHERE NOT in_memory_window AND NOT is_deleted;

-- Partial index for Layer 2-4 rebuild: in-window messages without semantic entry.
CREATE INDEX IF NOT EXISTS idx_messages_window_active
    ON messages (chat_id, sent_at ASC)
    WHERE in_memory_window = TRUE AND is_deleted = FALSE;
