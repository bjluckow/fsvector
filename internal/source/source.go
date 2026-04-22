package source

import (
	"context"
	"time"

	"github.com/bjluckow/fsvector/internal/model"
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

func (fi FileInfo) ToSourceFile() model.SourceFile {
	return model.SourceFile{
		Path:       fi.Path,
		Name:       fi.Name,
		Ext:        fi.Ext,
		Size:       fi.Size,
		MimeType:   fi.MimeType,
		Hash:       fi.Hash,
		CreatedAt:  fi.CreatedAt,
		ModifiedAt: fi.ModifiedAt,
		SourceURI:  fi.SourceURI,
	}
}

// Source abstracts a file collection — local filesystem, S3, email, etc.
type Source interface {
	Walk(ctx context.Context) ([]FileInfo, error)
	Reader() FileReader
	URI() string
	PollInterval() time.Duration // 0 = no polling, explicit reindex only
}

// Watchable is optionally implemented by sources that support
// real-time file events. Currently only LocalSource implements this.
type Watchable interface {
	Watch(ctx context.Context, events chan<- Event) error
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
