package watcher

import (
	"context"
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
)

// EventKind represents the type of filesystem event.
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

// Event is a normalized filesystem event.
type Event struct {
	Kind EventKind
	Path string
}

// Watch watches root recursively and sends normalized events to the returned
// channel. It blocks until ctx is cancelled or a fatal error occurs.
func Watch(ctx context.Context, root string, events chan<- Event) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	defer w.Close()

	// add root and all subdirectories
	if err := addDirs(w, root); err != nil {
		return fmt.Errorf("watcher add dirs: %w", err)
	}

	fmt.Printf("  watching %s\n", root)

	for {
		select {
		case <-ctx.Done():
			return nil

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)

		case e, ok := <-w.Events:
			if !ok {
				return nil
			}

			// skip directories themselves — we handle new dirs below
			if isDir(e.Name) {
				// if a new directory was created, watch it
				if e.Has(fsnotify.Create) {
					_ = addDirs(w, e.Name)
				}
				continue
			}

			switch {
			case e.Has(fsnotify.Create):
				events <- Event{Kind: EventCreate, Path: e.Name}
			case e.Has(fsnotify.Write):
				events <- Event{Kind: EventUpdate, Path: e.Name}
			case e.Has(fsnotify.Remove), e.Has(fsnotify.Rename):
				events <- Event{Kind: EventDelete, Path: e.Name}
			case e.Has(fsnotify.Chmod):
				// ignore permission changes
			}
		}
	}
}

// addDirs recursively adds root and all subdirectories to the watcher.
func addDirs(w *fsnotify.Watcher, root string) error {
	// add the root itself
	if err := w.Add(root); err != nil {
		return err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := addDirs(w, root+"/"+e.Name()); err != nil {
				// non-fatal — log and continue
				fmt.Fprintf(os.Stderr, "watcher: skipping %s: %v\n", e.Name(), err)
			}
		}
	}
	return nil
}

// isDir returns true if path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
