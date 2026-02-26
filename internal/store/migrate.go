package store

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/schema.sql
var schema string

// Migrate runs the schema SQL against the given connection.
// All statements are idempotent (IF NOT EXISTS), so this is safe to call on every startup.
func Migrate(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}
