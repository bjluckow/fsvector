package config

import (
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/pkg/parse"
)

// Config holds all runtime configuration shared across binaries.
// Values are read from environment variables, with sane defaults where applicable.
type Config struct {
	// Database
	DatabaseURL string

	// Services
	TextEmbedSvcURL  string
	ImageEmbedSvcURL string
	ConvertSvcURL    string

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
		DatabaseURL:      env("DATABASE_URL", ""),
		TextEmbedSvcURL:  env("TEXT_EMBEDSVC_URL", "http://embedsvc-text:8000"),
		ImageEmbedSvcURL: env("IMAGE_EMBEDSVC_URL", "http://embedsvc-image:8000"),
		ConvertSvcURL:    env("CONVERTSVC_URL", "http://convertsvc:8001"),
		WatchPath:        env("WATCH_PATH", "/data/source"),
		EmbedModel:       env("EMBED_MODEL", "sentence-transformers/all-MiniLM-L6-v2"),
		Source:           env("SOURCE", "local"),
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
