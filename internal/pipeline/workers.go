package pipeline

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/chunk"
	pgvector "github.com/pgvector/pgvector-go"
)

// chunkText is a placeholder that defers to the existing chunk package.
// Replace with actual import of your chunk.Split function.
func chunkText(text string, size, overlap, minSize int) []string {
	return chunk.Split(text, size, overlap, minSize)
}

// newClipEmbedWorker creates the CLIP image embedding batch worker.
func (p *Pipeline) newClipEmbedWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "clip_embed", BatchSize: p.cfg.EmbedBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageClipEmbed),
		Process: func(ctx context.Context, batch []*job) {
			if err := BatchClipEmbed(ctx, p.cfg.EmbedClient, p.cfg.EmbedModel, batch); err != nil {
				fmt.Printf("      clip_embed error: %v\n", err)
				p.releaseBatch(batch)
				return
			}
			for _, item := range batch {
				if item.embedding.Slice() != nil {
					p.routeToUpsert(r, item, "embed")
				}
				item.fileData.release()
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
		Process: func(ctx context.Context, batch []*job) {
			if err := BatchCaption(ctx, p.cfg.VisionClient, batch); err != nil {
				fmt.Printf("      caption error: %v\n", err)
				p.releaseBatch(batch)
				return
			}
			for _, item := range batch {
				if item.text != "" {
					p.routeToUpsert(r, item, "caption")

					if p.cfg.EmbedCaptionText {
						downstream := item.ForStage(StageTextEmbed)
						downstream.itemType = "caption"
						item.fileData.addRef()
						r.route(downstream)
					}
				}
				item.fileData.release()
			}
		},
	}
}

// newOCRWorker creates the parallel OCR worker.
func (p *Pipeline) newOCRWorker(r *router) *ParallelWorker {
	return &ParallelWorker{
		Name: "ocr", MaxWorkers: p.cfg.OCRWorkers,
		Queue: r.ch(StageOCR),
		Process: func(ctx context.Context, item *job) {
			if err := OCR(ctx, p.cfg.VisionClient, item); err != nil {
				fmt.Printf("      ocr error: %v\n", err)
				item.fileData.release()
				return
			}
			if item.text != "" {
				p.routeToUpsert(r, item, "ocr")

				if p.cfg.EmbedOCRText {
					chunks := chunkText(item.text, p.cfg.ChunkSize,
						p.cfg.ChunkOverlap, p.cfg.MinChunkSize)
					for i, c := range chunks {
						item.fileData.addRef()
						r.route(&job{
							fileData: item.fileData, modality: item.modality,
							stage: StageTextEmbed, itemType: "ocr",
							itemID: item.itemID, itemIndex: i,
							text: c,
						})
					}
				}
			}
			item.fileData.release()
		},
	}
}

// newTranscribeWorker creates the parallel transcription worker.
func (p *Pipeline) newTranscribeWorker(r *router) *ParallelWorker {
	return &ParallelWorker{
		Name: "transcribe", MaxWorkers: p.cfg.TranscribeWorkers,
		Queue: r.ch(StageTranscribe),
		Process: func(ctx context.Context, item *job) {
			if err := Transcribe(ctx, p.cfg.TranscribeClient, item); err != nil {
				fmt.Printf("      transcribe error: %v\n", err)
				item.fileData.release()
				return
			}
			if item.text != "" {
				// store transcript metadata on the item row
				if item.metadata != nil {
					store.UpdateItemMetadata(ctx, item.itemID, item.metadata)
				}

				p.routeToUpsert(r, item, "transcript")

				if p.cfg.EmbedTranscriptText {
					chunks := chunkText(item.text, p.cfg.ChunkSize,
						p.cfg.ChunkOverlap, p.cfg.MinChunkSize)
					for i, c := range chunks {
						item.fileData.addRef()
						r.route(&job{
							fileData: item.fileData, modality: item.modality,
							stage: StageTextEmbed, itemType: "transcript",
							itemID: item.itemID, itemIndex: i,
							text: c,
						})
					}
				}
			}
			item.fileData.release()
		},
	}
}

// newTextEmbedWorker creates the text embedding batch worker.
func (p *Pipeline) newTextEmbedWorker(r *router) *BatchWorker {
	return &BatchWorker{
		Name: "text_embed", BatchSize: p.cfg.TextEmbedBatchSize,
		FlushTimeout: p.cfg.FlushTimeout,
		Queue:        r.ch(StageTextEmbed),
		Process: func(ctx context.Context, batch []*job) {
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
		Process: func(ctx context.Context, batch []*job) {
			chunks := make([]model.Chunk, 0, len(batch))
			for _, item := range batch {
				var emb *pgvector.Vector
				if item.embedding.Slice() != nil {
					e := item.embedding
					emb = &e
				}
				chunks = append(chunks, model.Chunk{
					ItemID:      item.itemID,
					ChunkIndex:  item.itemIndex,
					ChunkType:   item.itemType,
					EmbedModel:  p.cfg.EmbedModel,
					Embedding:   emb,
					TextContent: strPtr(item.text),
				})
			}
			if err := store.UpsertChunkBatch(ctx, chunks); err != nil {
				fmt.Printf("      upsert error: %v\n", err)
			}
			p.releaseBatch(batch)
		},
	}
}
