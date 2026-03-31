package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// RetireByModality soft-retires all active embeddings for a given modality.
// Used during hot-swap to mark old model rows as superseded.
// Never touches rows where deleted_at IS NOT NULL.
func RetireByModality(ctx context.Context, conn *pgx.Conn, modality string) error {
	_, err := conn.Exec(ctx, `
		UPDATE files
		SET retired_at = now()
		WHERE modality = $1
		  AND retired_at IS NULL
		  AND deleted_at IS NULL
	`, modality)
	if err != nil {
		return fmt.Errorf("retire by modality %s: %w", modality, err)
	}
	return nil
}

// UnRetire clears retired_at for a specific file + model combination.
// Used during swap-back when a previous model's cached rows are reactivated.
func UnRetire(ctx context.Context, conn *pgx.Conn, path string, embedModel string) error {
	_, err := conn.Exec(ctx, `
		UPDATE files
		SET retired_at = NULL
		WHERE path = $1
		  AND embed_model = $2
		  AND deleted_at IS NULL
	`, path, embedModel)
	if err != nil {
		return fmt.Errorf("unretire %s: %w", path, err)
	}
	return nil
}
