package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

func processImage(ctx context.Context, cfg Config, fi source.FileInfo) (Result, error) {
	data, err := readFile(ctx, cfg, fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	// convert to JPEG if needed
	if fi.Ext != "jpg" && fi.Ext != "jpeg" {
		data, err = cfg.ConvertClient.ConvertToImage(ctx, fi.Name, data)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
	}

	// CLIP embedding
	vector, err := cfg.EmbedClient.EmbedImage(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("embed %s: %w", fi.Path, err)
	}

	// vision: caption + OCR
	captionText := describeImage(ctx, cfg, fi, data)
	ocrFiles := extractImageText(ctx, cfg, fi, data, 1)

	primaryRow := store.File{
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
		TextContent:    &captionText,
	}

	files := append([]store.File{primaryRow}, ocrFiles...)
	return Result{Files: files}, nil
}

// describeImage returns a caption for the image.
// Non-fatal — returns empty string on error.
func describeImage(
	ctx context.Context,
	cfg Config,
	fi source.FileInfo,
	imageData []byte,
) string {
	if cfg.VisionClient == nil {
		return ""
	}
	capResp, err := cfg.VisionClient.Caption(ctx, fi.Name, imageData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    caption %s: %v\n", fi.Path, err)
		return ""
	}
	return capResp.Caption
}

// extractImageText runs OCR and returns embedded text chunks.
// Non-fatal — returns nil on error or no text found.
func extractImageText(
	ctx context.Context,
	cfg Config,
	fi source.FileInfo,
	imageData []byte,
	chunkOffset int,
) []store.File {
	if cfg.VisionClient == nil {
		return nil
	}
	ocrResp, err := cfg.VisionClient.OCR(ctx, fi.Name, imageData)
	if err != nil || strings.TrimSpace(ocrResp.Text) == "" {
		return nil
	}

	chunks := chunk.Split(ocrResp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	ocrType := "ocr"
	var files []store.File
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, chunkOffset+i)
		if err != nil || f == nil {
			continue
		}
		f.Modality = "image"
		f.ChunkType = &ocrType
		files = append(files, *f)
	}
	return files
}
