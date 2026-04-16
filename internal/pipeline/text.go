package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
)

func (pl Pipeline) processText(ctx context.Context, fi source.FileInfo, data []byte) (Result, error) {
	var text string
	plainExts := map[string]bool{
		"txt": true, "md": true, "go": true, "py": true,
		"js": true, "ts": true, "css": true, "json": true,
		"yaml": true, "yml": true, "toml": true, "sh": true,
		"rs": true, "c": true, "cpp": true, "h": true,
		"java": true, "rb": true, "html": true, "htm": true,
	}

	if !plainExts[fi.Ext] {
		converted, err := pl.ConvertClient.ConvertToText(ctx, fi.Name, data)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
		text = string(converted)
	} else {
		text = string(data)
	}

	chunks := chunk.Split(text, pl.ChunkSize, pl.ChunkOverlap, pl.MinChunkSize)
	if len(chunks) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no embeddable content after chunking",
		}, nil
	}

	var files []store.UpsertFile
	for i, c := range chunks {
		f, err := pl.processTextChunk(ctx, fi, c, i)
		if err != nil {
			return Result{}, fmt.Errorf("chunk %d of %s: %w", i, fi.Path, err)
		}
		if f != nil {
			files = append(files, *f)
		}
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "all chunks failed to embed",
		}, nil
	}

	return Result{Files: files}, nil
}

// processTextChunk embeds a single text chunk and returns a store.File.
// Returns nil if the embed service returns no vectors.
func (pl Pipeline) processTextChunk(ctx context.Context, fi source.FileInfo, text string, chunkIndex int) (*store.UpsertFile, error) {
	vectors, err := pl.EmbedClient.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}

	return &store.UpsertFile{
		Path:           fi.Path,
		Source:         pl.Source,
		ContentHash:    fi.Hash,
		Size:           fi.Size,
		MimeType:       fi.MimeType,
		Modality:       "text",
		FileName:       fi.Name,
		FileExt:        fi.Ext,
		FileCreatedAt:  &fi.CreatedAt,
		FileModifiedAt: &fi.ModifiedAt,
		EmbedModel:     pl.EmbedModel,
		Embedding:      vectors[0],
		ChunkIndex:     chunkIndex,
		TextContent:    &text,
	}, nil
}
