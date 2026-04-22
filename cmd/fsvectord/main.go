package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/sync/errgroup"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/config"
	"github.com/bjluckow/fsvector/internal/indexer"
	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/server"
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
	httpClient := &http.Client{}

	embedClient := clients.NewEmbedClient(cfg.EmbedSvcURL, httpClient)
	health, err := embedClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: embedsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  embed model: %s (dim=%d)\n", health.Model, health.Dim)

	convertClient := clients.NewConvertClient(cfg.ConvertSvcURL, httpClient)
	convertHealth, err := convertClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: convertsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  convert backends: %v\n", convertHealth.Backends)

	transcribeClient := clients.NewTranscribeClient(cfg.TranscribeSvcURL, httpClient)
	transcribeHealth, err := transcribeClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: transcribesvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  transcribe model: %s\n", transcribeHealth.Model)

	visionClient := clients.NewVisionClient(cfg.VisionSvcURL, httpClient)
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

	// ── sources ───────────────────────────────────────────────────────────────

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
			PollInterval:       0,
		})
	default:
		src = source.NewLocalSource(cfg.WatchPath, true, 0)
	}

	fmt.Printf("  source       : %s\n", src.URI())

	// ── work channel ─────────────────────────────────────────────────────────
	work := make(chan model.Item, 256)

	// ── indexer config ────────────────────────────────────────────────────────
	progress := &indexer.Progress{}
	trigger := make(chan indexer.Trigger, 1)

	idx := indexer.New(indexer.Config{
		ConvertClient:   convertClient,
		ChunkSize:       cfg.ChunkSize,
		ChunkOverlap:    cfg.ChunkOverlap,
		MinChunkSize:    cfg.MinChunkSize,
		VideoFrameRate:  cfg.VideoFrameRate,
		DownloadWorkers: 8,
	}, src, work, progress)

	// ── pipeline config ───────────────────────────────────────────────────────
	pl := pipeline.New(pipeline.Config{
		EmbedClient:         embedClient,
		VisionClient:        visionClient,
		TranscribeClient:    transcribeClient,
		EmbedModel:          health.Model,
		EnableCaption:       cfg.EnableCaption,
		EnableOCR:           cfg.EnableOCR,
		EnableTranscribe:    cfg.EnableTranscribe,
		EmbedOCRText:        true,
		EmbedTranscriptText: true,
		EmbedCaptionText:    false,
	}, work)

	// ── start HTTP server ─────────────────────────────────────────────────────
	srv := server.New(embedClient, progress, trigger, src.URI())
	go srv.Serve(ctx, cfg.DaemonPort)

	// ── start indexer + pipeline ──────────────────────────────────────────────
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer idx.Close()
		return idx.Run(ctx, trigger)
	})
	g.Go(func() error {
		return pl.Start(ctx)
	})
	g.Wait()
}
