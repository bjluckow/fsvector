package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
)

func processVideo(ctx context.Context, cfg Config, fi source.FileInfo) (Result, error) {
	data, err := readFile(ctx, cfg, fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	var files []store.UpsertFile

	// frames
	frames, err := cfg.ConvertClient.ExtractVideoFrames(ctx, fi.Name, data, cfg.VideoFrameRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract frames %s: %v\n", fi.Path, err)
	} else {
		for _, frame := range frames {
			f, err := processVideoFrame(ctx, cfg, fi, frame)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    frame %d %s: %v\n", frame.Index, fi.Path, err)
				continue
			}
			files = append(files, f)
		}
	}

	// audio transcript
	files = append(files, processVideoAudio(ctx, cfg, fi, data, len(frames))...)

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no frames or transcript produced",
		}, nil
	}

	return Result{Files: files}, nil
}

// processVideoFrame embeds a single video frame and gets its caption.
func processVideoFrame(
	ctx context.Context,
	cfg Config,
	fi source.FileInfo,
	frame clients.VideoFrame,
) (store.UpsertFile, error) {
	vector, err := cfg.EmbedClient.EmbedImage(ctx, fi.Name, frame.Data)
	if err != nil {
		return store.UpsertFile{}, fmt.Errorf("embed: %w", err)
	}

	captionText := describeImage(ctx, cfg, fi, frame.Data)
	frameType := "frame"

	return store.UpsertFile{
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
		TextContent:    &captionText,
		Metadata: map[string]any{
			"timestamp_ms": frame.TimestampMs,
			"frame_index":  frame.Index,
			"fps":          cfg.VideoFrameRate,
		},
	}, nil
}

// processVideoAudio extracts, transcribes and chunks the audio track of a video.
// Returns transcript chunks starting at chunkOffset.
// Non-fatal — returns nil on error or empty transcript.
func processVideoAudio(
	ctx context.Context,
	cfg Config,
	fi source.FileInfo,
	videoData []byte,
	chunkOffset int,
) []store.UpsertFile {
	audioData, err := cfg.ConvertClient.ExtractVideoAudio(ctx, fi.Name, videoData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract audio %s: %v\n", fi.Path, err)
		return nil
	}

	resp, err := cfg.TranscribeClient.Transcribe(ctx, fi.Name+".wav", audioData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    transcribe %s: %v\n", fi.Path, err)
		return nil
	}
	if strings.TrimSpace(resp.Text) == "" {
		return nil
	}

	transcriptType := "transcript"
	chunks := chunk.Split(resp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	var files []store.UpsertFile
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, chunkOffset+i)
		if err != nil || f == nil {
			continue
		}
		f.Modality = "video"
		f.ChunkType = &transcriptType
		f.Metadata = map[string]any{
			"duration_seconds": resp.DurationSeconds,
			"language":         resp.Language,
		}
		files = append(files, *f)
	}
	return files
}
