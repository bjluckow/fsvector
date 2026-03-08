package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// SoftDelete marks all chunks for a given path as deleted.
func SoftDelete(ctx context.Context, conn *pgx.Conn, path string) error {
	_, err := conn.Exec(ctx, `
		UPDATE files
		SET deleted_at = $1
		WHERE path = $2
		  AND deleted_at IS NULL
	`, time.Now(), path)
	if err != nil {
		return fmt.Errorf("soft delete %s: %w", path, err)
	}
	return nil
}

// UnDelete clears the deleted_at flag for all chunks of a given path.
func UnDelete(ctx context.Context, conn *pgx.Conn, path string) error {
	_, err := conn.Exec(ctx, `
		UPDATE files
		SET deleted_at = NULL
		WHERE path = $1
	`, path)
	if err != nil {
		return fmt.Errorf("undelete %s: %w", path, err)
	}
	return nil
}

// LivePaths returns the paths of all non-deleted, canonical files.
// Used during startup reconciliation to diff against the filesystem.
func LivePaths(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT path, content_hash
		FROM files
		WHERE deleted_at IS NULL
		  AND canonical_path IS NULL
		  AND chunk_index = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("live paths: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		result[path] = hash
	}
	return result, rows.Err()
}
