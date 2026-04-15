package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ShowFile is the detailed metadata returned by Show.
type ShowFile struct {
	Path          string
	Source        string
	CanonicalPath *string
	ContentHash   string
	Size          int64
	MimeType      string
	Modality      string
	FileExt       string
	EmbedModel    string
	ChunkCount    int
	IndexedAt     time.Time
	ModifiedAt    *time.Time
	DeletedAt     *time.Time
}

// Show returns detailed metadata for a single file path.
func Show(ctx context.Context, path string) (*ShowFile, error) {
	var f ShowFile
	err := pool.QueryRow(ctx, `
		SELECT
			path,
			source,
			canonical_path,
			content_hash,
			size,
			mime_type,
			modality,
			file_ext,
			embed_model,
			(SELECT COUNT(*) FROM files c WHERE c.path = files.path) AS chunk_count,
			indexed_at,
			file_modified_at,
			deleted_at
		FROM files
		WHERE path = $1
		  AND chunk_index = 0
	`, path).Scan(
		&f.Path, &f.Source, &f.CanonicalPath,
		&f.ContentHash, &f.Size, &f.MimeType,
		&f.Modality, &f.FileExt, &f.EmbedModel,
		&f.ChunkCount, &f.IndexedAt, &f.ModifiedAt,
		&f.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("not found: %s", path)
	}
	if err != nil {
		return nil, fmt.Errorf("show: %w", err)
	}
	return &f, nil
}
