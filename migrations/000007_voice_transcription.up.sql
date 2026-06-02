-- Phase 5.1: Voice Transcription Pipeline
--
-- 1. chats.access_hash — required to reconstruct InputPeer after restart.
--    account-level, not session-level: persists across re-authentication.
--    ChatTypeGroup uses InputPeerChat (ID only) so 0 is correct default.
--
-- 2. message_semantic.transcribed_at — idempotent transcription checkpoint.
--    NULL  = not yet attempted.
--    NOT NULL = processed (transcript stored, or permanent failure recorded).
--    Worker queue: WHERE transcribed_at IS NULL.
--    No index added: existing idx_messages_media_kind on messages.media_kind
--    already filters to ~1723 voice rows; join to message_semantic is on PK.

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS access_hash BIGINT NOT NULL DEFAULT 0;

ALTER TABLE message_semantic
    ADD COLUMN IF NOT EXISTS transcribed_at TIMESTAMPTZ;
