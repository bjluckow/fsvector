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

func (pl Pipeline) processVideo(ctx context.Context, fi source.FileInfo, data []byte) (Result, error) {
	var files []store.UpsertFile

	// frames
	frames, err := pl.ConvertClient.ExtractVideoFrames(ctx, fi.Name, data, pl.VideoFrameRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract frames %s: %v\n", fi.Path, err)
	} else {
		for _, frame := range frames {
			f, err := pl.processVideoFrame(ctx, fi, frame)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    frame %d %s: %v\n", frame.Index, fi.Path, err)
				continue
			}
			files = append(files, f)
		}
	}

	// audio transcript
	files = append(files, pl.processVideoAudio(ctx, fi, data, len(frames))...)

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no frames or transcript produced",
		}, nil
	}

	return Result{Files: files}, nil
}

// processVideoFrame embeds a single video frame and gets its caption.
func (pl Pipeline) processVideoFrame(
	ctx context.Context,
	fi source.FileInfo,
	frame clients.VideoFrame,
) (store.UpsertFile, error) {
	vector, err := pl.EmbedClient.EmbedImage(ctx, fi.Name, frame.Data)
	if err != nil {
		return store.UpsertFile{}, fmt.Errorf("embed: %w", err)
	}

	captionText := pl.describeImage(ctx, fi, frame.Data)
	frameType := "frame"

	return store.UpsertFile{
		Path:           fi.Path,
		Source:         pl.Source,
		ContentHash:    fi.Hash,
		Size:           fi.Size,
		MimeType:       fi.MimeType,
		Modality:       "video",
		FileName:       fi.Name,
		FileExt:        fi.Ext,
		FileCreatedAt:  &fi.CreatedAt,
		FileModifiedAt: &fi.ModifiedAt,
		EmbedModel:     pl.EmbedModel,
		Embedding:      vector,
		ChunkIndex:     frame.Index,
		ChunkType:      &frameType,
		TextContent:    &captionText,
		Metadata: map[string]any{
			"timestamp_ms": frame.TimestampMs,
			"frame_index":  frame.Index,
			"fps":          pl.VideoFrameRate,
		},
	}, nil
}

// processVideoAudio extracts, transcribes and chunks the audio track of a video.
// Returns transcript chunks starting at chunkOffset.
// Non-fatal — returns nil on error or empty transcript.
func (pl Pipeline) processVideoAudio(
	ctx context.Context,
	fi source.FileInfo,
	videoData []byte,
	chunkOffset int,
) []store.UpsertFile {
	audioData, err := pl.ConvertClient.ExtractVideoAudio(ctx, fi.Name, videoData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    extract audio %s: %v\n", fi.Path, err)
		return nil
	}

	resp, err := pl.TranscribeClient.Transcribe(ctx, fi.Name+".wav", audioData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "    transcribe %s: %v\n", fi.Path, err)
		return nil
	}
	if strings.TrimSpace(resp.Text) == "" {
		return nil
	}

	transcriptType := "transcript"
	chunks := chunk.Split(resp.Text, pl.ChunkSize, pl.ChunkOverlap, pl.MinChunkSize)
	var files []store.UpsertFile
	for i, c := range chunks {
		f, err := pl.processTextChunk(ctx, fi, c, chunkOffset+i)
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
