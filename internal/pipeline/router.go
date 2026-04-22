package pipeline

import (
	"context"
	"fmt"
)

// router manages stage queues and gates work items based on
// enabled stages. Single point of control for feature flags.
type router struct {
	ctx     context.Context
	queues  map[Stage]chan *WorkItem
	enabled map[Stage]bool
}

func newRouter(ctx context.Context, queues map[Stage]chan *WorkItem, enabled map[Stage]bool) *router {
	return &router{ctx: ctx, queues: queues, enabled: enabled}
}

func (r *router) route(item *WorkItem) {
	if !r.enabled[item.Stage] {
		item.FileData.Release()
		return
	}
	q, ok := r.queues[item.Stage]
	if !ok {
		fmt.Printf("    router: no queue for stage %s, dropping\n", item.Stage)
		item.FileData.Release()
		return
	}
	select {
	case q <- item:
	case <-r.ctx.Done():
		item.FileData.Release()
	}
}

func (r *router) routeMany(items []*WorkItem) {
	for _, item := range items {
		r.route(item)
	}
}

func (r *router) ch(stage Stage) <-chan *WorkItem {
	return r.queues[stage]
}

func (r *router) closeCh(stage Stage) {
	close(r.queues[stage])
}
