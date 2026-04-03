package search

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/store"
	"github.com/pgvector/pgvector-go"
)

// searchVector performs pure cosine similarity search.
// No FTS component — fastest mode, best for image search and
// queries where keyword matching is not needed.
func searchVector(ctx context.Context, db store.Querier, q SearchQuery) ([]SearchResult, error) {
	v := pgvector.NewVector(q.Vector)

	// $1=vector $2=limit $3=offset
	args := []any{v, q.Limit, q.Offset}
	idx := 4

	where := `
		WHERE deleted_at IS NULL
		  AND (canonical_path IS NULL OR canonical_path = '')
		  AND embedding IS NOT NULL`

	where, args, idx = applyFilters(where, args, idx, q)

	inner := fmt.Sprintf(`
		SELECT DISTINCT ON (path)
			path, modality, file_ext, size,
			1 - (embedding <=> $1) AS score,
			indexed_at, file_modified_at
		FROM files
		%s
		ORDER BY path, embedding <=> $1`, where)

	return runSearch(ctx, db, inner, args)
}
