package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

// Transcribe calls transcribesvc /transcribe for a single WorkItem.
// Like OCR, this runs via ParallelWorker — Whisper processes one
// audio stream at a time, so concurrency comes from multiple
// simultaneous requests, not batching.
func Transcribe(ctx context.Context, client *clients.TranscribeClient, item *job) error {
	resp, err := client.Transcribe(ctx, item.fileData.FilePath, item.fileData.Data)
	if err != nil {
		return fmt.Errorf("transcribe %s: %w", item.fileData.FilePath, err)
	}
	item.text = resp.Text
	return nil
}
