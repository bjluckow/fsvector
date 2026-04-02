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

func processVideo(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	var files []store.File
	frameType := "frame"
	transcriptType := "transcript"

	// 1. extract and embed frames
	frames, err := cfg.ConvertClient.ExtractVideoFrames(ctx, fi.Name, data, cfg.VideoFrameRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract frames %s: %v\n", fi.Path, err)
	} else {
		for _, frame := range frames {
			vector, err := cfg.EmbedClient.EmbedImage(ctx, fi.Name, frame.Data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    embed frame %d %s: %v\n", frame.Index, fi.Path, err)
				continue
			}
			files = append(files, store.File{
				Path:           fi.Path,
				Source:         cfg.Source,
				ContentHash:    fi.Hash,
				Size:           fi.Size,
				MimeType:       fi.MimeType,
				Modality:       "video",
				FileName:       fi.Name,
				FileExt:        fi.Ext,
				FileCreatedAt:  &fi.CreatedAt,
				FileModifiedAt: &fi.ModifiedAt,
				EmbedModel:     cfg.EmbedModel,
				Embedding:      vector,
				ChunkIndex:     frame.Index,
				ChunkType:      &frameType,
				Metadata: map[string]any{
					"timestamp_ms": frame.TimestampMs,
					"frame_index":  frame.Index,
					"fps":          cfg.VideoFrameRate,
				},
			})
		}
	}

	transcriptOffset := len(frames)

	// 2. extract audio track and transcribe
	audioData, err := cfg.ConvertClient.ExtractVideoAudio(ctx, fi.Name, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract audio %s: %v\n", fi.Path, err)
	} else {
		resp, err := cfg.TranscribeClient.Transcribe(ctx, fi.Name+".wav", audioData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    transcribe %s: %v\n", fi.Path, err)
		} else if strings.TrimSpace(resp.Text) != "" {
			chunks := chunk.Split(resp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
			for i, c := range chunks {
				f, err := processTextChunk(ctx, cfg, fi, c, transcriptOffset+i)
				if err != nil {
					fmt.Fprintf(os.Stderr, "    transcript chunk %d %s: %v\n", i, fi.Path, err)
					continue
				}
				if f != nil {
					f.Modality = "video"
					f.ChunkType = &transcriptType
					f.Metadata = map[string]any{
						"duration_seconds": resp.DurationSeconds,
						"language":         resp.Language,
					}
					files = append(files, *f)
				}
			}
		}
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no frames or transcript produced",
		}, nil
	}

	return Result{Files: files}, nil
}
