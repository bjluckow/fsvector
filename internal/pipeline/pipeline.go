package pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bjluckow/fsvector/internal/chunk"
	"github.com/bjluckow/fsvector/internal/clients/convert"
	"github.com/bjluckow/fsvector/internal/clients/embed"
	"github.com/bjluckow/fsvector/internal/clients/transcribe"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/store"
)

// Config holds the dependencies for the pipeline.
type Config struct {
	EmbedClient      *embed.Client
	ConvertClient    *convert.Client
	TranscribeClient *transcribe.Client
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

// readFile reads the full contents of a file.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// Process runs a single FileInfo through the full pipeline:
// detect modality → convert → embed → return store.File ready for upsert.
func Process(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
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

func processText(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	var text string
	plainExts := map[string]bool{
		"txt": true, "md": true, "go": true, "py": true,
		"js": true, "ts": true, "css": true, "json": true,
		"yaml": true, "yml": true, "toml": true, "sh": true,
		"rs": true, "c": true, "cpp": true, "h": true,
		"java": true, "rb": true, "html": true, "htm": true,
	}

	if !plainExts[fi.Ext] {
		converted, err := cfg.ConvertClient.ConvertToText(ctx, fi.Name, data)
		if err != nil {
			return Result{}, fmt.Errorf("convert %s: %w", fi.Path, err)
		}
		text = string(converted)
	} else {
		text = string(data)
	}

	chunks := chunk.Split(text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	if len(chunks) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "no embeddable content after chunking",
		}, nil
	}

	var files []store.File
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, i)
		if err != nil {
			return Result{}, fmt.Errorf("chunk %d of %s: %w", i, fi.Path, err)
		}
		if f != nil {
			files = append(files, *f)
		}
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "all chunks failed to embed",
		}, nil
	}

	return Result{Files: files}, nil
}

// processTextChunk embeds a single text chunk and returns a store.File.
// Returns nil if the embed service returns no vectors.
func processTextChunk(ctx context.Context, cfg Config, fi fsindex.FileInfo, text string, chunkIndex int) (*store.File, error) {
	vectors, err := cfg.EmbedClient.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) == 0 {
		return nil, nil
	}

	return &store.File{
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
		ChunkIndex:     chunkIndex,
		TextContent:    &text,
	}, nil
}

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

func processAudio(ctx context.Context, cfg Config, fi fsindex.FileInfo) (Result, error) {
	data, err := readFile(fi.Path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", fi.Path, err)
	}

	resp, err := cfg.TranscribeClient.Transcribe(ctx, fi.Name, data)
	if err != nil {
		return Result{}, fmt.Errorf("transcribe %s: %w", fi.Path, err)
	}
	if strings.TrimSpace(resp.Text) == "" {
		return Result{
			Skipped:    true,
			SkipReason: "empty transcript",
		}, nil
	}

	chunks := chunk.Split(resp.Text, cfg.ChunkSize, cfg.ChunkOverlap, cfg.MinChunkSize)
	if len(chunks) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "transcript too short to chunk",
		}, nil
	}

	chunkType := "transcript"
	metadata := map[string]any{
		"duration_seconds": resp.DurationSeconds,
		"language":         resp.Language,
	}

	var files []store.File
	for i, c := range chunks {
		f, err := processTextChunk(ctx, cfg, fi, c, i)
		if err != nil {
			return Result{}, fmt.Errorf("chunk %d of %s: %w", i, fi.Path, err)
		}
		if f != nil {
			f.Modality = "audio"
			f.ChunkType = &chunkType
			f.Metadata = metadata
			files = append(files, *f)
		}
	}

	if len(files) == 0 {
		return Result{
			Skipped:    true,
			SkipReason: "all transcript chunks failed to embed",
		}, nil
	}

	return Result{Files: files}, nil
}

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
