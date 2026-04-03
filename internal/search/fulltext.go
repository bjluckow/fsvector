package search

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/store"
)

// searchFullText performs FTS-only search.
// No vector component — useful for exact keyword searches.
// Does not require an embedding vector.
func searchFullText(ctx context.Context, db store.Querier, q SearchQuery) ([]SearchResult, error) {
	if q.Query == "" {
		return nil, fmt.Errorf("fulltext search requires a query string")
	}

	cfg := q.Config
	if cfg.FTSScale == 0 {
		cfg = DefaultSearchConfig
	}

	// $1=query $2=limit $3=offset
	args := []any{q.Query, q.Limit, q.Offset}
	idx := 4

	where := `
		WHERE deleted_at IS NULL
		  AND (canonical_path IS NULL OR canonical_path = '')
		  AND text_content IS NOT NULL`

	where, args, idx = applyFilters(where, args, idx, q)

	inner := fmt.Sprintf(`
		SELECT DISTINCT ON (path)
			path, modality, file_ext, size,
			LEAST(1.0, COALESCE((
				SELECT MAX(ts_rank(
					to_tsvector('english', COALESCE(c.text_content, '')),
					plainto_tsquery('english', $1)
				)) * %f
				FROM files c
				WHERE c.path = files.path
				  AND c.deleted_at IS NULL
			), 0)) AS score,
			indexed_at, file_modified_at
		FROM files
		%s
		ORDER BY path, text_content IS NULL, ts_rank(
			to_tsvector('english', COALESCE(text_content, '')),
			plainto_tsquery('english', $1)
		) DESC`, cfg.FTSScale, where)

	return runSearch(ctx, db, inner, args)
}
