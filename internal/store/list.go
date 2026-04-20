package store

import (
	"context"
	"fmt"
	"time"

	"github.com/bjluckow/fsvector/pkg/api"
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

// ListQuery holds all list/export parameters.
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
// Returns one row per file (not per chunk).
func List(ctx context.Context, q ListQuery) ([]ListFile, error) {
	sql := `
		SELECT DISTINCT ON (path)
			path,
			modality,
			file_ext,
			size,
			indexed_at,
			file_modified_at,
			deleted_at,
			canonical_path IS NOT NULL AS is_duplicate
		FROM file_chunks
		WHERE 1=1
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

	rows, err := pool.Query(ctx, sql, args...)
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

// buildExportSQL builds the shared SQL and args for Export and ExportStream.
func buildExportSQL(q ListQuery) (string, []any) {
	sql := `
		SELECT
			path, source, canonical_path,
			content_hash, size, mime_type, modality,
			file_name, file_ext, embed_model, embedding,
			chunk_index, chunk_type, text_content,
			chunk_metadata, indexed_at, file_modified_at,
			file_created_at, deleted_at
		FROM file_chunks
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
	return sql, args
}

func scanExportRow(rows interface {
	Scan(dest ...any) error
}) (api.ExportRow, error) {
	var r api.ExportRow
	var embedding pgvector.Vector
	if err := rows.Scan(
		&r.Path, &r.Source, &r.CanonicalPath,
		&r.ContentHash, &r.Size, &r.MimeType, &r.Modality,
		&r.FileName, &r.Ext, &r.EmbedModel, &embedding,
		&r.ChunkIndex, &r.ChunkType, &r.TextContent,
		&r.Metadata, &r.IndexedAt, &r.ModifiedAt,
		&r.CreatedAt, &r.DeletedAt,
	); err != nil {
		return api.ExportRow{}, err
	}
	r.Embedding = embedding.Slice()
	return r, nil
}

// Export returns full file rows including embeddings.
// WARNING: Can be extremely memory intensive — use ExportStream instead.
func Export(ctx context.Context, q ListQuery) ([]api.ExportRow, error) {
	sql, args := buildExportSQL(q)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	defer rows.Close()

	var result []api.ExportRow
	for rows.Next() {
		r, err := scanExportRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ExportStream calls fn for each matching row as it comes from postgres.
// Never holds more than one row in memory at a time.
func ExportStream(ctx context.Context, q ListQuery, fn func(api.ExportRow) error) error {
	sql, args := buildExportSQL(q)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		r, err := scanExportRow(rows)
		if err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return rows.Err()
}
