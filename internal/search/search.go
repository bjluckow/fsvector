package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// SearchResult is a single result from a similarity search.
type SearchResult struct {
	Path       string
	Modality   string
	FileExt    string
	Size       int64
	Score      float64
	NormScore  float64 // populated by Normalize()
	IndexedAt  time.Time
	ModifiedAt *time.Time
}

// SearchQuery holds all search parameters.
// Only Vector and Limit are required — all other fields are optional filters.
type SearchQuery struct {
	Vector []float32
	Limit  int
	Offset int

	// filters
	Modality string
	Ext      string
	Source   string
	Since    *time.Time
	Before   *time.Time
	MinSize  *int64
	MaxSize  *int64
	MinScore *float64
}

// Search performs a cosine similarity search against live, canonical files.
func Search(ctx context.Context, conn *pgx.Conn, q SearchQuery) ([]SearchResult, error) {
	v := pgvector.NewVector(q.Vector)

	// select the right embedding column based on modality filter
	// if no modality filter, search text by default
	embeddingCol := "text_embedding"
	if q.Modality == "image" {
		embeddingCol = "image_embedding"
	}

	sql := fmt.Sprintf(`
        SELECT
            path,
            modality,
            file_ext,
            size,
            1 - (%s <=> $1) AS score,
            indexed_at,
            file_modified_at
        FROM files
        WHERE deleted_at IS NULL
          AND retired_at IS NULL
          AND canonical_path IS NULL
          AND %s IS NOT NULL
    `, embeddingCol, embeddingCol)

	args := []any{v, q.Limit, q.Offset}
	idx := 4 // next parameter index

	if q.Modality != "" {
		sql += fmt.Sprintf(" AND modality = $%d", idx)
		args = append(args, q.Modality)
		idx++
	}
	if q.Ext != "" {
		sql += fmt.Sprintf(" AND file_ext = $%d", idx)
		args = append(args, q.Ext)
		idx++
	}
	if q.Source != "" {
		sql += fmt.Sprintf(" AND source = $%d", idx)
		args = append(args, q.Source)
		idx++
	}
	if q.Since != nil {
		sql += fmt.Sprintf(" AND file_modified_at >= $%d", idx)
		args = append(args, q.Since)
		idx++
	}
	if q.Before != nil {
		sql += fmt.Sprintf(" AND file_modified_at <= $%d", idx)
		args = append(args, q.Before)
		idx++
	}
	if q.MinSize != nil {
		sql += fmt.Sprintf(" AND size >= $%d", idx)
		args = append(args, q.MinSize)
		idx++
	}
	if q.MaxSize != nil {
		sql += fmt.Sprintf(" AND size <= $%d", idx)
		args = append(args, q.MaxSize)
		idx++
	}
	if q.MinScore != nil {
		sql += fmt.Sprintf(" AND 1 - (embedding <=> $1) >= $%d", idx)
		args = append(args, q.MinScore)
		idx++
	}

	sql += " ORDER BY embedding <=> $1 LIMIT $2 OFFSET $3"

	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.Path, &r.Modality, &r.FileExt,
			&r.Size, &r.Score, &r.IndexedAt, &r.ModifiedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
