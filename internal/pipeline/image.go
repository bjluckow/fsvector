package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
)

func (pl Pipeline) processImage(ctx context.Context, fi source.FileInfo, data []byte) (Result, error) {
	var err error

	// convert to JPEG if needed
	if fi.Ext != "jpg" && fi.Ext != "jpeg" {
		data, err = pl.ConvertClient.ConvertToImage(ctx, fi.Name, data)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
	}

	// CLIP embedding
	vector, err := pl.EmbedClient.EmbedImage(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("embed %s: %w", fi.Path, err)
	}

	// vision: caption + OCR
	captionText := pl.describeImage(ctx, fi, data)
	ocrFiles := pl.extractImageText(ctx, fi, data, 1)

	primaryRow := store.UpsertFile{
		Path:           fi.Path,
		Source:         pl.Source,
		ContentHash:    fi.Hash,
		Size:           fi.Size,
		MimeType:       fi.MimeType,
		Modality:       "image",
		FileName:       fi.Name,
		FileExt:        fi.Ext,
		FileCreatedAt:  &fi.CreatedAt,
		FileModifiedAt: &fi.ModifiedAt,
		EmbedModel:     pl.EmbedModel,
		Embedding:      vector,
		ChunkIndex:     0,
		TextContent:    &captionText,
	}

	files := append([]store.UpsertFile{primaryRow}, ocrFiles...)
	return Result{Files: files}, nil
}

// describeImage returns a caption for the image.
// Non-fatal — returns empty string on error.
func (pl Pipeline) describeImage(
	ctx context.Context,
	fi source.FileInfo,
	imageData []byte,
) string {
	if pl.VisionClient == nil {
		return ""
	}
	capResp, err := pl.VisionClient.Caption(ctx, fi.Name, imageData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    caption %s: %v\n", fi.Path, err)
		return ""
	}
	return capResp.Caption
}

// extractImageText runs OCR and returns embedded text chunks.
// Non-fatal — returns nil on error or no text found.
func (pl Pipeline) extractImageText(
	ctx context.Context,
	fi source.FileInfo,
	imageData []byte,
	chunkOffset int,
) []store.UpsertFile {
	if pl.VisionClient == nil {
		return nil
	}
	ocrResp, err := pl.VisionClient.OCR(ctx, fi.Name, imageData)
	if err != nil || strings.TrimSpace(ocrResp.Text) == "" {
		return nil
	}

	chunks := chunk.Split(ocrResp.Text, pl.ChunkSize, pl.ChunkOverlap, pl.MinChunkSize)
	ocrType := "ocr"
	var files []store.UpsertFile
	for i, c := range chunks {
		f, err := pl.processTextChunk(ctx, fi, c, chunkOffset+i)
		if err != nil || f == nil {
			continue
		}
		f.Modality = "image"
		f.ChunkType = &ocrType
		files = append(files, *f)
	}
	return files
}
