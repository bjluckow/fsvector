package indexer

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

func (idx *Indexer) Reindex(ctx context.Context) error {
	idx.progress.start()
	defer idx.progress.finish()

	src := idx.source
	fmt.Printf("  reindexing %s\n", src.URI())
	start := time.Now()

	fsFiles, err := src.Walk(ctx)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	dbFiles, err := store.AllPaths(ctx)
	if err != nil {
		return fmt.Errorf("paths: %w", err)
	}
	if len(dbFiles) > 100 && len(fsFiles) < len(dbFiles)/10 {
		return fmt.Errorf("walk returned only %d files vs %d in DB — aborting",
			len(fsFiles), len(dbFiles))
	}

	idx.progress.setTotal(len(fsFiles))

	fsMap := buildFSMap(fsFiles)
	deleteStale(ctx, src.URI(), fsMap, dbFiles, idx.progress)

	diff := DiffFiles(ctx, fsFiles, dbFiles)
	idx.progress.addSkipped(diff.Skipped)
	idx.progress.addIndexed(diff.Dupes)
	for _, e := range diff.Errors {
		idx.progress.addError(e)
	}

	if len(diff.ToProcess) > 0 {
		fmt.Printf("  %d files to process\n", len(diff.ToProcess))
		if err := idx.Index(ctx, diff.ToProcess); err != nil {
			return err
		}
	}

	snap := idx.progress.Snapshot()
	fmt.Printf("  reindex complete: %d indexed, %d skipped, %d deleted, %d errors (%s)\n",
		snap.Indexed, snap.Skipped, snap.Deleted, len(snap.Errors),
		time.Since(start).Round(time.Second))

	return nil
}

func (idx *Indexer) handleEvents(ctx context.Context, events <-chan source.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			switch e.Kind {
			case source.EventDelete:
				if err := store.SoftDelete(ctx, e.Path); err != nil {
					fmt.Fprintf(os.Stderr, "  delete %s: %v\n", e.Path, err)
				} else {
					fmt.Printf("  deleted %s\n", e.Path)
				}

			case source.EventCreate, source.EventUpdate:
				fi, err := source.FileInfoFromPath(e.Path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  stat %s: %v\n", e.Path, err)
					continue
				}
				sf := fi.ToSourceFile()
				items, err := idx.indexFile(ctx, sf, modalityOrDefault(fi.Ext))
				if err != nil {
					fmt.Fprintf(os.Stderr, "  index %s: %v\n", e.Path, err)
					continue
				}
				for _, item := range items {
					select {
					case idx.output <- item:
					case <-ctx.Done():
						return
					}
				}
				fmt.Printf("  %s %s\n", e.Kind, e.Path)
			}
		}
	}
}

func modalityOrDefault(ext string) model.Modality {
	m, ok := model.FileModality(ext)
	if !ok {
		return "unknown"
	}
	return m
}

func buildFSMap(files []source.FileInfo) map[string]source.FileInfo {
	m := make(map[string]source.FileInfo, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

func deleteStale(ctx context.Context, sourceURI string, fsMap map[string]source.FileInfo, dbFiles map[string]string, progress *Progress) error {
	for path := range dbFiles {
		if _, exists := fsMap[path]; !exists {
			if err := store.SoftDelete(ctx, path); err != nil {
				fmt.Fprintf(os.Stderr, "    soft-delete %s: %v\n", path, err)
				progress.addError(fmt.Sprintf("soft-delete %s: %v", path, err))
				continue
			}
			progress.incDeleted()
		}
	}
	return nil
}
