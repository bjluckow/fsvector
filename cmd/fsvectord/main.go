package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bjluckow/fsvector/internal/clients/convert"
	"github.com/bjluckow/fsvector/internal/clients/embed"
	"github.com/bjluckow/fsvector/internal/clients/transcribe"
	"github.com/bjluckow/fsvector/internal/clients/vision"
	"github.com/bjluckow/fsvector/internal/config"
	"github.com/bjluckow/fsvector/internal/daemon"
	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("fsvectord starting")

	// ── connect to services ───────────────────────────────────
	embedClient := embed.NewClient(cfg.EmbedSvcURL)
	health, err := embedClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: embedsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  embed model: %s (dim=%d)\n", health.Model, health.Dim)

	convertClient := convert.NewClient(cfg.ConvertSvcURL)
	convertHealth, err := convertClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: convertsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  convert backends: %v\n", convertHealth.Backends)

	transcribeClient := transcribe.NewClient(cfg.TranscribeSvcURL)
	transcribeHealth, err := transcribeClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: transcribesvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  transcribe model: %s\n", transcribeHealth.Model)

	visionClient := vision.NewClient(cfg.VisionSvcURL)
	visionHealth, err := visionClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: visionsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  vision model : %s (ocr=%v)\n", visionHealth.CaptionModel, visionHealth.OCR)

	// ── connect to postgres ───────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: db connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// ── check for dimension mismatch ──────────────────────────────────────────
	existingDim, err := store.EmbeddingDim(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: dim check: %v\n", err)
		os.Exit(1)
	}
	if existingDim != 0 && existingDim != health.Dim {
		fmt.Fprintf(os.Stderr,
			"fsvectord: embedding dimension mismatch\n  database has vector(%d), embedsvc returns dim=%d\n  to re-index: docker compose down -v && docker compose up\n",
			existingDim, health.Dim,
		)
		os.Exit(1)
	}

	// ── migrate ───────────────────────────────────────────────────────────────
	if err := store.Migrate(ctx, pool, health.Dim); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  schema ok")

	// ── load and reconcile data source─────────────────────────────────────────

	src := source.Source(source.NewLocalSource(cfg.WatchPath))

	pCfg := pipeline.Config{
		Reader:           src.Reader(),
		EmbedClient:      embedClient,
		ConvertClient:    convertClient,
		TranscribeClient: transcribeClient,
		VisionClient:     visionClient,
		EmbedModel:       health.Model,
		Source:           cfg.Source,
		MinEmbedSize:     cfg.MinEmbedSize,
		ChunkSize:        cfg.ChunkSize,
		ChunkOverlap:     cfg.ChunkOverlap,
		MinChunkSize:     cfg.MinChunkSize,
		VideoFrameRate:   cfg.VideoFrameRate,
	}

	// ── run ───────────────────────────────────────────────────────────────────
	d := daemon.New(pool, src, pCfg, cfg.DaemonPort)
	if err := d.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: %v\n", err)
		os.Exit(1)
	}
}
