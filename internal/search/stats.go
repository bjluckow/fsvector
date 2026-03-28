package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Stats holds index-wide statistics.
type Stats struct {
	TotalFiles   int
	DeletedFiles int
	Duplicates   int
	TextFiles    int
	ImageFiles   int
	EmbedModel   string
}

// GetStats returns aggregate statistics about the index.
func GetStats(ctx context.Context, conn *pgx.Conn) (*Stats, error) {
	var s Stats
	err := conn.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE chunk_index = 0)                            AS total,
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL AND chunk_index = 0) AS deleted,
			COUNT(*) FILTER (WHERE canonical_path IS NOT NULL)                 AS duplicates,
			COUNT(*) FILTER (WHERE modality = 'text'   AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS text_files,
			COUNT(*) FILTER (WHERE modality = 'image'  AND chunk_index = 0
			                   AND deleted_at IS NULL)                         AS image_files,
			COALESCE(MAX(embed_model), 'none')                                 AS embed_model
		FROM files
	`).Scan(
		&s.TotalFiles, &s.DeletedFiles, &s.Duplicates,
		&s.TextFiles, &s.ImageFiles, &s.EmbedModel,
	)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	return &s, nil
}
