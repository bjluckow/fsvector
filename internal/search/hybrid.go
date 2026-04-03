package search

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/store"
	"github.com/pgvector/pgvector-go"
)

// searchHybrid combines cosine similarity with FTS.
// Default mode — best for general text queries.
func searchHybrid(ctx context.Context, db store.Querier, q SearchQuery) ([]SearchResult, error) {
	v := pgvector.NewVector(q.Vector)

	cfg := q.Config
	if cfg.FTSScale == 0 {
		cfg = DefaultSearchConfig
	}

	// $1=vector $2=limit $3=offset $4=query
	args := []any{v, q.Limit, q.Offset, q.Query}
	idx := 5

	where := `
		WHERE deleted_at IS NULL
		  AND (canonical_path IS NULL OR canonical_path = '')
		  AND embedding IS NOT NULL`

	where, args, idx = applyFilters(where, args, idx, q)

	ftsSubquery := `
		COALESCE((
			SELECT MAX(ts_rank(
				to_tsvector('english', COALESCE(c.text_content, '')),
				plainto_tsquery('english', $4)
			))
			FROM files c
			WHERE c.path = files.path
			  AND c.deleted_at IS NULL
		), 0)`

	scoreExpr := fmt.Sprintf(`
		LEAST(1.0,
			%f * (1 - (embedding <=> $1)) +
			%f * CASE
				WHEN %s = 0 THEN 0.0
				ELSE GREATEST(%f, LEAST(1.0, %s * %f))
			END
		)`,
		cfg.SemanticWeight(),
		cfg.FTSWeight,
		ftsSubquery,
		cfg.FTSMinBoost,
		ftsSubquery,
		cfg.FTSScale,
	)

	inner := fmt.Sprintf(`
		SELECT DISTINCT ON (path)
			path, modality, file_ext, size,
			%s AS score,
			indexed_at, file_modified_at
		FROM files
		%s
		ORDER BY path, embedding <=> $1`, scoreExpr, where)

	return runSearch(ctx, db, inner, args)
}
