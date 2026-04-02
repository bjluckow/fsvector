package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

func processImage(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	if fi.Ext != "jpg" && fi.Ext != "jpeg" {
		data, err = cfg.ConvertClient.ConvertToImage(ctx, fi.Name, data)
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
