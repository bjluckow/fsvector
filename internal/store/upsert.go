package store

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/model"
)

// // FileRow represents a file to upsert into the files table.
// type FileRow struct {
// 	Path          string
// 	Source        string
// 	CanonicalPath *string
// 	Modality      string
// 	FileName      string
// 	FileExt       string
// 	MimeType      string
// 	Size          int64
// 	ContentHash   string
// 	CreatedAt     time.Time
// 	ModifiedAt    time.Time
// 	Metadata      json.RawMessage // nil = SQL NULL
// }

// // ItemRow represents an item to upsert into the items table.
// type ItemRow struct {
// 	ItemType    string
// 	ItemName    string
// 	MimeType    string
// 	Size        int64
// 	ContentHash string
// 	ItemIndex   int
// 	Metadata    json.RawMessage
// }

// // ChunkRow represents a chunk to upsert into the chunks table.
// type ChunkRow struct {
// 	ItemID      int64
// 	ChunkIndex  int
// 	ChunkType   string
// 	EmbedModel  string
// 	Embedding   *pgvector.Vector
// 	TextContent *string // nil = SQL NULL
// 	Metadata    json.RawMessage
// }

// UpsertFile creates or updates a file row. Returns the file ID.
// Called during phase 1 (extraction) before work items are dispatched.
// ON CONFLICT updates the row and clears deleted_at (un-deletes if needed).
func UpsertFile(ctx context.Context, f model.File) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO files (
			path, source, canonical_path, modality, file_name, file_ext,
			mime_type, size, content_hash, file_created_at, file_modified_at,
			metadata, indexed_at, deleted_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, now(), NULL)
		ON CONFLICT (path) DO UPDATE SET
			source           = EXCLUDED.source,
			canonical_path   = EXCLUDED.canonical_path,
			modality         = EXCLUDED.modality,
			file_name        = EXCLUDED.file_name,
			file_ext         = EXCLUDED.file_ext,
			mime_type        = EXCLUDED.mime_type,
			size             = EXCLUDED.size,
			content_hash     = EXCLUDED.content_hash,
			file_created_at  = EXCLUDED.file_created_at,
			file_modified_at = EXCLUDED.file_modified_at,
			metadata         = EXCLUDED.metadata,
			indexed_at       = now(),
			deleted_at       = NULL
		RETURNING id`,
		f.Path, f.Source, f.CanonicalPath, f.Modality,
		f.Name, f.Ext, f.MimeType, f.Size,
		f.ContentHash, f.CreatedAt, f.ModifiedAt, f.Metadata,
	).Scan(&id)
	return id, err
}

// UpsertItem creates or updates an item row. Returns the item ID.
// Called during phase 1 (extraction) to register each item (image, frame,
// audio track, text body, etc.) before work items are dispatched.
// Idempotent — multiple calls with the same (file_id, item_type, item_index) are safe.
func UpsertItem(ctx context.Context, fileID int64, item model.Item) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx, `
		INSERT INTO items (
			file_id, item_type, item_name, mime_type,
			size, content_hash, item_index, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (file_id, item_type, item_index) DO UPDATE SET
			item_name    = EXCLUDED.item_name,
			mime_type    = EXCLUDED.mime_type,
			size         = EXCLUDED.size,
			content_hash = EXCLUDED.content_hash,
			metadata     = EXCLUDED.metadata
		RETURNING id`,
		fileID, item.ItemType, item.ItemName, item.MimeType,
		item.Size, item.ContentHash, item.ItemIndex, item.Metadata,
	).Scan(&id)
	return id, err
}

// UpsertChunkBatch upserts multiple chunks in a single transaction.
// Called by phase 2 workers when flushing a batch of results.
// Each chunk is written against an item ID that was established in phase 1.
func UpsertChunkBatch(ctx context.Context, chunks []model.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const query = `
		INSERT INTO chunks (
			item_id, chunk_index, chunk_type, embed_model,
			embedding, text_content, metadata, indexed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7, now())
		ON CONFLICT (item_id, chunk_index) DO UPDATE SET
			chunk_type   = EXCLUDED.chunk_type,
			embed_model  = EXCLUDED.embed_model,
			embedding    = EXCLUDED.embedding,
			text_content = EXCLUDED.text_content,
			metadata     = EXCLUDED.metadata,
			indexed_at   = now()`

	for _, c := range chunks {
		if _, err := tx.Exec(ctx, query,
			c.ItemID, c.ChunkIndex, c.ChunkType, c.EmbedModel,
			c.Embedding, c.TextContent, c.Metadata,
		); err != nil {
			return fmt.Errorf("chunk item=%d idx=%d: %w", c.ItemID, c.ChunkIndex, err)
		}
	}

	return tx.Commit(ctx)
}

// UpsertChunk upserts a single chunk. Convenience wrapper for cases where
// batching isn't needed (e.g., single-file watch events).
func UpsertChunk(ctx context.Context, chunk model.Chunk) error {
	return UpsertChunkBatch(ctx, []model.Chunk{chunk})
}
