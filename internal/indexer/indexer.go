package indexer

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

type Trigger struct {
	Purge bool // hard-delete soft-deleted files after reindex
}

type Config struct {
	ConvertClient   *clients.ConvertClient
	ChunkSize       int
	ChunkOverlap    int
	MinChunkSize    int
	VideoFrameRate  float64
	DownloadWorkers int
}

type Indexer struct {
	cfg      Config
	source   source.Source
	reader   source.FileReader
	output   chan<- model.Item
	progress *Progress
}

func (c *Config) withDefaults() {
	if c.DownloadWorkers == 0 {
		c.DownloadWorkers = 8
	}
	if c.VideoFrameRate == 0 {
		c.VideoFrameRate = 1.0
	}
}

func New(cfg Config, src source.Source, output chan<- model.Item, progress *Progress) *Indexer {
	cfg.withDefaults()
	return &Indexer{
		cfg:      cfg,
		source:   src,
		reader:   src.Reader(),
		output:   output,
		progress: progress,
	}
}

// Close closes the output channel, signaling to the pipeline
// that no more items will be sent.
func (idx *Indexer) Close() {
	close(idx.output)
}

func (idx *Indexer) Run(ctx context.Context, trigger <-chan Trigger) error {
	// initial reindex
	if err := idx.Reindex(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reindex %s: %v\n", idx.source.URI(), err)
	}

	// start watcher if supported
	if w, ok := idx.source.(source.Watchable); ok {
		events := make(chan source.Event, 64)
		go func() {
			if err := w.Watch(ctx, events); err != nil {
				fmt.Fprintf(os.Stderr, "watch %s: %v\n", idx.source.URI(), err)
			}
		}()
		go idx.handleEvents(ctx, events)
	}

	// poll loop
	if idx.source.PollInterval() > 0 {
		ticker := time.NewTicker(idx.source.PollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				if !idx.progress.IsRunning() {
					if err := idx.Reindex(ctx); err != nil {
						fmt.Fprintf(os.Stderr, "poll reindex %s: %v\n", idx.source.URI(), err)
					}
				}
			case t := <-trigger:
				if !idx.progress.IsRunning() {
					idx.Reindex(ctx)
					if t.Purge {
						store.PurgeSoftDeleted(ctx)
					}
				}
			}
		}
	}

	<-ctx.Done()
	return nil
}
