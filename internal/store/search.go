package store

import (
	"context"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"
)

// SearchResult is a single result from a similarity
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
	Query  string
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
func Search(ctx context.Context, q SearchQuery) ([]SearchResult, error) {
	v := pgvector.NewVector(q.Vector)
	embeddingCol := "embedding"

	args := []any{v, q.Limit, q.Offset}
	idx := 4

	var innerWhere string

	if q.Query != "" {
		args = append(args, q.Query) // $4
		idx = 5
		innerWhere = `
        SELECT DISTINCT ON (path)
            path, modality, file_ext, size,
            0.5 * (1 - (embedding <=> $1)) +
			0.5 * CASE
				WHEN COALESCE((
					SELECT MAX(ts_rank(
						to_tsvector('english', COALESCE(c.text_content, '')),
						plainto_tsquery('english', $4)
					))
					FROM files c
					WHERE c.path = files.path
					AND c.deleted_at IS NULL
				), 0) = 0 THEN 0.0
				ELSE GREATEST(
					0.3,  -- minimum bonus for any match
					COALESCE((
						SELECT MAX(ts_rank(
							to_tsvector('english', COALESCE(c.text_content, '')),
							plainto_tsquery('english', $4)
						))
						FROM files c
						WHERE c.path = files.path
						AND c.deleted_at IS NULL
					), 0) * 10  -- scale up ts_rank values
				)
			END AS score,
            indexed_at, file_modified_at
        FROM files
        WHERE deleted_at IS NULL
          AND (canonical_path IS NULL OR canonical_path = '')
          AND embedding IS NOT NULL`
	} else {
		innerWhere = `
        SELECT DISTINCT ON (path)
            path, modality, file_ext, size,
            1 - (embedding <=> $1) AS score,
            indexed_at, file_modified_at
        FROM files
        WHERE deleted_at IS NULL
          AND (canonical_path IS NULL OR canonical_path = '')
          AND embedding IS NOT NULL`
	}

	innerOrder := " ORDER BY path, embedding <=> $1"

	if q.Modality != "" {
		innerWhere += fmt.Sprintf(" AND modality = $%d", idx)
		args = append(args, q.Modality)
		idx++
	}
	if q.Ext != "" {
		innerWhere += fmt.Sprintf(" AND file_ext = $%d", idx)
		args = append(args, q.Ext)
		idx++
	}
	if q.Source != "" {
		innerWhere += fmt.Sprintf(" AND source = $%d", idx)
		args = append(args, q.Source)
		idx++
	}
	if q.Since != nil {
		innerWhere += fmt.Sprintf(" AND file_modified_at >= $%d", idx)
		args = append(args, q.Since)
		idx++
	}
	if q.Before != nil {
		innerWhere += fmt.Sprintf(" AND file_modified_at <= $%d", idx)
		args = append(args, q.Before)
		idx++
	}
	if q.MinSize != nil {
		innerWhere += fmt.Sprintf(" AND size >= $%d", idx)
		args = append(args, q.MinSize)
		idx++
	}
	if q.MaxSize != nil {
		innerWhere += fmt.Sprintf(" AND size <= $%d", idx)
		args = append(args, q.MaxSize)
		idx++
	}
	if q.MinScore != nil {
		innerWhere += fmt.Sprintf(" AND 1 - (%s <=> $1) >= $%d", embeddingCol, idx)
		args = append(args, q.MinScore)
		idx++
	}

	// DISTINCT ON requires ORDER BY to start with the distinct expression
	// then the similarity — this picks the best chunk per path
	inner := innerWhere + innerOrder

	// wrap in outer query to apply limit/offset after deduplication
	// and re-sort by score descending
	sql := fmt.Sprintf(`
		SELECT path, modality, file_ext, size, score, indexed_at, file_modified_at
		FROM (%s) deduped
		ORDER BY score DESC
		LIMIT $2 OFFSET $3
	`, inner)

	rows, err := pool.Query(ctx, sql, args...)
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
