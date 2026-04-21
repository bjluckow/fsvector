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
func Transcribe(ctx context.Context, client *clients.TranscribeClient, item *WorkItem) error {
	resp, err := client.Transcribe(ctx, item.FileData.FileInfo.Name, item.FileData.Data)
	if err != nil {
		return fmt.Errorf("transcribe %s: %w", item.FileData.FileInfo.Path, err)
	}
	item.Text = resp.Text
	return nil
}
