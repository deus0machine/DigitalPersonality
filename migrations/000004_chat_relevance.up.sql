-- Chat relevance scoring for personality-focused ingestion.
--
-- Every chat gets a relevance_score (0.0–1.0) and a personality_surface label
-- computed by the ChatRelevanceScorer at sync time.
--
-- This enables SQL inspection of why each chat was included/excluded,
-- and supports future features like weighted retrieval and memory prioritization.

ALTER TABLE chats
    ADD COLUMN IF NOT EXISTS relevance_score     REAL         NOT NULL DEFAULT 0.0
        CHECK (relevance_score BETWEEN 0.0 AND 1.0),
    ADD COLUMN IF NOT EXISTS relevance_reason    TEXT         NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS personality_surface TEXT         NOT NULL DEFAULT '';

-- Fast lookup: find high-value chats for memory retrieval.
CREATE INDEX IF NOT EXISTS idx_chats_relevance
    ON chats (relevance_score DESC);

-- Fast lookup: find chats by personality surface type.
CREATE INDEX IF NOT EXISTS idx_chats_surface
    ON chats (personality_surface)
    WHERE personality_surface != '';
