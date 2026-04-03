package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients/convert"
	"github.com/bjluckow/fsvector/internal/clients/embed"
	"github.com/bjluckow/fsvector/internal/clients/transcribe"
	"github.com/bjluckow/fsvector/internal/clients/vision"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

// Config holds the dependencies for the pipeline.
type Config struct {
	Reader           source.FileReader
	EmbedClient      *embed.Client
	ConvertClient    *convert.Client
	TranscribeClient *transcribe.Client
	VisionClient     *vision.Client
	EmbedModel       string
	Source           string
	MinEmbedSize     int64
	ChunkSize        int
	ChunkOverlap     int
	MinChunkSize     int
	VideoFrameRate   float64
}

// Result is returned after a file has been processed.
type Result struct {
	Files      []store.File
	Skipped    bool
	SkipReason string
}

func readFile(ctx context.Context, cfg Config, path string) ([]byte, error) {
	return cfg.Reader.Read(ctx, path)
}

// Process runs a single FileInfo through the full pipeline:
// detect modality → convert → embed → return store.File ready for upsert.
func Process(ctx context.Context, cfg Config, fi source.FileInfo) (Result, error) {
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
	case "video":
		return processVideo(ctx, cfg, fi)
	default:
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("unhandled modality: %s", modality),
		}, nil
	}
}
