package search

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/clients/embed"
	"github.com/bjluckow/fsvector/internal/store"
)

// SearchByImage embeds an image and performs vector search.
// Convenience wrapper used by both the daemon HTTP handler
// and any future callers that have raw image bytes.
func SearchByImage(
	ctx context.Context,
	db store.Querier,
	embedClient *embed.Client,
	filename string,
	data []byte,
	q SearchQuery,
) ([]SearchResult, error) {
	embedding, err := embedClient.EmbedImage(ctx, filename, data)
	if err != nil {
		return nil, fmt.Errorf("embed image: %w", err)
	}

	q.Vector = embedding
	q.Mode = SearchModeVector

	return Search(ctx, db, q)
}
