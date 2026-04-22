// internal/pipeline/pipeline.go

package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sourcegraph/conc/pool"
	"golang.org/x/sync/errgroup"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/source"
)

// Config holds all pipeline configuration.
type Config struct {
	EmbedClient      *clients.EmbedClient
	VisionClient     *clients.VisionClient
	TranscribeClient *clients.TranscribeClient
	ConvertClient    *clients.ConvertClient
	Reader           source.FileReader

	EmbedModel string

	EmbedBatchSize     int
	CaptionBatchSize   int
	TextEmbedBatchSize int
	UpsertBatchSize    int

	DownloadWorkers   int
	OCRWorkers        int
	TranscribeWorkers int

	FlushTimeout time.Duration

	ChunkSize    int
	ChunkOverlap int
	MinChunkSize int

	VideoFrameRate float64

	EmbedOCRText        bool
	EmbedCaptionText    bool
	EmbedTranscriptText bool
	// Feature flags — toggle processing stages
	EnableCaption    bool // BLIP captioning (slow on CPU)
	EnableOCR        bool // tesseract OCR
	EnableTranscribe bool // whisper transcription
}

func (c *Config) withDefaults() {
	if c.EmbedBatchSize == 0 {
		c.EmbedBatchSize = 32
	}
	if c.CaptionBatchSize == 0 {
		c.CaptionBatchSize = 4
	}
	if c.TextEmbedBatchSize == 0 {
		c.TextEmbedBatchSize = 64
	}
	if c.UpsertBatchSize == 0 {
		c.UpsertBatchSize = 50
	}
	if c.DownloadWorkers == 0 {
		c.DownloadWorkers = 8
	}
	if c.OCRWorkers == 0 {
		c.OCRWorkers = 4
	}
	if c.TranscribeWorkers == 0 {
		c.TranscribeWorkers = 2
	}
	if c.FlushTimeout == 0 {
		c.FlushTimeout = 3 * time.Second
	}
	if c.VideoFrameRate == 0 {
		c.VideoFrameRate = 1.0
	}
}

// ProgressFunc is called by the pipeline to report file-level progress.
type ProgressFunc func(path string, status string) // status: "indexed", "skipped", "error"

// Pipeline orchestrates the two-phase index+process architecture.
type Pipeline struct {
	cfg        Config
	extractors map[ModalityType]Extractor
	enabled    map[Stage]bool
}

// New creates a Pipeline with extractors wired for each modality.
func New(cfg Config) *Pipeline {
	cfg.withDefaults()

	enabled := map[Stage]bool{
		StageClipEmbed:  true,
		StageCaption:    cfg.EnableCaption,
		StageOCR:        cfg.EnableOCR,
		StageTranscribe: cfg.EnableTranscribe,
		StageTextEmbed:  true,
		StageUpsert:     true,
	}

	return &Pipeline{
		cfg:     cfg,
		enabled: enabled,
		extractors: map[ModalityType]Extractor{
			ModalityImage: &ImageExtractor{
				Reader: cfg.Reader, Convert: cfg.ConvertClient,
			},
			ModalityText: &TextExtractor{
				Reader: cfg.Reader, Convert: cfg.ConvertClient,
				ChunkSize: cfg.ChunkSize, ChunkOverlap: cfg.ChunkOverlap,
				MinChunkSize: cfg.MinChunkSize,
			},
			ModalityAudio: &AudioExtractor{
				Reader: cfg.Reader, Convert: cfg.ConvertClient,
			},
			ModalityVideo: &VideoExtractor{
				Reader: cfg.Reader, Convert: cfg.ConvertClient,
				FrameRate: cfg.VideoFrameRate,
			},
			ModalityEmail: &EmailExtractor{
				Reader: cfg.Reader, Convert: cfg.ConvertClient,
				ChunkSize: cfg.ChunkSize, ChunkOverlap: cfg.ChunkOverlap,
				MinChunkSize: cfg.MinChunkSize,
			},
		},
	}
}

// RunBatch processes files through both phases:
// Phase 1 (index): extract, convert, create DB rows, enqueue work items.
// Phase 2 (process): workers consume queues, call services, write chunks.
func (p *Pipeline) RunBatch(ctx context.Context, files []source.FileInfo, onProgress ProgressFunc) error {
	r := newRouter(ctx,
		map[Stage]chan *WorkItem{
			StageClipEmbed:  make(chan *WorkItem, p.cfg.EmbedBatchSize*2),
			StageCaption:    make(chan *WorkItem, p.cfg.CaptionBatchSize*2),
			StageOCR:        make(chan *WorkItem, p.cfg.OCRWorkers*2),
			StageTranscribe: make(chan *WorkItem, p.cfg.TranscribeWorkers*2),
			StageTextEmbed:  make(chan *WorkItem, p.cfg.TextEmbedBatchSize*2),
			StageUpsert:     make(chan *WorkItem, p.cfg.UpsertBatchSize*2),
		},
		p.enabled,
	)

	// --- Start workers in tiers (phase 2) ---

	// Tier 1: primary workers (consume from extraction)
	tier1, t1ctx := errgroup.WithContext(ctx)
	tier1.Go(func() error { p.newClipEmbedWorker(r).Run(t1ctx); return nil })
	tier1.Go(func() error { p.newCaptionWorker(r).Run(t1ctx); return nil })
	tier1.Go(func() error { p.newOCRWorker(r).Run(t1ctx); return nil })
	tier1.Go(func() error { p.newTranscribeWorker(r).Run(t1ctx); return nil })

	// Tier 2: text embed (consumes from tier 1 outputs)
	tier2, t2ctx := errgroup.WithContext(ctx)
	tier2.Go(func() error { p.newTextEmbedWorker(r).Run(t2ctx); return nil })

	// Tier 3: upsert (consumes from all tiers)
	tier3, t3ctx := errgroup.WithContext(ctx)
	tier3.Go(func() error { p.newUpsertWorker(r).Run(t3ctx); return nil })

	// --- Phase 1: extraction ---

	groups := groupByModality(files)
	var extracted atomic.Int32
	total := len(files)

	for _, mod := range ModalityOrder {
		group := groups[mod]
		if len(group) == 0 {
			continue
		}

		extractor := p.extractors[mod]
		if extractor == nil {
			continue
		}

		fmt.Printf("  extracting %d %s files...\n", len(group), mod)

		ep := pool.New().WithMaxGoroutines(p.cfg.DownloadWorkers)
		for _, fi := range group {
			fi := fi
			ep.Go(func() {
				items, err := extractor.Extract(ctx, fi)
				n := extracted.Add(1)

				if err != nil {
					fmt.Printf("    extract %s: %v\n", fi.Path, err)
					if onProgress != nil {
						onProgress(fi.Path, "error")
					}
				} else if len(items) == 0 {
					if onProgress != nil {
						onProgress(fi.Path, "skipped")
					}
				} else {
					r.routeMany(items)
					if onProgress != nil {
						onProgress(fi.Path, "indexed")
					}
				}

				if n%100 == 0 || int(n) == total {
					fmt.Printf("  [%d/%d] extraction progress\n", n, total)
				}
			})
		}
		ep.Wait()

	}

	// --- Cascade close ---

	// Tier 1 inputs done
	r.closeCh(StageClipEmbed)
	r.closeCh(StageCaption)
	r.closeCh(StageOCR)
	r.closeCh(StageTranscribe)
	if err := tier1.Wait(); err != nil {
		return fmt.Errorf("tier1: %w", err)
	}

	// Tier 1 finished — no more text_embed items will be produced
	r.closeCh(StageTextEmbed)
	if err := tier2.Wait(); err != nil {
		return fmt.Errorf("tier2: %w", err)
	}

	// Tier 2 finished — no more upsert items will be produced
	r.closeCh(StageUpsert)
	if err := tier3.Wait(); err != nil {
		return fmt.Errorf("tier3: %w", err)
	}

	return nil
}

// routeToUpsert creates an upsert-stage WorkItem and routes it.
func (p *Pipeline) routeToUpsert(r *router, item *WorkItem, chunkType string) {
	item.FileData.AddRef()
	r.route(&WorkItem{
		FileData:  item.FileData,
		Modality:  item.Modality,
		Stage:     StageUpsert,
		ItemType:  chunkType,
		ItemID:    item.ItemID,
		ItemIndex: item.ItemIndex,
		Text:      item.Text,
		Embedding: item.Embedding,
	})
}

// releaseBatch releases FileData refs for all items in a batch.
func (p *Pipeline) releaseBatch(batch []*WorkItem) {
	for _, item := range batch {
		item.FileData.Release()
	}
}

func groupByModality(files []source.FileInfo) map[ModalityType][]source.FileInfo {
	groups := make(map[ModalityType][]source.FileInfo)
	for _, f := range files {
		mod, ok := Modality(f.Ext)
		if !ok {
			continue
		}
		groups[ModalityType(mod)] = append(groups[ModalityType(mod)], f)
	}
	return groups
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
