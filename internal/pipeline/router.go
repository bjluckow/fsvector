package pipeline

import "fmt"

// StageQueues maps each stage to its worker's input channel.
type StageQueues map[Stage]chan *WorkItem

// Route sends a WorkItem to the appropriate queue based on its Stage.
// If no queue exists for the stage, the item's FileData refcount is
// released to prevent memory leaks, and the item is dropped with a log.
func (sq StageQueues) Route(item *WorkItem) {
	q, ok := sq[item.Stage]
	if !ok {
		fmt.Printf("    router: no queue for stage %s (item %s), dropping\n",
			item.Stage, item.FileData.FileInfo.Path)
		item.FileData.Release()
		return
	}
	q <- item
}

// RouteMany sends multiple WorkItems to their respective queues.
func (sq StageQueues) RouteMany(items []*WorkItem) {
	for _, item := range items {
		sq.Route(item)
	}
}
