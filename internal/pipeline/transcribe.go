package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

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
