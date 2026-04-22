package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
	"github.com/sourcegraph/conc/pool"
)

// Index downloads, converts, registers files in DB, emits items on output channel.
func (idx *Indexer) Index(ctx context.Context, files []model.SourceFile) error {
	total := len(files)
	var extracted atomic.Int32

	ep := pool.New().WithMaxGoroutines(idx.cfg.DownloadWorkers)
	for _, sf := range files {
		sf := sf
		ep.Go(func() {
			mod, ok := model.FileModality(sf.Ext)
			if !ok {
				idx.progress.incSkipped()
				extracted.Add(1)
				return
			}

			items, err := idx.indexFile(ctx, sf, mod)
			n := extracted.Add(1)

			if err != nil {
				fmt.Printf("    index %s: %v\n", sf.Path, err)
				idx.progress.addError(sf.Path)
			} else if len(items) == 0 {
				idx.progress.incSkipped()
			} else {
				for _, item := range items {
					select {
					case idx.output <- item:
					case <-ctx.Done():
						return
					}
				}
				idx.progress.incIndexed()
			}

			if n%100 == 0 || int(n) == total {
				fmt.Printf("  [%d/%d] indexing progress\n", n, total)
			}
		})
	}
	ep.Wait()
	return nil
}

// indexFile dispatches to the appropriate modality-specific indexer.
func (idx *Indexer) indexFile(ctx context.Context, sf model.SourceFile, modality model.Modality) ([]model.Item, error) {
	switch modality {
	case model.ModalityText:
		return idx.indexText(ctx, sf)
	case model.ModalityImage:
		return idx.indexImage(ctx, sf)
	case model.ModalityAudio:
		return idx.indexAudio(ctx, sf)
	case model.ModalityVideo:
		return idx.indexVideo(ctx, sf)
	case model.ModalityEmail:
		return idx.indexEmail(ctx, sf)
	default:
		return nil, nil
	}
}

func (idx *Indexer) indexText(ctx context.Context, sf model.SourceFile) ([]model.Item, error) {
	data, err := idx.reader.Read(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sf.Path, err)
	}

	ext := strings.ToLower(sf.Ext)
	if ext != "txt" && ext != "md" {
		data, err = idx.cfg.ConvertClient.ConvertToText(ctx, sf.Name, data)
		if err != nil {
			return nil, fmt.Errorf("convert %s: %w", sf.Path, err)
		}
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, nil
	}

	fileID, err := store.UpsertFile(ctx, model.File{
		Path:        sf.Path,
		Source:      sf.SourceURI,
		Modality:    "text",
		Name:        sf.Name,
		Ext:         sf.Ext,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		CreatedAt:   sf.CreatedAt,
		ModifiedAt:  sf.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", sf.Path, err)
	}

	textItemID, err := store.UpsertItem(ctx, fileID, model.Item{
		ItemType:    "text",
		ItemName:    sf.Name,
		MimeType:    "text/plain",
		Size:        int64(len(text)),
		ContentHash: sf.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert item %s: %w", sf.Path, err)
	}

	status, err := store.GetFileStatus(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", sf.Path, err)
	}
	needed := pipeline.AssessNeededStages("text", status, sf.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	chunks := chunk.Split(text, idx.cfg.ChunkSize, idx.cfg.ChunkOverlap, idx.cfg.MinChunkSize)
	if len(chunks) == 0 {
		return nil, nil
	}

	var items []model.Item
	for i, c := range chunks {
		items = append(items, model.Item{
			ID:        textItemID,
			FileID:    fileID,
			ItemType:  "text",
			ItemIndex: i,
			Modality:  "text",
			Text:      c,
			FilePath:  sf.Path,
		})
	}
	return items, nil
}

func (idx *Indexer) indexImage(ctx context.Context, sf model.SourceFile) ([]model.Item, error) {
	data, err := idx.reader.Read(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sf.Path, err)
	}

	ext := strings.ToLower(sf.Ext)
	if ext != "jpg" && ext != "jpeg" && ext != "png" {
		data, err = idx.cfg.ConvertClient.ConvertToImage(ctx, sf.Name, data)
		if err != nil {
			return nil, fmt.Errorf("convert %s: %w", sf.Path, err)
		}
	}

	fileID, err := store.UpsertFile(ctx, model.File{
		Path:        sf.Path,
		Source:      sf.SourceURI,
		Modality:    "image",
		Name:        sf.Name,
		Ext:         sf.Ext,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		CreatedAt:   sf.CreatedAt,
		ModifiedAt:  sf.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", sf.Path, err)
	}

	itemID, err := store.UpsertItem(ctx, fileID, model.Item{
		ItemType:    "image",
		ItemName:    sf.Name,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert item %s: %w", sf.Path, err)
	}

	status, err := store.GetFileStatus(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", sf.Path, err)
	}

	needed := pipeline.AssessNeededStages("image", status, sf.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	return []model.Item{{
		ID:        itemID,
		FileID:    fileID,
		ItemType:  "image",
		ItemName:  sf.Name,
		MimeType:  sf.MimeType,
		Size:      sf.Size,
		ItemIndex: 0,
		Modality:  "image",
		Data:      data,
		FilePath:  sf.Path,
	}}, nil
}

func (idx *Indexer) indexAudio(ctx context.Context, sf model.SourceFile) ([]model.Item, error) {
	data, err := idx.reader.Read(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sf.Path, err)
	}

	ext := strings.ToLower(sf.Ext)
	if ext != "wav" {
		data, err = idx.cfg.ConvertClient.NormalizeAudio(ctx, sf.Name, data)
		if err != nil {
			return nil, fmt.Errorf("normalize %s: %w", sf.Path, err)
		}
	}

	fileID, err := store.UpsertFile(ctx, model.File{
		Path:        sf.Path,
		Source:      sf.SourceURI,
		Modality:    "audio",
		Name:        sf.Name,
		Ext:         sf.Ext,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		CreatedAt:   sf.CreatedAt,
		ModifiedAt:  sf.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", sf.Path, err)
	}

	itemID, err := store.UpsertItem(ctx, fileID, model.Item{
		ItemType:    "audio",
		ItemName:    sf.Name,
		MimeType:    "audio/wav",
		Size:        int64(len(data)),
		ContentHash: sf.Hash,
		ItemIndex:   0,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert item %s: %w", sf.Path, err)
	}

	status, err := store.GetFileStatus(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", sf.Path, err)
	}
	needed := pipeline.AssessNeededStages("audio", status, sf.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	return []model.Item{{
		ID:        itemID,
		FileID:    fileID,
		ItemType:  "audio",
		ItemName:  sf.Name,
		MimeType:  "audio/wav",
		Size:      int64(len(data)),
		ItemIndex: 0,
		Modality:  "audio",
		Data:      data,
		FilePath:  sf.Path,
	}}, nil
}

func (idx *Indexer) indexVideo(ctx context.Context, sf model.SourceFile) ([]model.Item, error) {
	data, err := idx.reader.Read(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sf.Path, err)
	}

	fileID, err := store.UpsertFile(ctx, model.File{
		Path:        sf.Path,
		Source:      sf.SourceURI,
		Modality:    "video",
		Name:        sf.Name,
		Ext:         sf.Ext,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		CreatedAt:   sf.CreatedAt,
		ModifiedAt:  sf.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", sf.Path, err)
	}

	status, err := store.GetFileStatus(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("assess %s: %w", sf.Path, err)
	}
	needed := pipeline.AssessNeededStages("video", status, sf.Hash)
	if len(needed) == 0 {
		return nil, nil
	}

	needSet := make(map[string]bool, len(needed))
	for _, s := range needed {
		needSet[string(s)] = true
	}

	var items []model.Item

	// frames
	if needSet["clip_embed"] || needSet["caption"] {
		frames, err := idx.cfg.ConvertClient.ExtractVideoFrames(ctx, sf.Name, data, idx.cfg.VideoFrameRate)
		if err != nil {
			fmt.Printf("      extract frames %s: %v\n", sf.Path, err)
		} else {
			for _, frame := range frames {
				itemID, err := store.UpsertItem(ctx, fileID, model.Item{
					ItemType:  "frame",
					ItemName:  fmt.Sprintf("frame_%06d.jpg", frame.Index),
					MimeType:  "image/jpeg",
					Size:      int64(len(frame.Data)),
					ItemIndex: frame.Index,
					Metadata: mustJSON(map[string]any{
						"timestamp_ms": frame.TimestampMs,
						"fps":          idx.cfg.VideoFrameRate,
					}),
				})
				if err != nil {
					fmt.Printf("      upsert frame %s[%d]: %v\n", sf.Path, frame.Index, err)
					continue
				}
				items = append(items, model.Item{
					ID:        itemID,
					FileID:    fileID,
					ItemType:  "frame",
					ItemIndex: frame.Index,
					Modality:  "video",
					Data:      frame.Data,
					FilePath:  sf.Path,
				})
			}
		}
	}

	// audio track
	if needSet["transcribe"] {
		audioData, err := idx.cfg.ConvertClient.ExtractVideoAudio(ctx, sf.Name, data)
		if err != nil {
			fmt.Printf("      extract audio %s: %v\n", sf.Path, err)
		} else {
			itemID, err := store.UpsertItem(ctx, fileID, model.Item{
				ItemType:  "audio_track",
				ItemName:  sf.Name,
				MimeType:  "audio/wav",
				Size:      int64(len(audioData)),
				ItemIndex: 0,
			})
			if err != nil {
				fmt.Printf("      upsert audio %s: %v\n", sf.Path, err)
			} else {
				items = append(items, model.Item{
					ID:        itemID,
					FileID:    fileID,
					ItemType:  "audio_track",
					ItemIndex: 0,
					Modality:  "video",
					Data:      audioData,
					FilePath:  sf.Path,
				})
			}
		}
	}

	return items, nil
}

func (idx *Indexer) indexEmail(ctx context.Context, sf model.SourceFile) ([]model.Item, error) {
	data, err := idx.reader.Read(ctx, sf.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", sf.Path, err)
	}

	parsed, err := idx.cfg.ConvertClient.ParseEmail(ctx, sf.Name, data)
	if err != nil {
		return nil, fmt.Errorf("parse email %s: %w", sf.Path, err)
	}

	fileID, err := store.UpsertFile(ctx, model.File{
		Path:        sf.Path,
		Source:      sf.SourceURI,
		Modality:    "email",
		Name:        sf.Name,
		Ext:         sf.Ext,
		MimeType:    sf.MimeType,
		Size:        sf.Size,
		ContentHash: sf.Hash,
		CreatedAt:   sf.CreatedAt,
		ModifiedAt:  sf.ModifiedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("upsert file %s: %w", sf.Path, err)
	}

	var items []model.Item

	body := strings.TrimSpace(parsed.Body)
	if body != "" {
		bodyItemID, err := store.UpsertItem(ctx, fileID, model.Item{
			ItemType:  "body",
			ItemName:  parsed.Subject,
			MimeType:  "text/plain",
			Size:      int64(len(body)),
			ItemIndex: 0,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert body %s: %w", sf.Path, err)
		}

		chunks := chunk.Split(body, idx.cfg.ChunkSize, idx.cfg.ChunkOverlap, idx.cfg.MinChunkSize)
		for i, c := range chunks {
			items = append(items, model.Item{
				ID:        bodyItemID,
				FileID:    fileID,
				ItemType:  "body",
				ItemIndex: i,
				Modality:  "email",
				Text:      c,
				FilePath:  sf.Path,
			})
		}
	}

	// TODO M0.9.5: decode base64 attachments, route through
	// appropriate indexer (image, text, etc.)

	return items, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
