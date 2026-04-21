package store

import (
	"context"
	"database/sql"
)

// FileStatus reports what processing artifacts already exist for a file.
// Used by extractors to determine which stages to skip (Option A:
// infer completion from existing items/chunks rows, no status columns).
type FileStatus struct {
	FileID      int64
	ContentHash string
	HasItems    map[string]bool // item_type → exists (e.g. "image", "ocr", "transcript")
	HasChunks   map[string]bool // "item_type:chunk_type" → exists (e.g. "image:embed", "image:caption")
}

// GetFileStatus returns what items and chunks exist for a given file path.
// Returns nil if the file doesn't exist in the DB.
//
// The query joins files → items → chunks and collects which
// (item_type, chunk_type) combinations already have rows.
// Extractors use this to skip stages that are already complete.
func GetFileStatus(ctx context.Context, path string) (*FileStatus, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.id, f.content_hash, i.item_type, c.chunk_type
		FROM files f
		LEFT JOIN items i ON i.file_id = f.id
		LEFT JOIN chunks c ON c.item_id = i.id
		WHERE f.path = $1 AND f.deleted_at IS NULL`,
		path,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fs *FileStatus
	for rows.Next() {
		var (
			fileID      int64
			contentHash string
			itemType    sql.NullString
			chunkType   sql.NullString
		)
		if err := rows.Scan(&fileID, &contentHash, &itemType, &chunkType); err != nil {
			return nil, err
		}

		if fs == nil {
			fs = &FileStatus{
				FileID:      fileID,
				ContentHash: contentHash,
				HasItems:    make(map[string]bool),
				HasChunks:   make(map[string]bool),
			}
		}

		if itemType.Valid {
			fs.HasItems[itemType.String] = true
		}
		if itemType.Valid && chunkType.Valid {
			fs.HasChunks[itemType.String+":"+chunkType.String] = true
		}
	}

	return fs, rows.Err()
}

// GetFileStatusBatch returns FileStatus for multiple paths in one query.
// More efficient than calling GetFileStatus in a loop during bulk extraction.
func GetFileStatusBatch(ctx context.Context, paths []string) (map[string]*FileStatus, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.path, f.id, f.content_hash, i.item_type, c.chunk_type
		FROM files f
		LEFT JOIN items i ON i.file_id = f.id
		LEFT JOIN chunks c ON c.item_id = i.id
		WHERE f.path = ANY($1) AND f.deleted_at IS NULL`,
		paths,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*FileStatus)
	for rows.Next() {
		var (
			path        string
			fileID      int64
			contentHash string
			itemType    sql.NullString
			chunkType   sql.NullString
		)
		if err := rows.Scan(&path, &fileID, &contentHash, &itemType, &chunkType); err != nil {
			return nil, err
		}

		fs, ok := result[path]
		if !ok {
			fs = &FileStatus{
				FileID:      fileID,
				ContentHash: contentHash,
				HasItems:    make(map[string]bool),
				HasChunks:   make(map[string]bool),
			}
			result[path] = fs
		}

		if itemType.Valid {
			fs.HasItems[itemType.String] = true
		}
		if itemType.Valid && chunkType.Valid {
			fs.HasChunks[itemType.String+":"+chunkType.String] = true
		}
	}

	return result, rows.Err()
}
