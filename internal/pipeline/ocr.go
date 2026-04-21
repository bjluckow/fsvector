package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

// OCR calls visionsvc /ocr for a single WorkItem. This is called
// concurrently by the ParallelWorker, not batched, because tesseract
// is already fast per-image.
func OCR(ctx context.Context, client *clients.VisionClient, item *WorkItem) error {
	resp, err := client.OCR(ctx, item.FileData.FileInfo.Name, item.FileData.Data)
	if err != nil {
		return fmt.Errorf("ocr %s: %w", item.FileData.FileInfo.Path, err)
	}
	item.Text = resp.Text
	return nil
}
