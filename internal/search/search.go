package search

import (
	"context"
	"fmt"
	"time"

	"github.com/bjluckow/fsvector/internal/store"
)

// SearchQuery holds all search parameters.
type SearchQuery struct {
	Query  string
	Vector []float32
	Mode   SearchMode
	Config SearchConfig

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

// Search performs a search against live canonical files.
func Search(ctx context.Context, db store.Querier, q SearchQuery) ([]SearchResult, error) {
	// apply defaults
	if q.Mode == "" {
		q.Mode = q.Config.DefaultMode
	}
	if q.Mode == "" {
		q.Mode = SearchModeHybrid
	}
	if q.Limit == 0 {
		q.Limit = 10
	}

	switch q.Mode {
	case SearchModeVector:
		return searchVector(ctx, db, q)
	case SearchModeFullText:
		return searchFullText(ctx, db, q)
	default:
		return searchHybrid(ctx, db, q)
	}
}

// applyFilters appends WHERE clauses for optional filters.
// Returns updated where string, args slice, and next param index.
func applyFilters(where string, args []any, idx int, q SearchQuery) (string, []any, int) {
	if q.Modality != "" {
		where += fmt.Sprintf(" AND modality = $%d", idx)
		args = append(args, q.Modality)
		idx++
	}
	if q.Ext != "" {
		where += fmt.Sprintf(" AND file_ext = $%d", idx)
		args = append(args, q.Ext)
		idx++
	}
	if q.Source != "" {
		where += fmt.Sprintf(" AND source = $%d", idx)
		args = append(args, q.Source)
		idx++
	}
	if q.Since != nil {
		where += fmt.Sprintf(" AND file_modified_at >= $%d", idx)
		args = append(args, q.Since)
		idx++
	}
	if q.Before != nil {
		where += fmt.Sprintf(" AND file_modified_at <= $%d", idx)
		args = append(args, q.Before)
		idx++
	}
	if q.MinSize != nil {
		where += fmt.Sprintf(" AND size >= $%d", idx)
		args = append(args, q.MinSize)
		idx++
	}
	if q.MaxSize != nil {
		where += fmt.Sprintf(" AND size <= $%d", idx)
		args = append(args, q.MaxSize)
		idx++
	}
	if q.MinScore != nil {
		where += fmt.Sprintf(" AND 1 - (embedding <=> $1) >= $%d", idx)
		args = append(args, q.MinScore)
		idx++
	}
	return where, args, idx
}

// runSearch wraps an inner DISTINCT ON query with pagination
// and scans results.
func runSearch(ctx context.Context, db store.Querier, inner string, args []any) ([]SearchResult, error) {
	sql := fmt.Sprintf(`
		SELECT path, modality, file_ext, size, score, indexed_at, file_modified_at
		FROM (%s) deduped
		ORDER BY score DESC
		LIMIT $2 OFFSET $3
	`, inner)

	rows, err := db.Query(ctx, sql, args...)
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
