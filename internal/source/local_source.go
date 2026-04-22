package source

import (
	"context"
	"strings"

	"github.com/bjluckow/fsvector/internal/fswalk"
	"github.com/bjluckow/fsvector/internal/model"
)

// LocalSource wraps fsindex and watcher for local filesystem access.
// fsindex and watcher packages are unchanged.
type LocalSource struct {
	Root         string
	WatchEnabled bool
}

func NewLocalSource(root string, watch bool) *LocalSource {
	return &LocalSource{
		Root:         root,
		WatchEnabled: watch,
	}
}

func (s *LocalSource) Walk(ctx context.Context) ([]model.SourceFile, error) {
	files, err := fswalk.Walk(s.Root)
	if err != nil {
		return nil, err
	}
	return convertFileInfos(files), nil
}

func (s *LocalSource) Reader() FileReader { return &LocalReader{} }
func (s *LocalSource) URI() string        { return "local://" + s.Root }

// Watch implements Watchable — only available when WatchEnabled is true.
func (s *LocalSource) Watch(ctx context.Context, events chan<- Event) error {
	internal := make(chan fswalk.Event, 64)
	go func() {
		for e := range internal {
			events <- convertEvent(e)
		}
	}()
	return fswalk.Watch(ctx, s.Root, internal)
}

// FileInfoFromPath returns a source.FileInfo for a single local path.
// Used by handleEvents for fsnotify file events.
func FileInfoFromPath(path string) (model.SourceFile, error) {
	fi, err := fswalk.FileInfoFromPath(path)
	if err != nil {
		return model.SourceFile{}, err
	}
	return convertFileInfo(fi), nil
}

// convertFileInfo converts a single fsindex.FileInfo to source.FileInfo.
func convertFileInfo(f fswalk.FileInfo) model.SourceFile {
	return model.SourceFile{
		Path:       f.Path,
		Name:       f.Name,
		Ext:        strings.ToLower(f.Ext),
		Size:       f.Size,
		MimeType:   f.MimeType,
		Hash:       f.Hash,
		ModifiedAt: f.ModifiedAt,
		CreatedAt:  f.CreatedAt,
		SourceURI:  "local",
	}
}

// convertFileInfos converts fsindex.FileInfo slice to source.FileInfo slice.
func convertFileInfos(files []fswalk.FileInfo) []model.SourceFile {
	result := make([]model.SourceFile, len(files))
	for i, f := range files {
		result[i] = convertFileInfo(f)
	}
	return result
}

// convertEvent converts watcher.Event to source.Event.
func convertEvent(e fswalk.Event) Event {
	var kind EventKind
	switch e.Kind {
	case fswalk.EventCreate:
		kind = EventCreate
	case fswalk.EventUpdate:
		kind = EventUpdate
	case fswalk.EventDelete:
		kind = EventDelete
	}
	return Event{Kind: kind, Path: e.Path}
}
