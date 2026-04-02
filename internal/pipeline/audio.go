package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

func processAudio(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	resp, err := cfg.TranscribeClient.Transcribe(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("transcribe %s: %w", fi.Path, err)
	}
	if strings.TrimSpace(resp.Text) == "" {
		return Result{
			Skipped:    true,
			SkipReason: "empty transcript",
		}, nil
	}

	chunks := chunk.Split(resp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	if len(chunks) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "transcript too short to chunk",
		}, nil
	}

	chunkType := "transcript"
	metadata := map[string]any{
		"duration_seconds": resp.DurationSeconds,
		"language":         resp.Language,
	}

	var files []store.File
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, i)
		if err != nil {
			return Result{}, fmt.Errorf("chunk %d of %s: %w", i, fi.Path, err)
		}
		if f != nil {
			f.Modality = "audio"
			f.ChunkType = &chunkType
			f.Metadata = metadata
			files = append(files, *f)
		}
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "all transcript chunks failed to embed",
		}, nil
	}

	return Result{Files: files}, nil
}
