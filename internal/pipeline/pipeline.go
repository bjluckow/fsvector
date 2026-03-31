package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/convert"
	"github.com/bjluckow/fsvector/internal/embed"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

// Config holds the dependencies for the pipeline.
type Config struct {
	TextEmbed       *embed.TextClient
	ImageEmbed      *embed.ImageClient
	ConvertClient   *convert.Client
	TextEmbedModel  string
	ImageEmbedModel string
	Source          string
	MinEmbedSize    int64
}

// Result is returned after a file has been processed.
type Result struct {
	File       store.File
	Skipped    bool
	SkipReason string
}

// Process runs a single FileInfo through the full pipeline:
// detect modality → convert if needed → embed → return store.File
func Process(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	// skip files that are too small
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
	default:
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("unhandled modality: %s", modality),
		}, nil
	}
}

func processText(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	var text string

	if target := ConvertTarget(fi.Ext); target != "" {
		data, err := readFile(fi.Path)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
		}
		text = string(data)
	} else {
		data, err := readFile(fi.Path)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
		}
		converted, err := cfg.ConvertClient.Convert(ctx, fi.Name, data, "txt")
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
		text = string(converted)
	}

	text = truncate(text, 4096)

	if len(strings.TrimSpace(text)) < 50 {
		return Result{
			Skipped:    true,
			SkipReason: "content too short to embed meaningfully",
		}, nil
	}

	vectors, err := cfg.TextEmbed.EmbedTexts(ctx, []string{text})
	if err != nil {
		return Result{}, fmt.Errorf("embed text %s: %w", fi.Path, err)
	}
	if len(vectors) == 0 {
		return Result{}, fmt.Errorf("embed returned no vectors for %s", fi.Path)
	}

	return Result{
		File: store.File{
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
			EmbedModel:     cfg.TextEmbedModel,
			Embedding:      vectors[0],
			ChunkIndex:     0,
		},
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

	vector, err := cfg.ImageEmbed.EmbedImage(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("embed image %s: %w", fi.Path, err)
	}

	return Result{
		File: store.File{
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
			EmbedModel:     cfg.ImageEmbedModel,
			Embedding:      vector,
			ChunkIndex:     0,
		},
	}, nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func truncate(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
