package store

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed schema.sql
var schemaTmpl string

// Migrate applies the schema to the database, substituting the embedding
// dimension. It is idempotent — safe to call on every startup.
func Migrate(ctx context.Context, dim int) error {
	schema := strings.ReplaceAll(schemaTmpl, "%%EMBEDDING_DIM%%", fmt.Sprintf("%d", dim))
	_, err := pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
