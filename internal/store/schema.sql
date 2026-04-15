CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS files (
    id               BIGSERIAL PRIMARY KEY,
    path             TEXT NOT NULL UNIQUE,
    source           TEXT NOT NULL DEFAULT 'local',
    canonical_path   TEXT,
    modality         TEXT NOT NULL,
    file_name        TEXT,
    file_ext         TEXT,
    mime_type        TEXT,
    size             BIGINT,
    content_hash     TEXT,
    file_created_at  TIMESTAMPTZ,
    file_modified_at TIMESTAMPTZ,
    indexed_at       TIMESTAMPTZ DEFAULT now(),
    deleted_at       TIMESTAMPTZ,
    metadata         JSONB
);

CREATE TABLE IF NOT EXISTS items (
    id           BIGSERIAL PRIMARY KEY,
    file_id      BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    item_type    TEXT NOT NULL,
    item_name    TEXT,
    mime_type    TEXT,
    size         BIGINT,
    content_hash TEXT,
    item_index   INT NOT NULL DEFAULT 0,
    metadata     JSONB
);

CREATE TABLE IF NOT EXISTS chunks (
    id           BIGSERIAL PRIMARY KEY,
    item_id      BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    chunk_index  INT NOT NULL,
    chunk_type   TEXT,
    embed_model  TEXT,
    embedding    vector(%%EMBEDDING_DIM%%),
    text_content TEXT,
    metadata     JSONB,
    indexed_at   TIMESTAMPTZ DEFAULT now(),

    UNIQUE (item_id, chunk_index)
);

-- chunk search indexes
CREATE INDEX IF NOT EXISTS chunks_embedding_idx ON chunks
    USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;

CREATE INDEX IF NOT EXISTS chunks_fts_idx ON chunks
    USING gin(to_tsvector('english', text_content))
    WHERE text_content IS NOT NULL;

-- file filter indexes
CREATE INDEX IF NOT EXISTS files_modified_at_idx ON files (file_modified_at)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS files_ext_idx ON files (file_ext)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS files_source_idx ON files (source)
    WHERE deleted_at IS NULL;

-- join indexes
CREATE INDEX IF NOT EXISTS items_file_id_idx ON items (file_id);
CREATE INDEX IF NOT EXISTS chunks_item_id_idx ON chunks (item_id);

-- create view

CREATE OR REPLACE VIEW file_chunks AS
SELECT
    f.id          AS file_id,
    f.path,
    f.source,
    f.canonical_path,
    f.content_hash,
    f.size,
    f.mime_type,
    f.modality,
    f.file_name,
    f.file_ext,
    f.file_created_at,
    f.file_modified_at,
    f.indexed_at,
    f.deleted_at,
    f.metadata    AS file_metadata,
    i.id          AS item_id,
    i.item_type,
    i.item_name,
    i.item_index,
    c.id          AS chunk_id,
    c.chunk_index,
    c.chunk_type,
    c.embed_model,
    c.embedding,
    c.text_content,
    c.metadata    AS chunk_metadata,
    c.indexed_at  AS chunk_indexed_at
FROM files f
JOIN items i ON i.file_id = f.id
JOIN chunks c ON c.item_id = i.id;