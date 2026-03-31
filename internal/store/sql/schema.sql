CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS schema_version (
    version     INT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS models (
    modality    TEXT NOT NULL,
    model_name  TEXT NOT NULL,
    dim         INT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'ready',
    is_active   BOOLEAN NOT NULL DEFAULT false,
    indexed_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (modality, model_name)
);

CREATE UNIQUE INDEX IF NOT EXISTS models_active_modality_idx
    ON models (modality)
    WHERE is_active = true;

CREATE TABLE IF NOT EXISTS files (
    id                BIGSERIAL PRIMARY KEY,
    path              TEXT NOT NULL,
    source            TEXT NOT NULL DEFAULT 'local',
    canonical_path    TEXT,
    content_hash      TEXT NOT NULL,
    size              BIGINT,
    mime_type         TEXT,
    modality          TEXT NOT NULL,
    file_name         TEXT,
    file_ext          TEXT,
    file_created_at   TIMESTAMPTZ,
    file_modified_at  TIMESTAMPTZ,
    embed_model       TEXT NOT NULL,
    embedding         vector(%%EMBEDDING_DIM%%),
    chunk_index       INT NOT NULL DEFAULT 0,
    metadata          JSONB,
    indexed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    retired_at        TIMESTAMPTZ,
    CONSTRAINT files_embed_model_fk
        FOREIGN KEY (modality, embed_model)
        REFERENCES models (modality, model_name)
        DEFERRABLE INITIALLY DEFERRED
);

CREATE UNIQUE INDEX IF NOT EXISTS files_path_chunk_model_idx
    ON files (path, chunk_index, embed_model);

CREATE UNIQUE INDEX IF NOT EXISTS files_canonical_hash_idx
    ON files (content_hash)
    WHERE canonical_path IS NULL
      AND deleted_at IS NULL
      AND retired_at IS NULL;

CREATE INDEX IF NOT EXISTS files_embedding_idx
    ON files USING hnsw (embedding vector_cosine_ops)
    WHERE deleted_at IS NULL
      AND retired_at IS NULL
      AND canonical_path IS NULL;

CREATE INDEX IF NOT EXISTS files_modified_at_idx
    ON files (file_modified_at)
    WHERE deleted_at IS NULL AND retired_at IS NULL;

CREATE INDEX IF NOT EXISTS files_ext_idx
    ON files (file_ext)
    WHERE deleted_at IS NULL AND retired_at IS NULL;

CREATE INDEX IF NOT EXISTS files_source_idx
    ON files (source)
    WHERE deleted_at IS NULL AND retired_at IS NULL;