package indexer

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients"
)

// ConvertToText calls convertsvc to convert a document to plain text.
// Used by TextExtractor and EmailExtractor during phase 1.
func ConvertToText(ctx context.Context, client *clients.ConvertClient, filename string, data []byte) ([]byte, error) {
	result, err := client.ConvertToText(ctx, filename, data)
	if err != nil {
		return nil, fmt.Errorf("convert to text %s: %w", filename, err)
	}
	return result, nil
}

// ConvertToImage calls convertsvc to convert an image to JPEG.
// Used by ImageExtractor during phase 1 for non-JPEG/PNG formats.
func ConvertToImage(ctx context.Context, client *clients.ConvertClient, filename string, data []byte) ([]byte, error) {
	result, err := client.ConvertToImage(ctx, filename, data)
	if err != nil {
		return nil, fmt.Errorf("convert to image %s: %w", filename, err)
	}
	return result, nil
}
