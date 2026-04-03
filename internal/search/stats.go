package search

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/store"
)

// Stats holds index-wide statistics.
type Stats struct {
	TotalFiles   int
	DeletedFiles int
	Duplicates   int
	TextFiles    int
	ImageFiles   int
	AudioFiles   int
	VideoFiles   int
	EmbedModel   string
}

// GetStats returns aggregate statistics about the index.
func GetStats(ctx context.Context, db store.Querier) (*Stats, error) {
	var s Stats
	err := db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE chunk_index = 0)                            AS total,
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL AND chunk_index = 0) AS deleted,
			COUNT(*) FILTER (WHERE canonical_path IS NOT NULL)                 AS duplicates,
			COUNT(*) FILTER (WHERE modality = 'text'   AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS text_files,
			COUNT(*) FILTER (WHERE modality = 'image'  AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS image_files,
			COUNT(*) FILTER (WHERE modality = 'audio' AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS audio_files,
			COUNT(*) FILTER (WHERE modality = 'video' AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS video_files,
			COALESCE(MAX(embed_model), 'none')                                 AS embed_model
		FROM files
	`).Scan(
		&s.TotalFiles, &s.DeletedFiles, &s.Duplicates,
		&s.TextFiles, &s.ImageFiles, &s.AudioFiles, &s.EmbedModel,
	)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	return &s, nil
}
