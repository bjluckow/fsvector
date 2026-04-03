package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

func processText(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(ctx, cfg, fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	var text string
	plainExts := map[string]bool{
		"txt": true, "md": true, "go": true, "py": true,
		"js": true, "ts": true, "css": true, "json": true,
		"yaml": true, "yml": true, "toml": true, "sh": true,
		"rs": true, "c": true, "cpp": true, "h": true,
		"java": true, "rb": true, "html": true, "htm": true,
	}

	if !plainExts[fi.Ext] {
		converted, err := cfg.ConvertClient.ConvertToText(ctx, fi.Name, data)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
		text = string(converted)
	} else {
		text = string(data)
	}

	chunks := chunk.Split(text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	if len(chunks) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no embeddable content after chunking",
		}, nil
	}

	var files []store.File
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, i)
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
func processTextChunk(ctx context.Context, cfg Config, fi fsindex.FileInfo, text string, chunkIndex int) (*store.File, error) {
	vectors, err := cfg.EmbedClient.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}

	return &store.File{
		Path:           fi.Path,
		Source:         cfg.Source,
		ContentHash:    fi.Hash,
		Size:           fi.Size,
		MimeType:       fi.MimeType,
		Modality:       "text",
		FileName:       fi.Name,
		FileExt:        fi.Ext,
		FileCreatedAt:  &fi.CreatedAt,
		FileModifiedAt: &fi.ModifiedAt,
		EmbedModel:     cfg.EmbedModel,
		Embedding:      vectors[0],
		ChunkIndex:     chunkIndex,
		TextContent:    &text,
	}, nil
}
