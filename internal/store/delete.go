package store

import (
	"context"
	"fmt"
	"time"
)

// SoftDelete marks a file as deleted by setting deleted_at.
// Cascades logically — items and chunks are excluded from search
// via the files.deleted_at filter on all queries.
func SoftDelete(ctx context.Context, path string) error {
	_, err := pool.Exec(ctx, `
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

// UnDelete clears the deleted_at flag for a given path.
func UnDelete(ctx context.Context, path string) error {
	_, err := pool.Exec(ctx, `
		UPDATE files
		SET deleted_at = NULL
		WHERE path = $1
	`, path)
	if err != nil {
		return fmt.Errorf("undelete %s: %w", path, err)
	}
	return nil
}

// DeleteStaleChunks hard-deletes chunks with index >= newChunkCount
// for a given path. Called after re-indexing to remove chunks that
// no longer exist. Hard delete is safe here — stale chunks are index
// artifacts, not filesystem deletions.
func DeleteStaleChunks(ctx context.Context, path, embedModel string, newChunkCount int) error {
	// find the item IDs for this path
	_, err := pool.Exec(ctx, `
		DELETE FROM chunks
		WHERE item_id IN (
			SELECT i.id FROM items i
			JOIN files f ON f.id = i.file_id
			WHERE f.path = $1
		)
		AND embed_model = $2
		AND chunk_index >= $3
	`, path, embedModel, newChunkCount)
	if err != nil {
		return fmt.Errorf("delete stale chunks %s: %w", path, err)
	}
	return nil
}

// PurgeSoftDeleted hard-deletes all files marked as deleted.
// Cascades to items and chunks via ON DELETE CASCADE.
func PurgeSoftDeleted(ctx context.Context) (int64, error) {
	tag, err := pool.Exec(ctx, `
		DELETE FROM files
		WHERE deleted_at IS NOT NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("purge soft deleted: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteStaleItems removes items (and their cascaded chunks) for a file
// that no longer exist after re-extraction. For example, if a video
// previously had 10 frames but now has 8, items 8 and 9 should be removed.
func DeleteStaleItems(ctx context.Context, fileID int64, itemType string, keepCount int) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM items
		WHERE file_id = $1 AND item_type = $2 AND item_index >= $3`,
		fileID, itemType, keepCount,
	)
	return err
}

// DeleteStaleChunks removes chunks for an item that are beyond the current
// chunk count. Used when re-processing produces fewer chunks than before.
func DeleteStaleChunksByItem(ctx context.Context, itemID int64, keepCount int) error {
	_, err := pool.Exec(ctx, `
		DELETE FROM chunks
		WHERE item_id = $1 AND chunk_index >= $2`,
		itemID, keepCount,
	)
	return err
}
