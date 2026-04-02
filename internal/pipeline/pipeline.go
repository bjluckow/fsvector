package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/convert"
	"github.com/bjluckow/fsvector/internal/embed"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/internal/transcribe"
)

// Config holds the dependencies for the pipeline.
type Config struct {
	EmbedClient      *embed.Client
	ConvertClient    *convert.Client
	TranscribeClient *transcribe.Client
	EmbedModel       string
	Source           string
	MinEmbedSize     int64
	ChunkSize        int
	ChunkOverlap     int
	MinChunkSize     int
}

// Result is returned after a file has been processed.
type Result struct {
	Files      []store.File
	Skipped    bool
	SkipReason string
}

// Process runs a single FileInfo through the full pipeline:
// detect modality → convert → embed → return store.File ready for upsert.
func Process(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	if fi.Size < cfg.MinEmbedSize {
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("file too small (< %d bytes)", cfg.MinEmbedSize),
		}, nil
	}

	modality, supported := Modality(fi.Ext)
	if !supported {
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("unsupported extension: %s", fi.Ext),
		}, nil
	}

	switch modality {
	case "text":
		return processText(ctx, cfg, fi)
	case "image":
		return processImage(ctx, cfg, fi)
	case "audio":
		return processAudio(ctx, cfg, fi)
	default:
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("unhandled modality: %s", modality),
		}, nil
	}
}

func processText(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	var text string
	if target := ConvertTarget(fi.Ext); target != "" {
		converted, err := cfg.ConvertClient.Convert(ctx, fi.Name, data, target)
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

func processImage(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	if target := ConvertTarget(fi.Ext); target != "" {
		data, err = cfg.ConvertClient.Convert(ctx, fi.Name, data, target)
		if err != nil {
			return Result{}, fmt.Errorf("convert image %s: %w", fi.Path, err)
		}
	}

	// embed as image
	vector, err := cfg.EmbedClient.EmbedImage(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("embed image %s: %w", fi.Path, err)
	}

	return Result{
		Files: []store.File{
			{
				Path:           fi.Path,
				Source:         cfg.Source,
				ContentHash:    fi.Hash,
				Size:           fi.Size,
				MimeType:       fi.MimeType,
				Modality:       "image",
				FileName:       fi.Name,
				FileExt:        fi.Ext,
				FileCreatedAt:  &fi.CreatedAt,
				FileModifiedAt: &fi.ModifiedAt,
				EmbedModel:     cfg.EmbedModel,
				Embedding:      vector,
				ChunkIndex:     0,
			},
		},
	}, nil
}

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

// readFile reads the full contents of a file.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// truncate cuts text to at most maxChars characters.
func truncate(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
