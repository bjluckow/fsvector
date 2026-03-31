package store

import (
	"context"
	"fmt"
	"strings"

	_ "embed"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/schema.sql
var schemaTmpl string

// Migrate applies all pending schema migrations in order.
// Safe to call on every startup — already-applied migrations are skipped.
func Migrate(ctx context.Context, conn *pgx.Conn, textDim, imageDim int) error {
	if err := ensureVersionTable(ctx, conn); err != nil {
		return err
	}

	version, err := currentVersion(ctx, conn)
	if err != nil {
		return err
	}

	migrations := []struct {
		version int
		fn      func(context.Context, *pgx.Conn, int, int) error
	}{
		{1, migrateV1},
		{2, migrateV2},
	}

	for _, m := range migrations {
		if version >= m.version {
			continue
		}
		fmt.Printf("  applying schema migration v%d\n", m.version)
		if err := m.fn(ctx, conn, textDim, imageDim); err != nil {
			return fmt.Errorf("migration v%d: %w", m.version, err)
		}
		if err := recordVersion(ctx, conn, m.version); err != nil {
			return err
		}
	}
	return nil
}

func ensureVersionTable(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version     INT NOT NULL,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func currentVersion(ctx context.Context, conn *pgx.Conn) (int, error) {
	var version int
	err := conn.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM schema_version
	`).Scan(&version)
	return version, err
}

func recordVersion(ctx context.Context, conn *pgx.Conn, version int) error {
	_, err := conn.Exec(ctx,
		`INSERT INTO schema_version (version) VALUES ($1)`, version)
	return err
}

// migrateV1 applies the initial schema.
func migrateV1(ctx context.Context, conn *pgx.Conn, textDim, imageDim int) error {
	schema := strings.ReplaceAll(schemaTmpl, "%%TEXT_DIM%%", fmt.Sprintf("%d", textDim))
	schema = strings.ReplaceAll(schema, "%%IMAGE_DIM%%", fmt.Sprintf("%d", imageDim))
	_, err := conn.Exec(ctx, schema)
	return err
}

// migrateV2 applies v1.2 changes:
// - adds retired_at column to files
// - replaces (path, chunk_index) unique index with (path, chunk_index, embed_model)
// - creates models table and its unique index
func migrateV2(ctx context.Context, conn *pgx.Conn, _, _ int) error {
	_, err := conn.Exec(ctx, `
        ALTER TABLE files ADD COLUMN IF NOT EXISTS retired_at TIMESTAMPTZ;
        DROP INDEX IF EXISTS files_path_chunk_idx;
        CREATE UNIQUE INDEX IF NOT EXISTS files_path_chunk_model_idx
            ON files (path, chunk_index, embed_model);
        CREATE TABLE IF NOT EXISTS models (
            modality    TEXT NOT NULL,
            model_name  TEXT NOT NULL,
            dim         INT NOT NULL,
            status      TEXT NOT NULL DEFAULT 'ready',
            is_active   BOOLEAN NOT NULL DEFAULT false,
            indexed_at  TIMESTAMPTZ,
            created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
            updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
            PRIMARY KEY (modality, model_name)
        );
        CREATE UNIQUE INDEX IF NOT EXISTS models_active_modality_idx
            ON models (modality)
            WHERE is_active = true;
    `)
	return err
}
