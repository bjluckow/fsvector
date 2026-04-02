package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

func processImage(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
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
	captionText, ocrFiles, err := describeImage(ctx, cfg, fi, data, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    vision %s: %v\n", fi.Path, err)
		captionText = ""
		ocrFiles = nil
	}

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

// describeImage calls visionsvc to get a caption and OCR text.
// captionText is returned for the primary row's text_content.
// ocrFiles are additional rows starting at chunkOffset.
// Non-fatal — returns empty caption and nil ocrFiles on error.
func describeImage(
	ctx context.Context,
	cfg Config,
	fi fsindex.FileInfo,
	imageData []byte,
	chunkOffset int,
) (captionText string, ocrFiles []store.File, err error) {
	if cfg.VisionClient == nil {
		return "", nil, nil
	}

	// caption
	capResp, err := cfg.VisionClient.Caption(ctx, fi.Name, imageData)
	if err != nil {
		return "", nil, fmt.Errorf("caption: %w", err)
	}
	captionText = capResp.Caption

	// OCR
	ocrResp, err := cfg.VisionClient.OCR(ctx, fi.Name, imageData)
	if err != nil || strings.TrimSpace(ocrResp.Text) == "" {
		return captionText, nil, nil
	}

	// chunk OCR text
	chunks := chunk.Split(ocrResp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	ocrType := "ocr"
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, chunkOffset+i)
		if err != nil || f == nil {
			continue
		}
		f.Modality = "image"
		f.ChunkType = &ocrType
		f.Embedding = nil
		ocrFiles = append(ocrFiles, *f)
	}

	return captionText, ocrFiles, nil
}
