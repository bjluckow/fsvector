package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type UpsertFile struct {
	Path           string
	Source         string
	CanonicalPath  *string
	ContentHash    string
	Size           int64
	MimeType       string
	Modality       string
	FileName       string
	FileExt        string
	FileCreatedAt  *time.Time
	FileModifiedAt *time.Time
	EmbedModel     string
	Embedding      []float32
	ChunkIndex     int
	ChunkType      *string
	Metadata       map[string]any
	TextContent    *string
}

func itemType(chunkType *string, modality string) string {
	if chunkType != nil {
		switch *chunkType {
		case "frame":
			return "frames"
		case "transcript":
			return "transcript"
		}
	}
	switch modality {
	case "audio":
		return "transcript"
	default:
		return "whole"
	}
}

func itemIndex(chunkType *string, modality string) int {
	if chunkType != nil && *chunkType == "transcript" && modality == "video" {
		return 1
	}
	return 0
}

// Upsert inserts or updates a file, its item, and its chunk atomically.
// If CanonicalPath is set, only the files row is written (duplicate tracking).
func Upsert(ctx context.Context, f UpsertFile) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := upsertTx(ctx, tx, f); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func upsertTx(ctx context.Context, tx pgx.Tx, f UpsertFile) error {
	// 1. upsert files row
	var fileID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO files (
			path, source, canonical_path, modality,
			file_name, file_ext, mime_type, size, content_hash,
			file_created_at, file_modified_at, indexed_at, deleted_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8, $9,
			$10, $11, now(), NULL
		)
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
			indexed_at       = now(),
			deleted_at       = NULL
		RETURNING id
	`,
		f.Path, f.Source, f.CanonicalPath, f.Modality,
		f.FileName, f.FileExt, f.MimeType, f.Size, f.ContentHash,
		f.FileCreatedAt, f.FileModifiedAt,
	).Scan(&fileID)
	if err != nil {
		return fmt.Errorf("upsert file %s: %w", f.Path, err)
	}

	// duplicate — only files row needed, no items or chunks
	if f.CanonicalPath != nil && *f.CanonicalPath != "" {
		return nil
	}

	// 2. upsert items row
	it := itemType(f.ChunkType, f.Modality)
	ii := itemIndex(f.ChunkType, f.Modality)

	var itemID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO items (file_id, item_type, item_index)
		VALUES ($1, $2, $3)
		ON CONFLICT (file_id, item_type, item_index) DO UPDATE SET
			item_type = EXCLUDED.item_type
		RETURNING id
	`, fileID, it, ii).Scan(&itemID)
	if err != nil {
		return fmt.Errorf("upsert item %s: %w", f.Path, err)
	}

	// 3. upsert chunks row
	var embedding *pgvector.Vector
	if f.Embedding != nil {
		v := pgvector.NewVector(f.Embedding)
		embedding = &v
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO chunks (
			item_id, chunk_index, chunk_type, embed_model,
			embedding, text_content, metadata, indexed_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, now()
		)
		ON CONFLICT (item_id, chunk_index) DO UPDATE SET
			chunk_type   = EXCLUDED.chunk_type,
			embed_model  = EXCLUDED.embed_model,
			embedding    = EXCLUDED.embedding,
			text_content = EXCLUDED.text_content,
			metadata     = EXCLUDED.metadata,
			indexed_at   = now()
	`,
		itemID, f.ChunkIndex, f.ChunkType, f.EmbedModel,
		embedding, f.TextContent, f.Metadata,
	)
	if err != nil {
		return fmt.Errorf("upsert chunk %s[%d]: %w", f.Path, f.ChunkIndex, err)
	}

	return nil
}
