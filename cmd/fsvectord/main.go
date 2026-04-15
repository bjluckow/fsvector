package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

	if err := store.Init(ctx, cfg.DatabaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: db: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// ── check for dimension mismatch ──────────────────────────────────────────
	existingDim, err := store.GetEmbeddingDim(ctx)
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
	if err := store.Migrate(ctx, health.Dim); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  schema ok")

	// ── load and reconcile data source─────────────────────────────────────────

	var src source.Source
	switch cfg.SourceType {
	case "s3":
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(cfg.S3Region),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fsvectord: aws config: %v\n", err)
			os.Exit(1)
		}
		src = source.NewS3Source(source.S3Config{
			Client:             s3.NewFromConfig(awsCfg),
			Bucket:             cfg.S3Bucket,
			Prefix:             cfg.S3Prefix,
			LargeFileThreshold: cfg.LargeFileThreshold,
		})
		fmt.Printf("  source       : %s\n", src.URI())
	default:
		src = source.NewLocalSource(cfg.WatchPath)
		fmt.Printf("  source       : %s\n", cfg.WatchPath)
	}

	pCfg := pipeline.Config{
		Reader:           src.Reader(),
		EmbedClient:      embedClient,
		ConvertClient:    convertClient,
		TranscribeClient: transcribeClient,
		VisionClient:     visionClient,
		EmbedModel:       health.Model,
		Source:           cfg.SourceType,
		MinEmbedSize:     cfg.MinEmbedSize,
		ChunkSize:        cfg.ChunkSize,
		ChunkOverlap:     cfg.ChunkOverlap,
		MinChunkSize:     cfg.MinChunkSize,
		VideoFrameRate:   cfg.VideoFrameRate,
	}

	// ── run ───────────────────────────────────────────────────────────────────
	d := daemon.New(pool, src, pCfg, embedClient, cfg.DaemonPort)
	if err := d.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: %v\n", err)
		os.Exit(1)
	}
}
