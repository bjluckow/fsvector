package pipeline

import (
	"context"
	"fmt"
)

// router manages stage queues and gates work items based on
// enabled stages. Single point of control for feature flags.
type router struct {
	ctx     context.Context
	queues  map[Stage]chan *job
	enabled map[Stage]bool
}

func newRouter(ctx context.Context, queues map[Stage]chan *job, enabled map[Stage]bool) *router {
	return &router{ctx: ctx, queues: queues, enabled: enabled}
}

func (r *router) route(item *job) {
	if !r.enabled[item.stage] {
		item.fileData.release()
		return
	}
	q, ok := r.queues[item.stage]
	if !ok {
		fmt.Printf("    router: no queue for stage %s, dropping\n", item.stage)
		item.fileData.release()
		return
	}
	select {
	case q <- item:
	case <-r.ctx.Done():
		item.fileData.release()
	}
}

func (r *router) routeMany(items []*job) {
	for _, item := range items {
		r.route(item)
	}
}

func (r *router) ch(stage Stage) <-chan *job {
	return r.queues[stage]
}

func (r *router) closeCh(stage Stage) {
	close(r.queues[stage])
}
