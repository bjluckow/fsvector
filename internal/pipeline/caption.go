package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

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
