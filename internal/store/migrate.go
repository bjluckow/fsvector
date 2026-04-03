package store

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/schema.sql
var schemaTmpl string

// Migrate applies the schema to the database, substituting the embedding
// dimension. It is idempotent — safe to call on every startup.
func Migrate(ctx context.Context, db Querier, dim int) error {
	schema := strings.ReplaceAll(schemaTmpl, "%%EMBEDDING_DIM%%", fmt.Sprintf("%d", dim))
	_, err := db.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// EmbeddingDim returns the current vector dimension of the files table,
// or 0 if the table does not exist yet.
func EmbeddingDim(ctx context.Context, db Querier) (int, error) {
	var dim int
	err := db.QueryRow(ctx, `
		SELECT atttypmod
		FROM pg_attribute
		WHERE attrelid = 'files'::regclass
		  AND attname = 'embedding'
	`).Scan(&dim)

	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		// table doesn't exist yet
		return 0, nil
	}
	return dim, nil
}
