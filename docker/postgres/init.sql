-- Enable required extensions on DB initialization.
-- pgvector is installed by the pgvector/pgvector Docker image.
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pg_trgm;
