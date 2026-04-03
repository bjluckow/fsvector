package source

import (
	"context"
	"time"
)

// FileInfo is source-agnostic file metadata.
type FileInfo struct {
	Path       string
	Name       string
	Ext        string
	Size       int64
	MimeType   string
	Hash       string
	ModifiedAt time.Time
	CreatedAt  time.Time
	SourceURI  string
}

// EventKind represents the type of source event.
type EventKind int

const (
	EventCreate EventKind = iota
	EventUpdate
	EventDelete
)

func (e EventKind) String() string {
	switch e {
	case EventCreate:
		return "create"
	case EventUpdate:
		return "update"
	case EventDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// Event represents a file change event from a source.
type Event struct {
	Kind EventKind
	Path string
}

// Source abstracts a file collection.
type Source interface {
	Walk(ctx context.Context) ([]FileInfo, error)
	Reader() FileReader
	URI() string
}

// Watcher is implemented by sources that support live event watching.
// Sources that don't support watching (e.g. S3) do not implement this.
type Watcher interface {
	Watch(ctx context.Context, events chan<- Event) error
}
