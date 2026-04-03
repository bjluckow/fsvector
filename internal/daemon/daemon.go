package daemon

import (
	"context"
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Daemon struct {
	pool     *pgxpool.Pool
	src      source.Source
	pCfg     pipeline.Config
	progress *Progress
	trigger  chan struct{}
	port     int
}

func New(pool *pgxpool.Pool, src source.Source, pCfg pipeline.Config, port int) *Daemon {
	return &Daemon{
		pool:     pool,
		src:      src,
		pCfg:     pCfg,
		progress: &Progress{},
		trigger:  make(chan struct{}, 1),
		port:     port,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	// start HTTP server
	srv := newServer(d.progress, d.trigger, d.src.URI())
	go srv.Serve(ctx, d.port)

	// initial reconcile
	if err := Reindex(ctx, d.pool, d.pCfg, d.src, d.progress); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	// handle reconcile triggers from HTTP server
	go d.listenForTriggers(ctx)

	// watch if supported
	if w, ok := d.src.(source.Watcher); ok {
		events := make(chan source.Event, 64)
		go func() {
			if err := w.Watch(ctx, events); err != nil {
				fmt.Fprintf(os.Stderr, "watcher: %v\n", err)
			}
		}()
		d.handleEvents(ctx, events)
	} else {
		fmt.Println("  source does not support watching — use fsvector reconcile")
		<-ctx.Done()
	}

	return nil
}

func (d *Daemon) listenForTriggers(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.trigger:
			if d.progress.Running {
				continue
			}
			if err := Reindex(ctx, d.pool, d.pCfg, d.src, d.progress); err != nil {
				fmt.Fprintf(os.Stderr, "triggered reconcile: %v\n", err)
			}
		}
	}
}

func (d *Daemon) handleEvents(ctx context.Context, events <-chan source.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			switch e.Kind {
			case source.EventDelete:
				if err := store.SoftDelete(ctx, d.pool, e.Path); err != nil {
					fmt.Fprintf(os.Stderr, "delete %s: %v\n", e.Path, err)
				} else {
					fmt.Printf("  deleted %s\n", e.Path)
				}

			case source.EventCreate, source.EventUpdate:
				fi, err := source.FileInfoFromPath(e.Path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  stat %s: %v\n", e.Path, err)
					continue
				}

				result, err := pipeline.Process(ctx, d.pCfg, fi)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  pipeline %s: %v\n", e.Path, err)
					continue
				}
				if result.Skipped {
					continue
				}

				for _, f := range result.Files {
					if err := store.Upsert(ctx, d.pool, f); err != nil {
						fmt.Fprintf(os.Stderr, "  upsert %s chunk %d: %v\n", e.Path, f.ChunkIndex, err)
						continue
					}
				}
				if err := store.DeleteStaleChunks(ctx, d.pool, e.Path, d.pCfg.EmbedModel, len(result.Files)); err != nil {
					fmt.Fprintf(os.Stderr, "  stale chunks %s: %v\n", e.Path, err)
				}
				fmt.Printf("  %s %s (%s, %d chunks)\n", e.Kind, e.Path, result.Files[0].Modality, len(result.Files))
			}
		}
	}
}
