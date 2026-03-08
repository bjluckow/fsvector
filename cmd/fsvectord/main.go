package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/config"
	"github.com/bjluckow/fsvector/internal/convert"
	"github.com/bjluckow/fsvector/internal/embed"
	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("fsvectord starting")

	// ── connect to embedsvc, get dimension ───────────────────────────────────
	embedClient := embed.NewClient(cfg.EmbedSvcURL)
	health, err := embedClient.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: embedsvc unreachable: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  embed model: %s (dim=%d)\n", health.Model, health.Dim)

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
	convertClient := convert.NewClient(cfg.ConvertSvcURL)
	pCfg := pipeline.Config{
		EmbedClient:   embedClient,
		ConvertClient: convertClient,
		EmbedModel:    health.Model,
		Source:        cfg.Source,
	}

	if err := reconcile(ctx, conn, pCfg, cfg.WatchPath); err != nil {
		fmt.Fprintf(os.Stderr, "fsvectord: reconcile: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("fsvectord ready")
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

		// run through the pipeline
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

		if err := store.Upsert(ctx, conn, result.File); err != nil {
			fmt.Fprintf(os.Stderr, "    upsert %s: %v\n", fi.Path, err)
			continue
		}

		fmt.Printf("    indexed %s (%s)\n", fi.Path, result.File.Modality)
		indexed++
	}

	fmt.Printf("  reconcile done: %d indexed, %d deleted, %d unchanged\n",
		indexed, deleted, skipped)
	return nil
}
