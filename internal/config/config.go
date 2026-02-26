package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration shared across binaries.
// Values are read from environment variables, with sane defaults where applicable.
type Config struct {
	// Database
	DatabaseURL string

	// Services
	EmbedSvcURL   string
	ConvertSvcURL string

	// Daemon
	WatchPath  string
	EmbedModel string
	Source     string // "local" or "s3://bucket/prefix"
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   env("DATABASE_URL", ""),
		EmbedSvcURL:   env("EMBEDSVC_URL", "http://embedsvc:8000"),
		ConvertSvcURL: env("CONVERTSVC_URL", "http://convertd:8001"),
		WatchPath:     env("WATCH_PATH", "/data/source"),
		EmbedModel:    env("EMBED_MODEL", "sentence-transformers/all-MiniLM-L6-v2"),
		Source:        env("SOURCE", "local"),
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

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
