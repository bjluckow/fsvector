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
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/fswatch"
	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/jackc/pgx/v5"
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
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: db connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	// ── check for dimension mismatch ──────────────────────────────────────────
	existingDim, err := store.EmbeddingDim(ctx, conn)
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
	if err := store.Migrate(ctx, conn, health.Dim); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: migrate: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  schema ok")

	// ── reconcile ─────────────────────────────────────────────────────────────
	pCfg := pipeline.Config{
		Reader:           &source.LocalReader{},
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

	if err := reconcile(ctx, conn, pCfg, cfg.WatchPath); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: reconcile: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("fsvectord ready — watching for changes")

	events := make(chan fswatch.Event, 64)

	go func() {
		if err := fswatch.Watch(ctx, cfg.WatchPath, events); err != nil {
			fmt.Fprintf(os.Stderr, "watcher: %v\n", err)
			os.Exit(1)
		}
	}()

	handleEvents(ctx, conn, pCfg, events)
}

// reconcile diffs the filesystem against the database and brings the DB
// into sync. It runs once on startup before the fsnotify watcher takes over.
func reconcile(ctx context.Context, conn *pgx.Conn, pCfg pipeline.Config, watchPath string) error {
	fmt.Printf("  reconciling %s\n", watchPath)

	// 1. walk the filesystem
	fsFiles, err := fsindex.Walk(watchPath)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	// build a map for easy lookup
	fsMap := make(map[string]fsindex.FileInfo, len(fsFiles))
	for _, f := range fsFiles {
		fsMap[f.Path] = f
	}

	// 2. load live DB state
	dbFiles, err := store.LivePaths(ctx, conn)
	if err != nil {
		return fmt.Errorf("live paths: %w", err)
	}

	// 3. soft-delete files in DB that no longer exist on disk
	deleted := 0
	for path := range dbFiles {
		if _, exists := fsMap[path]; !exists {
			if err := store.SoftDelete(ctx, conn, path); err != nil {
				fmt.Fprintf(os.Stderr, "    soft-delete %s: %v\n", path, err)
				continue
			}
			fmt.Printf("    deleted %s\n", path)
			deleted++
		}
	}

	// 4. insert or re-embed files that are new or changed
	indexed := 0
	skipped := 0
	for _, fi := range fsFiles {
		existingHash, inDB := dbFiles[fi.Path]

		// unchanged — skip
		if inDB && existingHash == fi.Hash {
			skipped++
			continue
		}

		// check for duplicate content
		if canonicalPath, isDupe, err := store.FindByHash(ctx, conn, fi.Hash); err != nil {
			fmt.Fprintf(os.Stderr, "    hash check %s: %v\n", fi.Path, err)
			continue
		} else if isDupe && canonicalPath != fi.Path {
			f := store.File{
				Path:           fi.Path,
				Source:         pCfg.Source,
				ContentHash:    fi.Hash,
				Size:           fi.Size,
				MimeType:       fi.MimeType,
				Modality:       "text",
				FileName:       fi.Name,
				FileExt:        fi.Ext,
				FileCreatedAt:  &fi.CreatedAt,
				FileModifiedAt: &fi.ModifiedAt,
				EmbedModel:     pCfg.EmbedModel,
			}
			if err := store.UpsertDuplicate(ctx, conn, f, canonicalPath); err != nil {
				fmt.Fprintf(os.Stderr, "    dupe upsert %s: %v\n", fi.Path, err)
			} else {
				fmt.Printf("    duplicate %s -> %s\n", fi.Path, canonicalPath)
				indexed++
			}
			continue
		}

		result, err := pipeline.Process(ctx, pCfg, fi)
		if err != nil {
			fmt.Fprintf(os.Stderr, "    pipeline %s: %v\n", fi.Path, err)
			continue
		}
		if result.Skipped {
			fmt.Printf("    skipped %s (%s)\n", fi.Path, result.SkipReason)
			skipped++
			continue
		}

		for _, f := range result.Files {
			if err := store.Upsert(ctx, conn, f); err != nil {
				fmt.Fprintf(os.Stderr, "    upsert %s chunk %d: %v\n", fi.Path, f.ChunkIndex, err)
				continue
			}
		}

		// clean up stale chunks from previous indexing
		if err := store.DeleteStaleChunks(ctx, conn, fi.Path, pCfg.EmbedModel, len(result.Files)); err != nil {
			fmt.Fprintf(os.Stderr, "    stale chunks %s: %v\n", fi.Path, err)
		}

		fmt.Printf("    indexed %s (%s, %d chunks)\n", fi.Path, result.Files[0].Modality, len(result.Files))
		indexed++
	}

	fmt.Printf("  reconcile done: %d indexed, %d deleted, %d unchanged\n",
		indexed, deleted, skipped)
	return nil
}

func handleEvents(ctx context.Context, conn *pgx.Conn, pCfg pipeline.Config, events <-chan fswatch.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			switch e.Kind {
			case fswatch.EventDelete:
				if err := store.SoftDelete(ctx, conn, e.Path); err != nil {
					fmt.Fprintf(os.Stderr, "delete %s: %v\n", e.Path, err)
				} else {
					fmt.Printf("  deleted %s\n", e.Path)
				}

			case fswatch.EventCreate, fswatch.EventUpdate:
				fi, err := fsindex.FileInfoFromPath(e.Path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  stat %s: %v\n", e.Path, err)
					continue
				}

				result, err := pipeline.Process(ctx, pCfg, fi)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  pipeline %s: %v\n", e.Path, err)
					continue
				}
				if result.Skipped {
					continue
				}

				for _, f := range result.Files {
					if err := store.Upsert(ctx, conn, f); err != nil {
						fmt.Fprintf(os.Stderr, "  upsert %s chunk %d: %v\n", e.Path, f.ChunkIndex, err)
						continue
					}
				}

				if err := store.DeleteStaleChunks(ctx, conn, e.Path, pCfg.EmbedModel, len(result.Files)); err != nil {
					fmt.Fprintf(os.Stderr, "  stale chunks %s: %v\n", e.Path, err)
				}

				fmt.Printf("  %s %s (%s, %d chunks)\n", e.Kind, e.Path, result.Files[0].Modality, len(result.Files))
			}
		}
	}
}
