package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/store"
	pgvector "github.com/pgvector/pgvector-go"
)

// newClipEmbedWorker creates the CLIP image embedding batch worker.
func (p *Pipeline) newClipEmbedWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "clip_embed", BatchSize: p.cfg.EmbedBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageClipEmbed),
		Process: func(ctx context.Context, batch []*WorkItem) {
			if err := BatchClipEmbed(ctx, p.cfg.EmbedClient, p.cfg.EmbedModel, batch); err != nil {
				fmt.Printf("      clip_embed error: %v\n", err)
				p.releaseBatch(batch)
				return
			}
			for _, item := range batch {
				if item.Embedding.Slice() != nil {
					p.routeToUpsert(r, item, "embed")
				}
				item.FileData.Release()
			}
		},
	}
}

// newCaptionWorker creates the BLIP captioning batch worker.
func (p *Pipeline) newCaptionWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "caption", BatchSize: p.cfg.CaptionBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageCaption),
		Process: func(ctx context.Context, batch []*WorkItem) {
			if err := BatchCaption(ctx, p.cfg.VisionClient, batch); err != nil {
				fmt.Printf("      caption error: %v\n", err)
				p.releaseBatch(batch)
				return
			}
			for _, item := range batch {
				if item.Text != "" {
					p.routeToUpsert(r, item, "caption")

					if p.cfg.EmbedCaptionText {
						downstream := item.ForStage(StageTextEmbed)
						downstream.ItemType = "caption"
						item.FileData.AddRef()
						r.route(downstream)
					}
				}
				item.FileData.Release()
			}
		},
	}
}

// newOCRWorker creates the parallel OCR worker.
func (p *Pipeline) newOCRWorker(r *router) *ParallelWorker {
	return &ParallelWorker{
		Name: "ocr", MaxWorkers: p.cfg.OCRWorkers,
		Queue: r.ch(StageOCR),
		Process: func(ctx context.Context, item *WorkItem) {
			if err := OCR(ctx, p.cfg.VisionClient, item); err != nil {
				fmt.Printf("      ocr error: %v\n", err)
				item.FileData.Release()
				return
			}
			if item.Text != "" {
				p.routeToUpsert(r, item, "ocr")

				if p.cfg.EmbedOCRText {
					chunks := chunkText(item.Text, p.cfg.ChunkSize,
						p.cfg.ChunkOverlap, p.cfg.MinChunkSize)
					for i, c := range chunks {
						item.FileData.AddRef()
						r.route(&WorkItem{
							FileData: item.FileData, Modality: item.Modality,
							Stage: StageTextEmbed, ItemType: "ocr",
							ItemID: item.ItemID, ItemIndex: i,
							Text: c,
						})
					}
				}
			}
			item.FileData.Release()
		},
	}
}

// newTranscribeWorker creates the parallel transcription worker.
func (p *Pipeline) newTranscribeWorker(r *router) *ParallelWorker {
	return &ParallelWorker{
		Name: "transcribe", MaxWorkers: p.cfg.TranscribeWorkers,
		Queue: r.ch(StageTranscribe),
		Process: func(ctx context.Context, item *WorkItem) {
			if err := Transcribe(ctx, p.cfg.TranscribeClient, item); err != nil {
				fmt.Printf("      transcribe error: %v\n", err)
				item.FileData.Release()
				return
			}
			if item.Text != "" {
				p.routeToUpsert(r, item, "transcript")

				if p.cfg.EmbedTranscriptText {
					chunks := chunkText(item.Text, p.cfg.ChunkSize,
						p.cfg.ChunkOverlap, p.cfg.MinChunkSize)
					for i, c := range chunks {
						item.FileData.AddRef()
						r.route(&WorkItem{
							FileData: item.FileData, Modality: item.Modality,
							Stage: StageTextEmbed, ItemType: "transcript",
							ItemID: item.ItemID, ItemIndex: i,
							Text: c,
						})
					}
				}
			}
			item.FileData.Release()
		},
	}
}

// newTextEmbedWorker creates the text embedding batch worker.
func (p *Pipeline) newTextEmbedWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "text_embed", BatchSize: p.cfg.TextEmbedBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageTextEmbed),
		Process: func(ctx context.Context, batch []*WorkItem) {
			if err := BatchTextEmbed(ctx, p.cfg.EmbedClient, p.cfg.EmbedModel, batch); err != nil {
				fmt.Printf("      text_embed error: %v\n", err)
				p.releaseBatch(batch)
				return
			}
			for _, item := range batch {
				p.routeToUpsert(r, item, "embed")
			}
		},
	}
}

// newUpsertWorker creates the batched DB upsert worker.
func (p *Pipeline) newUpsertWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "upsert", BatchSize: p.cfg.UpsertBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageUpsert),
		Process: func(ctx context.Context, batch []*WorkItem) {
			chunks := make([]store.ChunkRow, 0, len(batch))
			for _, item := range batch {
				var emb *pgvector.Vector
				if item.Embedding.Slice() != nil {
					e := item.Embedding
					emb = &e
				}
				chunks = append(chunks, store.ChunkRow{
					ItemID:      item.ItemID,
					ChunkIndex:  item.ItemIndex,
					ChunkType:   item.ItemType,
					EmbedModel:  p.cfg.EmbedModel,
					Embedding:   emb,
					TextContent: strPtr(item.Text),
				})
			}
			if err := store.UpsertChunkBatch(ctx, chunks); err != nil {
				fmt.Printf("      upsert error: %v\n", err)
			}
			p.releaseBatch(batch)
		},
	}
}
