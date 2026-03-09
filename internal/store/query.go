package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// SearchResult is a single result from a similarity search.
type SearchResult struct {
	Path      string
	Modality  string
	FileExt   string
	Size      int64
	Score     float64
	IndexedAt time.Time
	DeletedAt *time.Time
}

// Search performs a cosine similarity search against live, canonical files.
func Search(ctx context.Context, conn *pgx.Conn, queryVec []float32, limit int) ([]SearchResult, error) {
	v := pgvector.NewVector(queryVec)
	rows, err := conn.Query(ctx, `
		SELECT
			path,
			modality,
			file_ext,
			size,
			1 - (embedding <=> $1) AS score,
			indexed_at
		FROM files
		WHERE deleted_at IS NULL
		  AND canonical_path IS NULL
		  AND embedding IS NOT NULL
		ORDER BY embedding <=> $1
		LIMIT $2
	`, v, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.Path, &r.Modality, &r.FileExt,
			&r.Size, &r.Score, &r.IndexedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListFile is a single row returned by Ls.
type ListFile struct {
	Path        string
	Modality    string
	FileExt     string
	Size        int64
	IndexedAt   time.Time
	ModifiedAt  *time.Time
	DeletedAt   *time.Time
	IsDuplicate bool
}

// Ls returns all indexed files, optionally including deleted ones.
func Ls(ctx context.Context, conn *pgx.Conn, includeDeleted bool) ([]ListFile, error) {
	query := `
		SELECT
			path,
			modality,
			file_ext,
			size,
			indexed_at,
			file_modified_at,
			deleted_at,
			canonical_path IS NOT NULL AS is_duplicate
		FROM files
		WHERE chunk_index = 0
	`
	if !includeDeleted {
		query += " AND deleted_at IS NULL"
	}
	query += " ORDER BY path"

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ls: %w", err)
	}
	defer rows.Close()

	var files []ListFile
	for rows.Next() {
		var f ListFile
		if err := rows.Scan(
			&f.Path, &f.Modality, &f.FileExt,
			&f.Size, &f.IndexedAt, &f.ModifiedAt,
			&f.DeletedAt, &f.IsDuplicate,
		); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

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
func Show(ctx context.Context, conn *pgx.Conn, path string) (*ShowFile, error) {
	var f ShowFile
	err := conn.QueryRow(ctx, `
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

// Stats holds index-wide statistics.
type Stats struct {
	TotalFiles   int
	DeletedFiles int
	Duplicates   int
	TextFiles    int
	ImageFiles   int
	EmbedModel   string
}

// GetStats returns aggregate statistics about the index.
func GetStats(ctx context.Context, conn *pgx.Conn) (*Stats, error) {
	var s Stats
	err := conn.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE chunk_index = 0)                            AS total,
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL AND chunk_index = 0) AS deleted,
			COUNT(*) FILTER (WHERE canonical_path IS NOT NULL)                 AS duplicates,
			COUNT(*) FILTER (WHERE modality = 'text'   AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS text_files,
			COUNT(*) FILTER (WHERE modality = 'image'  AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS image_files,
			COALESCE(MAX(embed_model), 'none')                                 AS embed_model
		FROM files
	`).Scan(
		&s.TotalFiles, &s.DeletedFiles, &s.Duplicates,
		&s.TextFiles, &s.ImageFiles, &s.EmbedModel,
	)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	return &s, nil
}
