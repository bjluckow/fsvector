package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
	pgvector "github.com/pgvector/pgvector-go"
)

// BatchClipEmbed calls embedsvc /embed/image/batch for a batch of
// WorkItems that need CLIP embeddings. Writes the resulting vectors
// back onto each item's Embedding field. Items whose embeddings
// come back nil (per-image failure) are logged and skipped.
func BatchClipEmbed(ctx context.Context, client *clients.EmbedClient, model string, batch []*job) error {
	inputs := make([]clients.FileInput, len(batch))
	for i, item := range batch {
		inputs[i] = clients.FileInput{
			Filename: item.fileData.FilePath,
			Data:     item.fileData.Data,
		}
	}

	vectors, err := client.EmbedImageBatch(ctx, inputs)
	if err != nil {
		return fmt.Errorf("clip embed batch: %w", err)
	}

	for i, item := range batch {
		if vectors[i] == nil {
			fmt.Printf("      clip embed: nil result for %s\n", item.fileData.FilePath)
			continue
		}
		item.embedding = pgvector.NewVector(vectors[i])
	}
	return nil
}

// BatchTextEmbed calls embedsvc /embed/text for a batch of WorkItems
// that have text content needing embedding. Reads from item.Text,
// writes to item.Embedding.
func BatchTextEmbed(ctx context.Context, client *clients.EmbedClient, model string, batch []*job) error {
	texts := make([]string, len(batch))
	for i, item := range batch {
		texts[i] = item.text
	}

	vectors, err := client.EmbedTexts(ctx, texts)
	if err != nil {
		return fmt.Errorf("text embed batch: %w", err)
	}

	for i, item := range batch {
		item.embedding = pgvector.NewVector(vectors[i])
	}
	return nil
}

// OCR calls visionsvc /ocr for a single WorkItem. This is called
// concurrently by the ParallelWorker, not batched, because tesseract
// is already fast per-image.
func OCR(ctx context.Context, client *clients.VisionClient, item *job) error {
	resp, err := client.OCR(ctx, item.fileData.FilePath, item.fileData.Data)
	if err != nil {
		return fmt.Errorf("ocr %s: %w", item.fileData.FilePath, err)
	}
	item.text = resp.Text
	return nil
}

// BatchCaption calls visionsvc /caption/batch for a batch of WorkItems.
// Writes the resulting caption strings back onto each item's Text field.
func BatchCaption(ctx context.Context, client *clients.VisionClient, batch []*job) error {
	inputs := make([]clients.FileInput, len(batch))
	for i, item := range batch {
		inputs[i] = clients.FileInput{
			Filename: item.fileData.FilePath,
			Data:     item.fileData.Data,
		}
	}

	captions, err := client.CaptionBatch(ctx, inputs)
	if err != nil {
		return fmt.Errorf("caption batch: %w", err)
	}

	for i, item := range batch {
		if captions[i] == "" {
			fmt.Printf("      caption: empty result for %s\n", item.fileData.FilePath)
		}
		item.text = captions[i]
	}
	return nil
}

// Transcribe calls transcribesvc /transcribe for a single WorkItem.
// Like OCR, this runs via ParallelWorker — Whisper processes one
// audio stream at a time, so concurrency comes from multiple
// simultaneous requests, not batching.
func Transcribe(ctx context.Context, client *clients.TranscribeClient, j *job) error {
	resp, err := client.Transcribe(ctx, j.fileData.FilePath, j.fileData.Data)
	if err != nil {
		return fmt.Errorf("transcribe %s: %w", j.fileData.FilePath, err)
	}
	j.text = resp.Text
	j.metadata = mustJSON(map[string]any{
		"duration_seconds": resp.DurationSeconds,
		"language":         resp.Language,
	})
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
