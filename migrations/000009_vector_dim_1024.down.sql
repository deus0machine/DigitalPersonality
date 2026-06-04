DROP INDEX IF EXISTS utterance_embeddings_hnsw;

ALTER TABLE utterance_embeddings
    ALTER COLUMN vector TYPE vector(1536);

CREATE INDEX utterance_embeddings_hnsw
    ON utterance_embeddings
    USING hnsw (vector vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
