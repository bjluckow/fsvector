-- migration.sql

-- Step 1: rename old table
ALTER TABLE files RENAME TO files_old;

-- Step 2: create new tables
CREATE TABLE files (
    id               BIGSERIAL PRIMARY KEY,
    path             TEXT NOT NULL UNIQUE,
    source           TEXT NOT NULL,
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

CREATE TABLE items (
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

CREATE TABLE chunks (
    id           BIGSERIAL PRIMARY KEY,
    item_id      BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    chunk_index  INT NOT NULL,
    chunk_type   TEXT,
    embed_model  TEXT,
    embedding    vector(512),
    text_content TEXT,
    metadata     JSONB,
    indexed_at   TIMESTAMPTZ DEFAULT now(),
    UNIQUE (item_id, chunk_index)
);

CREATE INDEX chunks_embedding_idx ON chunks
    USING hnsw (embedding vector_cosine_ops)
    WHERE embedding IS NOT NULL;

CREATE INDEX chunks_fts_idx ON chunks
    USING gin(to_tsvector('english', text_content))
    WHERE text_content IS NOT NULL;

CREATE INDEX files_modified_at_idx ON files (file_modified_at)
    WHERE deleted_at IS NULL;
CREATE INDEX files_ext_idx ON files (file_ext)
    WHERE deleted_at IS NULL;
CREATE INDEX files_source_idx ON files (source)
    WHERE deleted_at IS NULL;
CREATE INDEX items_file_id_idx ON items (file_id);
CREATE INDEX chunks_item_id_idx ON chunks (item_id);

-- Step 3: migrate files (one row per distinct path)
INSERT INTO files (
    path, source, canonical_path, modality,
    file_name, file_ext, mime_type, size, content_hash,
    file_created_at, file_modified_at, indexed_at, deleted_at
)
SELECT DISTINCT ON (path)
    path, source, canonical_path, modality,
    file_name, file_ext, mime_type, size, content_hash,
    file_created_at, file_modified_at, indexed_at, deleted_at
FROM files_old
ORDER BY path, chunk_index;

-- Step 4: create items

-- whole item for text and image (no special chunk types)
INSERT INTO items (file_id, item_type, item_index)
SELECT f.id, 'whole', 0
FROM files f
WHERE NOT EXISTS (
    SELECT 1 FROM files_old fo
    WHERE fo.path = f.path
    AND fo.chunk_type IN ('frame', 'transcript')
);

-- frames item for video
INSERT INTO items (file_id, item_type, item_index)
SELECT DISTINCT f.id, 'frames', 0
FROM files f
JOIN files_old fo ON fo.path = f.path
WHERE fo.chunk_type = 'frame';

-- transcript item for video and audio
INSERT INTO items (file_id, item_type, item_index)
SELECT DISTINCT f.id, 'transcript',
    CASE WHEN EXISTS (
        SELECT 1 FROM files_old fo2
        WHERE fo2.path = f.path AND fo2.chunk_type = 'frame'
    ) THEN 1 ELSE 0 END
FROM files f
JOIN files_old fo ON fo.path = f.path
WHERE fo.chunk_type = 'transcript';

-- Step 5: migrate chunks
INSERT INTO chunks (
    item_id, chunk_index, chunk_type, embed_model,
    embedding, text_content, metadata, indexed_at
)
SELECT
    i.id,
    ROW_NUMBER() OVER (PARTITION BY i.id ORDER BY fo.chunk_index) - 1,
    fo.chunk_type,
    fo.embed_model,
    fo.embedding,
    fo.text_content,
    fo.metadata,
    fo.indexed_at
FROM files_old fo
JOIN files f ON f.path = fo.path
JOIN items i ON i.file_id = f.id
WHERE (
    (fo.chunk_type = 'frame'      AND i.item_type = 'frames') OR
    (fo.chunk_type = 'transcript' AND i.item_type = 'transcript') OR
    (fo.chunk_type IS NULL        AND i.item_type = 'whole') OR
    (fo.chunk_type = 'ocr'        AND i.item_type = 'whole')
);