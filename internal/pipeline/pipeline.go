// internal/pipeline/pipeline.go

package pipeline

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/bjluckow/fsvector/internal/clients"
	"github.com/bjluckow/fsvector/internal/model"
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
	cfg     Config
	input   <-chan model.Item
	enabled map[Stage]bool
}

// New creates a Pipeline with extractors wired for each modality.
func New(cfg Config, input <-chan model.Item) *Pipeline {
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
		input:   input,
		enabled: enabled,
	}
}

// Start begins processing items from the input channel.
// Blocks until the input channel is closed and all workers drain.
// Call from a goroutine.
func (p *Pipeline) Start(ctx context.Context) error {
	r := newRouter(ctx,
		map[Stage]chan *job{
			StageClipEmbed:  make(chan *job, p.cfg.EmbedBatchSize*2),
			StageCaption:    make(chan *job, p.cfg.CaptionBatchSize*2),
			StageOCR:        make(chan *job, p.cfg.OCRWorkers*2),
			StageTranscribe: make(chan *job, p.cfg.TranscribeWorkers*2),
			StageTextEmbed:  make(chan *job, p.cfg.TextEmbedBatchSize*2),
			StageUpsert:     make(chan *job, p.cfg.UpsertBatchSize*2),
		},
		p.enabled,
	)

	var tier1Done sync.WaitGroup
	tier1Done.Add(4)

	var tier2Done sync.WaitGroup
	tier2Done.Add(1)

	g, ctx := errgroup.WithContext(ctx)

	// Tier 1
	g.Go(func() error { p.newClipEmbedWorker(r).Run(ctx); tier1Done.Done(); return nil })
	g.Go(func() error { p.newCaptionWorker(r).Run(ctx); tier1Done.Done(); return nil })
	g.Go(func() error { p.newOCRWorker(r).Run(ctx); tier1Done.Done(); return nil })
	g.Go(func() error { p.newTranscribeWorker(r).Run(ctx); tier1Done.Done(); return nil })

	// Cascade: close text_embed when tier 1 done
	g.Go(func() error { tier1Done.Wait(); r.closeCh(StageTextEmbed); return nil })

	// Tier 2
	g.Go(func() error { p.newTextEmbedWorker(r).Run(ctx); tier2Done.Done(); return nil })

	// Cascade: close upsert when tier 1 and tier 2 done
	g.Go(func() error { tier1Done.Wait(); tier2Done.Wait(); r.closeCh(StageUpsert); return nil })

	// Tier 3
	g.Go(func() error { p.newUpsertWorker(r).Run(ctx); return nil })

	// Read from input channel, create jobs, route
	for item := range p.input {
		fd := newFileData(item.FilePath, item.Data, 0)
		jobs := jobsForItem(item, fd, p.enabled)
		fd.pending.Store(int32(len(jobs)))
		for _, j := range jobs {
			r.route(j)
		}
	}

	// Input closed — close tier 1 inputs
	r.closeCh(StageClipEmbed)
	r.closeCh(StageCaption)
	r.closeCh(StageOCR)
	r.closeCh(StageTranscribe)

	return g.Wait()
}

// routeToUpsert creates an upsert-stage WorkItem and routes it.
func (p *Pipeline) routeToUpsert(r *router, item *job, chunkType string) {
	item.fileData.addRef()
	r.route(&job{
		fileData:  item.fileData,
		modality:  item.modality,
		stage:     StageUpsert,
		itemType:  chunkType,
		itemID:    item.itemID,
		itemIndex: item.itemIndex,
		text:      item.text,
		embedding: item.embedding,
	})
}

// releaseBatch releases FileData refs for all items in a batch.
func (p *Pipeline) releaseBatch(batch []*job) {
	for _, item := range batch {
		item.fileData.release()
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
