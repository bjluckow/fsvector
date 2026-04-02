package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/bjluckow/fsvector/pkg/parse"
)

// Config holds all runtime configuration shared across binaries.
// Values are read from environment variables, with sane defaults where applicable.
type Config struct {
	// Database
	DatabaseURL  string
	ChunkSize    int
	ChunkOverlap int
	MinChunkSize int

	// Services
	EmbedSvcURL   string
	ConvertSvcURL string

	// Daemon
	WatchPath    string
	EmbedModel   string
	Source       string // "local" or "s3://bucket/prefix"
	MinEmbedSize int64
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   env("DATABASE_URL", ""),
		ChunkSize:     envInt("CHUNK_SIZE", 1000),
		ChunkOverlap:  envInt("CHUNK_OVERLAP", 100),
		MinChunkSize:  envInt("MIN_CHUNK_SIZE", 100),
		EmbedSvcURL:   env("EMBEDSVC_URL", "http://embedsvc:8000"),
		ConvertSvcURL: env("CONVERTSVC_URL", "http://convertd:8001"),
		WatchPath:     env("WATCH_PATH", "/data/source"),
		EmbedModel:    env("EMBED_MODEL", "sentence-transformers/all-MiniLM-L6-v2"),
		Source:        env("SOURCE", "local"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	minEmbedSizeStr := env("MIN_EMBED_SIZE", "100")
	minEmbedSize, err := parse.Size(minEmbedSizeStr)
	if err != nil {
		return nil, fmt.Errorf("MIN_EMBED_SIZE: %w", err)
	}
	c.MinEmbedSize = minEmbedSize

	return c, nil
}

// env returns the value of the named environment variable,
// or the provided fallback if the variable is unset or empty.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envInt returns the integer value of the named environment variable,
// or the provided fallback if the variable is unset or empty.
func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
