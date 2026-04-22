package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

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
