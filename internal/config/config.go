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
	EmbedSvcURL      string
	ConvertSvcURL    string
	TranscribeSvcURL string
	VisionSvcURL     string

	// Daemon
	WatchPath    string
	EmbedModel   string
	Source       string // "local" or "s3://bucket/prefix"
	MinEmbedSize int64

	// Processing
	VideoFrameRate float64
}

// Load reads configuration from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:      env("DATABASE_URL", ""),
		ChunkSize:        envInt("CHUNK_SIZE", 1000),
		ChunkOverlap:     envInt("CHUNK_OVERLAP", 100),
		MinChunkSize:     envInt("MIN_CHUNK_SIZE", 10), // TODO: may not need this with hybrid search
		EmbedSvcURL:      env("EMBEDSVC_URL", "http://embedsvc:8000"),
		ConvertSvcURL:    env("CONVERTSVC_URL", "http://convertd:8001"),
		TranscribeSvcURL: env("TRANSCRIBESVC_URL", "http://transcribesvc:8002"),
		VisionSvcURL:     env("VISIONSVC_URL", "http://visionsvc:8003"),
		WatchPath:        env("WATCH_PATH", "/data/source"),
		EmbedModel:       env("EMBED_MODEL", "sentence-transformers/all-MiniLM-L6-v2"),
		Source:           env("SOURCE", "local"),
		VideoFrameRate:   envFloat("VIDEO_FRAME_RATE", 1.0),
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

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
