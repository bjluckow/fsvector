package store

import (
	"context"
	"fmt"
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
func GetStats(ctx context.Context) (*Stats, error) {
	var s Stats
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*)                                                        AS total,
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL)                  AS deleted,
			COUNT(*) FILTER (WHERE canonical_path IS NOT NULL)              AS duplicates,
			COUNT(*) FILTER (WHERE modality = 'text'  AND deleted_at IS NULL) AS text_files,
			COUNT(*) FILTER (WHERE modality = 'image' AND deleted_at IS NULL) AS image_files,
			COUNT(*) FILTER (WHERE modality = 'audio' AND deleted_at IS NULL) AS audio_files,
			COUNT(*) FILTER (WHERE modality = 'video' AND deleted_at IS NULL) AS video_files,
			COALESCE((SELECT MAX(embed_model) FROM chunks), 'none')         AS embed_model
		FROM files
	`).Scan(
		&s.TotalFiles, &s.DeletedFiles, &s.Duplicates,
		&s.TextFiles, &s.ImageFiles, &s.AudioFiles, &s.VideoFiles,
		&s.EmbedModel,
	)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	return &s, nil
}
