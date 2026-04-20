package reindex

import (
	"sync"
	"time"

	"github.com/bjluckow/fsvector/pkg/api"
)

// Progress tracks the state of a reindex run.
// Exported so server can read it.
type Progress struct {
	mu         sync.RWMutex
	Running    bool
	Total      int
	Indexed    int
	Deleted    int
	Skipped    int
	Errors     []string
	StartedAt  time.Time
	FinishedAt *time.Time
}

func (p *Progress) Snapshot() api.ProgressSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return api.ProgressSnapshot{
		Running:    p.Running,
		Total:      p.Total,
		Indexed:    p.Indexed,
		Deleted:    p.Deleted,
		Skipped:    p.Skipped,
		Errors:     p.Errors,
		StartedAt:  p.StartedAt,
		FinishedAt: p.FinishedAt,
	}
}

func (p *Progress) start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Running = true
	p.StartedAt = time.Now()
	p.Indexed, p.Deleted, p.Skipped = 0, 0, 0
	p.Errors = nil
	p.FinishedAt = nil
}

func (p *Progress) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Running = false
	now := time.Now()
	p.FinishedAt = &now
}

func (p *Progress) setTotal(n int) { p.mu.Lock(); p.Total = n; p.mu.Unlock() }
func (p *Progress) incIndexed()    { p.mu.Lock(); p.Indexed++; p.mu.Unlock() }
func (p *Progress) incDeleted()    { p.mu.Lock(); p.Deleted++; p.mu.Unlock() }
func (p *Progress) incSkipped()    { p.mu.Lock(); p.Skipped++; p.mu.Unlock() }
func (p *Progress) addError(msg string) {
	p.mu.Lock()
	p.Errors = append(p.Errors, msg)
	p.mu.Unlock()
}
