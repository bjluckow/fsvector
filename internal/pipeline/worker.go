package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BatchWorker accumulates WorkItems from its input channel and
// flushes them as a batch when either the batch is full or the
// flush timeout fires. This is the core pattern for all model
// inference workers (embed, caption, text_embed) and the upsert worker.
//
// For non-batched operations (OCR, transcribe), use ParallelWorker instead.
type BatchWorker struct {
	Name         string
	BatchSize    int
	FlushTimeout time.Duration
	Queue        <-chan *job
	Process      func(ctx context.Context, batch []*job)
}

// Run starts the worker loop. It blocks until the input channel
// is closed (and drained) or the context is canceled.
func (w *BatchWorker) Run(ctx context.Context) {
	var buf []*job
	ticker := time.NewTicker(w.FlushTimeout)
	defer ticker.Stop()

	for {
		select {
		case item, ok := <-w.Queue:
			if !ok {
				w.flush(ctx, buf)
				return
			}
			buf = append(buf, item)
			if len(buf) >= w.BatchSize {
				w.flush(ctx, buf)
				buf = nil
			}
		case <-ticker.C:
			if len(buf) > 0 {
				w.flush(ctx, buf)
				buf = nil
			}
		case <-ctx.Done():
			// release any buffered items
			for _, item := range buf {
				item.fileData.release()
			}
			return
		}
	}
}

func (w *BatchWorker) flush(ctx context.Context, batch []*job) {
	if len(batch) == 0 {
		return
	}
	start := time.Now()
	w.Process(ctx, batch)
	fmt.Printf("    %s: flushed %d items (%s)\n", w.Name, len(batch), time.Since(start).Round(time.Millisecond))
}

// ParallelWorker processes items one at a time but with bounded
// concurrency. Used for operations that can't batch (OCR, transcribe)
// but benefit from concurrent requests to the service.
type ParallelWorker struct {
	Name       string
	MaxWorkers int
	Queue      <-chan *job
	Process    func(ctx context.Context, item *job)
}

// Run starts the worker loop with bounded concurrency.
// It blocks until the input channel is closed and all in-flight
// items are done, or the context is canceled.
func (w *ParallelWorker) Run(ctx context.Context) {
	sem := make(chan struct{}, w.MaxWorkers)
	var wg sync.WaitGroup

	for {
		select {
		case item, ok := <-w.Queue:
			if !ok {
				wg.Wait()
				return
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(it *job) {
				defer func() {
					<-sem
					wg.Done()
				}()
				w.Process(ctx, it)
			}(item)
		case <-ctx.Done():
			wg.Wait()
			return
		}
	}
}
