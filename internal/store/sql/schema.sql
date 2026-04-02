CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS files (
    -- identity
    id                BIGSERIAL PRIMARY KEY,
    path              TEXT NOT NULL,
    source            TEXT NOT NULL DEFAULT 'local',  -- 'local' | 's3://...'
    canonical_path    TEXT,                           -- non-NULL means this is a duplicate of canonical_path

    -- content
    content_hash      TEXT NOT NULL,
    size              BIGINT,
    mime_type         TEXT,
    modality          TEXT NOT NULL,                  -- 'text' | 'image'

    -- filesystem metadata
    file_name         TEXT,                           -- basename, e.g. "report.pdf"
    file_ext          TEXT,                           -- lowercased extension, e.g. "pdf"
    file_created_at   TIMESTAMPTZ,                    -- birth time (where OS supports it)
    file_modified_at  TIMESTAMPTZ,                    -- mtime from filesystem

    -- embedding
    embed_model       TEXT NOT NULL,
    embedding         vector(%%EMBEDDING_DIM%%),
    chunk_index       INT NOT NULL DEFAULT 0,
    chunk_type        TEXT,                           -- NULL = text/image, 'frame' = video frame, 'transcript' = audio/video transcript

    -- rich metadata (EXIF, page count, dimensions, etc.)
    metadata          JSONB,

    -- raw extracted text
    text_content      TEXT,

    -- housekeeping
    indexed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);

-- one row per chunk per file
CREATE UNIQUE INDEX IF NOT EXISTS files_path_chunk_idx
    ON files (path, chunk_index);

-- embedding similarity search (excludes deleted and duplicate rows)
CREATE INDEX IF NOT EXISTS files_embedding_idx
    ON files USING hnsw (embedding vector_cosine_ops)
    WHERE deleted_at IS NULL AND canonical_path IS NULL;

-- common filter indexes
CREATE INDEX IF NOT EXISTS files_modified_at_idx
    ON files (file_modified_at)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS files_ext_idx
    ON files (file_ext)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS files_source_idx
    ON files (source)
    WHERE deleted_at IS NULL;
