package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
	pgvector "github.com/pgvector/pgvector-go"
)

// BatchClipEmbed calls embedsvc /embed/image/batch for a batch of
// WorkItems that need CLIP embeddings. Writes the resulting vectors
// back onto each item's Embedding field. Items whose embeddings
// come back nil (per-image failure) are logged and skipped.
func BatchClipEmbed(ctx context.Context, client *clients.EmbedClient, model string, batch []*WorkItem) error {
	inputs := make([]clients.FileInput, len(batch))
	for i, item := range batch {
		inputs[i] = clients.FileInput{
			Filename: item.FileData.FileInfo.Name,
			Data:     item.FileData.Data,
		}
	}

	vectors, err := client.EmbedImageBatch(ctx, inputs)
	if err != nil {
		return fmt.Errorf("clip embed batch: %w", err)
	}

	for i, item := range batch {
		if vectors[i] == nil {
			fmt.Printf("      clip embed: nil result for %s\n", item.FileData.FileInfo.Path)
			continue
		}
		item.Embedding = pgvector.NewVector(vectors[i])
	}
	return nil
}

// BatchTextEmbed calls embedsvc /embed/text for a batch of WorkItems
// that have text content needing embedding. Reads from item.Text,
// writes to item.Embedding.
func BatchTextEmbed(ctx context.Context, client *clients.EmbedClient, model string, batch []*WorkItem) error {
	texts := make([]string, len(batch))
	for i, item := range batch {
		texts[i] = item.Text
	}

	vectors, err := client.EmbedTexts(ctx, texts)
	if err != nil {
		return fmt.Errorf("text embed batch: %w", err)
	}

	for i, item := range batch {
		item.Embedding = pgvector.NewVector(vectors[i])
	}
	return nil
}
