package daemon

import (
	"sync"
	"time"
)

type (
	// Progress tracks the state of a reconciliation run.
	Progress struct {
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

	// ProgressSnapshot is a read-only copy of Progress for serialization.
	ProgressSnapshot struct {
		Running    bool       `json:"running"`
		Total      int        `json:"total"`
		Indexed    int        `json:"indexed"`
		Deleted    int        `json:"deleted"`
		Skipped    int        `json:"skipped"`
		Errors     []string   `json:"errors"`
		StartedAt  time.Time  `json:"started_at"`
		FinishedAt *time.Time `json:"finished_at"`
	}
)

func (p *Progress) snapshot() ProgressSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return ProgressSnapshot{
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

func (p *Progress) setTotal(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Total = n
}

func (p *Progress) incIndexed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Indexed++
}

func (p *Progress) incDeleted() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Deleted++
}

func (p *Progress) incSkipped() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Skipped++
}

func (p *Progress) addError(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Errors = append(p.Errors, msg)
}
