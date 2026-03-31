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

// textExts are file extensions we treat as text-modal and send through pandoc if needed.
var textExts = map[string]string{
	"txt":  "txt",
	"md":   "txt",
	"go":   "txt",
	"py":   "txt",
	"js":   "txt",
	"ts":   "txt",
	"html": "txt",
	"htm":  "txt",
	"css":  "txt",
	"json": "txt",
	"yaml": "txt",
	"yml":  "txt",
	"toml": "txt",
	"sh":   "txt",
	"pdf":  "txt",
	"docx": "txt",
	"doc":  "txt",
	"odt":  "txt",
	"rtf":  "txt",
}

// imageExts are file extensions we treat as image-modal.
var imageExts = map[string]string{
	"jpg":  "jpeg",
	"jpeg": "jpeg",
	"png":  "jpeg",
	"gif":  "jpeg",
	"webp": "jpeg",
	"bmp":  "jpeg",
	"tiff": "jpeg",
	"tif":  "jpeg",
	"heic": "jpeg",
	"heif": "jpeg",
}

// Config holds the dependencies for the pipeline.
type Config struct {
	TextEmbed     *embed.TextClient
	ImageEmbed    *embed.ImageClient
	ConvertClient *convert.Client
	EmbedModel    string
	Source        string
	MinEmbedSize  int64
}

// Result is returned after a file has been processed.
type Result struct {
	File       store.File
	Skipped    bool
	SkipReason string
}

// Process runs a single FileInfo through the full pipeline:
// detect modality → convert → embed → return store.File ready for upsert.
func Process(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	ext := strings.ToLower(fi.Ext)

	// skip files that are too small to be worth embedding
	if fi.Size < cfg.MinEmbedSize {
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("file too small (< %d bytes)", cfg.MinEmbedSize),
		}, nil
	}

	// detect modality
	if targetFmt, ok := textExts[ext]; ok {
		return processText(ctx, cfg, fi, targetFmt)
	}
	if targetFmt, ok := imageExts[ext]; ok {
		return processImage(ctx, cfg, fi, targetFmt)
	}

	// unsupported type — skip cleanly
	return Result{
		Skipped:    true,
		SkipReason: fmt.Sprintf("unsupported extension: %s", ext),
	}, nil
}

func processText(ctx context.Context, cfg Config, fi fsindex.FileInfo, targetFmt string) (Result, error) {
	var text string

	// plain text formats can be read directly without conversion
	plainExts := map[string]bool{
		"txt": true, "md": true, "go": true, "py": true,
		"js": true, "ts": true, "css": true, "json": true,
		"yaml": true, "yml": true, "toml": true, "sh": true,
	}

	if plainExts[fi.Ext] {
		data, err := readFile(fi.Path)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
		}
		text = string(data)
	} else {
		// send through convertsvc (pdf, docx, etc.)
		data, err := readFile(fi.Path)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
		}
		converted, err := cfg.ConvertClient.Convert(ctx, fi.Name, data, targetFmt)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
		text = string(converted)
	}

	// truncate to avoid blowing the model's token limit
	text = truncate(text, 4096)

	// embed
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
			EmbedModel:     cfg.EmbedModel,
			Embedding:      vectors[0],
			ChunkIndex:     0,
		},
	}, nil
}

func processImage(ctx context.Context, cfg Config, fi fsindex.FileInfo, targetFmt string) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// convert to normalized format if needed
	if fi.Ext != targetFmt && fi.Ext != "jpg" {
		data, err = cfg.ConvertClient.Convert(ctx, fi.Name, data, targetFmt)
		if err != nil {
			return Result{}, fmt.Errorf("convert image %s: %w", fi.Path, err)
		}
	}

	// embed as image
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
			EmbedModel:     cfg.EmbedModel,
			Embedding:      vector,
			ChunkIndex:     0,
		},
	}, nil
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
