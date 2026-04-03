package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// File represents a row in the files table.
type File struct {
	Path           string
	Source         string
	CanonicalPath  *string
	ContentHash    string
	Size           int64
	MimeType       string
	Modality       string
	FileName       string
	FileExt        string
	FileCreatedAt  *time.Time
	FileModifiedAt *time.Time
	EmbedModel     string
	Embedding      []float32
	ChunkIndex     int
	ChunkType      *string
	Metadata       map[string]any
	TextContent    *string // populated for text modality only
}

// Upsert inserts or updates a file row, including its embedding.
// Matches on (path, chunk_index).
func Upsert(ctx context.Context, q Querier, f File) error {
	var embedding *pgvector.Vector
	if f.Embedding != nil {
		v := pgvector.NewVector(f.Embedding)
		embedding = &v
	}

	_, err := q.Exec(ctx, `
		INSERT INTO files (
			path, source, canonical_path,
			content_hash, size, mime_type, modality,
			file_name, file_ext, file_created_at, file_modified_at,
			embed_model, embedding, chunk_index, chunk_type,
			metadata, text_content, indexed_at, deleted_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, $9, $10, $11,
			$12, $13, $14, $15,
			$16, $17, now(), NULL
		)
		ON CONFLICT (path, chunk_index) DO UPDATE SET
			source           = EXCLUDED.source,
			canonical_path   = EXCLUDED.canonical_path,
			content_hash     = EXCLUDED.content_hash,
			size             = EXCLUDED.size,
			mime_type        = EXCLUDED.mime_type,
			modality         = EXCLUDED.modality,
			file_name        = EXCLUDED.file_name,
			file_ext         = EXCLUDED.file_ext,
			file_created_at  = EXCLUDED.file_created_at,
			file_modified_at = EXCLUDED.file_modified_at,
			embed_model      = EXCLUDED.embed_model,
			embedding        = EXCLUDED.embedding,
			chunk_type   	 = EXCLUDED.chunk_type,
			metadata         = EXCLUDED.metadata,
			text_content 	 = EXCLUDED.text_content,
			indexed_at       = now(),
			deleted_at       = NULL
	`,
		f.Path, f.Source, f.CanonicalPath,
		f.ContentHash, f.Size, f.MimeType, f.Modality,
		f.FileName, f.FileExt, f.FileCreatedAt, f.FileModifiedAt,
		f.EmbedModel, embedding, f.ChunkIndex, f.ChunkType,
		f.Metadata, f.TextContent,
	)
	if err != nil {
		return fmt.Errorf("upsert %s: %w", f.Path, err)
	}
	return nil
}

// FindByHash returns the canonical path for a given content hash, if one exists.
// Used for deduplication — if a hash is already indexed, the new file is a duplicate.
func FindByHash(ctx context.Context, q Querier, hash string) (string, bool, error) {
	var path string
	err := q.QueryRow(ctx, `
		SELECT path FROM files
		WHERE content_hash = $1
		  AND canonical_path IS NULL
		  AND deleted_at IS NULL
		LIMIT 1
	`, hash).Scan(&path)

	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find by hash: %w", err)
	}
	return path, true, nil
}

// UpsertDuplicate inserts a file row that points to an existing canonical path.
// No embedding is stored — the canonical row owns the vector.
func UpsertDuplicate(ctx context.Context, q Querier, f File, canonicalPath string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO files (
			path, source, canonical_path,
			content_hash, size, mime_type, modality,
			file_name, file_ext, file_created_at, file_modified_at,
			embed_model, chunk_index, indexed_at, deleted_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, $9, $10, $11,
			$12, 0, now(), NULL
		)
		ON CONFLICT (path, chunk_index) DO UPDATE SET
			canonical_path   = EXCLUDED.canonical_path,
			content_hash     = EXCLUDED.content_hash,
			indexed_at       = now(),
			deleted_at       = NULL
	`,
		f.Path, f.Source, canonicalPath,
		f.ContentHash, f.Size, f.MimeType, f.Modality,
		f.FileName, f.FileExt, f.FileCreatedAt, f.FileModifiedAt,
		f.EmbedModel,
	)
	if err != nil {
		return fmt.Errorf("upsert duplicate %s: %w", f.Path, err)
	}
	return nil
}
