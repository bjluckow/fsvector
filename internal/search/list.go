package search

import (
	"context"
	"fmt"
	"time"

	"github.com/bjluckow/fsvector/internal/store"
)

// ListFile is a single row returned by List.
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

// ListQuery holds all list parameters.
type ListQuery struct {
	Limit          int
	Offset         int
	IncludeDeleted bool

	// filters
	Modality string
	Ext      string
	Source   string
	Since    *time.Time
	Before   *time.Time
}

// List returns indexed files matching the query.
func List(ctx context.Context, db store.Querier, q ListQuery) ([]ListFile, error) {
	sql := `
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

	args := []any{q.Limit, q.Offset}
	idx := 3

	if !q.IncludeDeleted {
		sql += " AND deleted_at IS NULL"
	}
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

	sql += " ORDER BY path LIMIT $1 OFFSET $2"

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
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
