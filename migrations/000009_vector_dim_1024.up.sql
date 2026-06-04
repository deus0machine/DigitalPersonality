-- Phase 5.3: resize embedding vector from 1536 (OpenAI) to 1024 (BGE-M3 via Ollama).
-- Safe: utterance_embeddings is empty at this point — no data migration needed.

ALTER TABLE utterance_embeddings
    ALTER COLUMN vector TYPE vector(1024);

DROP INDEX IF EXISTS utterance_embeddings_hnsw;

CREATE INDEX utterance_embeddings_hnsw
    ON utterance_embeddings
    USING hnsw (vector vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
