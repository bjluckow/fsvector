package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

// Init initializes the package-level connection pool.
// Must be called once before any store functions are used.
func Init(ctx context.Context, databaseURL string) error {
	p, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("store init: %w", err)
	}
	pool = p
	return nil
}

// Close closes the connection pool.
func Close() {
	if pool != nil {
		pool.Close()
	}
}

// Pool returns the package-level pool for cases where direct access is needed.
func Pool() *pgxpool.Pool {
	return pool
}
