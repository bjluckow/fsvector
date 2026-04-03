package search

import (
	"context"
	"fmt"
	"time"

	"github.com/bjluckow/fsvector/internal/store"
	"github.com/pgvector/pgvector-go"
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

type ExportRow struct {
	Path        string         `json:"path"`
	Source      string         `json:"source"`
	Modality    string         `json:"modality"`
	Ext         string         `json:"ext"`
	MimeType    string         `json:"mime_type"`
	EmbedModel  string         `json:"embed_model"`
	Embedding   []float32      `json:"embedding"`
	ChunkIndex  int            `json:"chunk_index"`
	ChunkType   *string        `json:"chunk_type,omitempty"`
	TextContent *string        `json:"text_content,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	IndexedAt   time.Time      `json:"indexed_at"`
	ModifiedAt  *time.Time     `json:"modified_at,omitempty"`
}

// Export returns full file rows including embeddings, using the same
// filters as List. Used for cross-instance sync and plugin data access.
// WARNING: Can be extremely memory intensive without streaming, use ExportStream instead
func Export(ctx context.Context, db store.Querier, q ListQuery) ([]ExportRow, error) {
	sql := `
		SELECT
			path, source, modality, file_ext, mime_type,
			embed_model, embedding, chunk_index, chunk_type,
			text_content, metadata, indexed_at, file_modified_at
		FROM files
		WHERE (canonical_path IS NULL OR canonical_path = '')
	`

	args := []any{}
	idx := 1

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

	sql += " ORDER BY path, chunk_index"

	// no LIMIT for export — stream everything
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	defer rows.Close()

	var result []ExportRow
	for rows.Next() {
		var r ExportRow
		var embedding pgvector.Vector
		if err := rows.Scan(
			&r.Path, &r.Source, &r.Modality, &r.Ext, &r.MimeType,
			&r.EmbedModel, &embedding, &r.ChunkIndex, &r.ChunkType,
			&r.TextContent, &r.Metadata, &r.IndexedAt, &r.ModifiedAt,
		); err != nil {
			return nil, err
		}
		r.Embedding = embedding.Slice()
		result = append(result, r)
	}
	return result, rows.Err()
}

// ExportStream calls fn for each matching row as it comes from postgres.
// Never holds more than one row in memory at a time.
func ExportStream(ctx context.Context, db store.Querier, q ListQuery, fn func(ExportRow) error) error {
	sql := `
		SELECT
			path, source, modality, file_ext, mime_type,
			embed_model, embedding, chunk_index, chunk_type,
			text_content, metadata, indexed_at, file_modified_at
		FROM files
		WHERE (canonical_path IS NULL OR canonical_path = '')
	`

	args := []any{}
	idx := 1

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

	sql += " ORDER BY path, chunk_index"

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var r ExportRow
		var embedding pgvector.Vector
		if err := rows.Scan(
			&r.Path, &r.Source, &r.Modality, &r.Ext, &r.MimeType,
			&r.EmbedModel, &embedding, &r.ChunkIndex, &r.ChunkType,
			&r.TextContent, &r.Metadata, &r.IndexedAt, &r.ModifiedAt,
		); err != nil {
			return err
		}
		r.Embedding = embedding.Slice()
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}
