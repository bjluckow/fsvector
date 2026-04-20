package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

type Pipeline struct {
	Reader           source.FileReader
	EmbedClient      *clients.EmbedClient
	ConvertClient    *clients.ConvertClient
	TranscribeClient *clients.TranscribeClient
	VisionClient     *clients.VisionClient
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
	Files      []store.UpsertFile
	Skipped    bool
	SkipReason string
}

// ReadAndProcessFile reads the file and processes it.
func (pl *Pipeline) ReadAndProcessFile(ctx context.Context, fi source.FileInfo) (Result, error) {
	data, err := pl.Reader.Read(ctx, fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}
	return pl.ProcessFileData(ctx, fi, data)
}

// ProcessFileData processes a file with already-read bytes.
// Used for email attachments where bytes are decoded from base64.
func (pl *Pipeline) ProcessFileData(ctx context.Context, fi source.FileInfo, data []byte) (Result, error) {
	if fi.Size < pl.MinEmbedSize {
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("file too small (< %d bytes)", pl.MinEmbedSize),
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
		return pl.processText(ctx, fi, data)
	case "image":
		return pl.processImage(ctx, fi, data)
	case "audio":
		return pl.processAudio(ctx, fi, data)
	case "video":
		return pl.processVideo(ctx, fi, data)
	case "email":
		return pl.processEmail(ctx, fi, data)
	default:
		return Result{
			Skipped:    true,
			SkipReason: fmt.Sprintf("unhandled modality: %s", modality),
		}, nil
	}
}
