package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RegisterModel inserts or updates a model in the registry.
// If the model already exists, updates dim and updated_at.
func RegisterModel(ctx context.Context, conn *pgx.Conn, modality, modelName string, dim int) error {
	_, err := conn.Exec(ctx, `
		INSERT INTO models (modality, model_name, dim, status, is_active)
		VALUES ($1, $2, $3, 'ready', true)
		ON CONFLICT (modality, model_name) DO UPDATE SET
			dim        = EXCLUDED.dim,
			is_active  = true,
			status     = 'ready',
			updated_at = now()
	`, modality, modelName, dim)
	if err != nil {
		return fmt.Errorf("register model %s/%s: %w", modality, modelName, err)
	}
	return nil
}

// ActiveModel returns the currently active model name and dim for a modality.
func ActiveModel(ctx context.Context, conn *pgx.Conn, modality string) (string, int, error) {
	var name string
	var dim int
	err := conn.QueryRow(ctx, `
		SELECT model_name, dim FROM models
		WHERE modality = $1 AND is_active = true
	`, modality).Scan(&name, &dim)
	if err == pgx.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("active model %s: %w", modality, err)
	}
	return name, dim, nil
}

// DeactivateModels sets is_active=false for all models of a modality except
// the given model name. Used during hot-swap.
func DeactivateModels(ctx context.Context, conn *pgx.Conn, modality, keepModel string) error {
	_, err := conn.Exec(ctx, `
		UPDATE models
		SET is_active  = false,
		    status     = 'retired',
		    updated_at = now()
		WHERE modality   = $1
		  AND model_name != $2
		  AND is_active  = true
	`, modality, keepModel)
	if err != nil {
		return fmt.Errorf("deactivate models %s: %w", modality, err)
	}
	return nil
}

// SetModelIndexedAt records when a full reindex completed for a model.
func SetModelIndexedAt(ctx context.Context, conn *pgx.Conn, modality, modelName string, t time.Time) error {
	_, err := conn.Exec(ctx, `
		UPDATE models SET indexed_at = $1, updated_at = now()
		WHERE modality = $2 AND model_name = $3
	`, t, modality, modelName)
	if err != nil {
		return fmt.Errorf("set indexed_at %s/%s: %w", modality, modelName, err)
	}
	return nil
}
