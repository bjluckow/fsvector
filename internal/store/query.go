package store

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/jackc/pgx/v5"
)

// GetEmbeddingDim returns the current vector dimension of the chunks table,
// or 0 if the table does not exist yet.
func GetEmbeddingDim(ctx context.Context) (int, error) {
	var dim int
	err := pool.QueryRow(ctx, `
		SELECT atttypmod
		FROM pg_attribute
		WHERE attrelid = 'chunks'::regclass
		  AND attname = 'embedding'
	`).Scan(&dim)

	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, nil
	}
	return dim, nil
}

// LivePaths returns path → content_hash for all live canonical files.
// Used during reconciliation to diff against the source.
func LivePaths(ctx context.Context) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT path, content_hash
		FROM files
		WHERE deleted_at IS NULL
		AND canonical_path IS NULL
		AND path NOT LIKE '%' || $1 || '%'
	`, api.AttachmentSep)
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

// FindByHash returns the canonical path for a given content hash.
// Used for deduplication during indexing.
func FindByHash(ctx context.Context, hash string) (string, bool, error) {
	var path string
	err := pool.QueryRow(ctx, `
		SELECT path FROM files
		WHERE content_hash = $1
		  AND canonical_path IS NULL
		  AND deleted_at IS NULL
		LIMIT 1
	`, hash).Scan(&path)

	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find by hash: %w", err)
	}
	return path, true, nil
}
