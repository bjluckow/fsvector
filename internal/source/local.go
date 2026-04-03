package source

import (
	"context"
	"strings"

	"github.com/bjluckow/fsvector/internal/fsindex"
	"github.com/bjluckow/fsvector/internal/fswatch"
)

// LocalSource wraps fsindex and watcher for local filesystem access.
// fsindex and watcher packages are unchanged.
type LocalSource struct {
	Root string
}

func NewLocalSource(root string) *LocalSource {
	return &LocalSource{Root: root}
}

func (s *LocalSource) Walk(ctx context.Context) ([]FileInfo, error) {
	files, err := fsindex.Walk(s.Root)
	if err != nil {
		return nil, err
	}
	return convertFileInfos(files), nil
}

func (s *LocalSource) Reader() FileReader {
	return &LocalReader{}
}

func (s *LocalSource) URI() string {
	return "local"
}

// Watch implements Watcher — wraps watcher.Watch and converts events.
func (s *LocalSource) Watch(ctx context.Context, events chan<- Event) error {
	internal := make(chan fswatch.Event, 64)
	go func() {
		for e := range internal {
			events <- convertEvent(e)
		}
	}()
	return fswatch.Watch(ctx, s.Root, internal)
}

// FileInfoFromPath returns a source.FileInfo for a single local path.
// Used by handleEvents for fsnotify file events.
func FileInfoFromPath(path string) (FileInfo, error) {
	fi, err := fsindex.FileInfoFromPath(path)
	if err != nil {
		return FileInfo{}, err
	}
	return convertFileInfo(fi), nil
}

// convertFileInfo converts a single fsindex.FileInfo to source.FileInfo.
func convertFileInfo(f fsindex.FileInfo) FileInfo {
	return FileInfo{
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
func convertFileInfos(files []fsindex.FileInfo) []FileInfo {
	result := make([]FileInfo, len(files))
	for i, f := range files {
		result[i] = convertFileInfo(f)
	}
	return result
}

// convertEvent converts watcher.Event to source.Event.
func convertEvent(e fswatch.Event) Event {
	var kind EventKind
	switch e.Kind {
	case fswatch.EventCreate:
		kind = EventCreate
	case fswatch.EventUpdate:
		kind = EventUpdate
	case fswatch.EventDelete:
		kind = EventDelete
	}
	return Event{Kind: kind, Path: e.Path}
}
