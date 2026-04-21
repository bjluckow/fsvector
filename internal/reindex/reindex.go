package reindex

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
	"github.com/bjluckow/fsvector/pkg/api"
)

type Trigger struct {
	Purge bool
	// future: Force bool     // ignore hash check
}

// IndexAndPoll manages the full lifecycle of a single source:
// initial reindex, optional watching, and periodic polling.
func IndexAndPoll(ctx context.Context, src source.Source, pl pipeline.Pipeline, progress *Progress, trigger <-chan Trigger) {
	// initial reindex
	if err := Reindex(ctx, src, pl, progress); err != nil {
		fmt.Fprintf(os.Stderr, "reindex %s: %v\n", src.URI(), err)
	}

	// start watcher if supported
	if w, ok := src.(source.Watchable); ok {
		events := make(chan source.Event, 64)
		go func() {
			if err := w.Watch(ctx, events); err != nil {
				fmt.Fprintf(os.Stderr, "watch %s: %v\n", src.URI(), err)
			}
		}()
		go handleEvents(ctx, src, pl, events)
	}

	// poll if interval set
	if src.PollInterval() > 0 {
		ticker := time.NewTicker(src.PollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !progress.Running {
					if err := Reindex(ctx, src, pl, progress); err != nil {
						fmt.Fprintf(os.Stderr, "poll reindex %s: %v\n", src.URI(), err)
					}
				}
			case t := <-trigger:
				if !progress.Running {
					Reindex(ctx, src, pl, progress)
					if t.Purge {
						store.PurgeSoftDeleted(ctx)
					}
				}
			}
		}
	} else {
		<-ctx.Done()
	}
}

// Reindex diffs the source against the DB and brings the index into sync.
func Reindex(ctx context.Context, src source.Source, pl pipeline.Pipeline, progress *Progress) error {
	progress.start()
	defer progress.finish()

	fmt.Printf("  reindexing %s\n", src.URI())

	fsFiles, err := src.Walk(ctx)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	// safety check — abort if walk returns suspiciously few files
	dbFiles, err := store.LivePaths(ctx)
	if err != nil {
		return fmt.Errorf("live paths: %w", err)
	}
	if len(dbFiles) > 100 && len(fsFiles) < len(dbFiles)/10 {
		return fmt.Errorf("walk returned only %d files vs %d in DB — aborting to prevent data loss",
			len(fsFiles), len(dbFiles))
	}

	progress.setTotal(len(fsFiles))

	// sort by modality priority — text first, email last
	sort.Slice(fsFiles, func(i, j int) bool {
		return modalityPriority(fsFiles[i].Ext) < modalityPriority(fsFiles[j].Ext)
	})

	fsMap := buildFSMap(fsFiles)

	if err := deleteStale(ctx, src.URI(), fsMap, dbFiles, progress); err != nil {
		return err
	}

	if err := indexNew(ctx, pl, fsFiles, dbFiles, progress); err != nil {
		return err
	}

	snap := progress.Snapshot()
	fmt.Printf("  reindex complete: %d indexed, %d skipped, %d deleted, %d errors (%s)\n",
		snap.Indexed, snap.Skipped, snap.Deleted, len(snap.Errors),
		time.Since(snap.StartedAt).Round(time.Second))

	return nil
}

// modalityPriority returns processing order — lower is higher priority.
func modalityPriority(ext string) int {
	modality, _ := pipeline.Modality(ext)
	switch modality {
	case "text":
		return 0
	case "image":
		return 1
	case "audio":
		return 2
	case "video":
		return 3
	case "email":
		return 4
	default:
		return 5
	}
}

func buildFSMap(files []source.FileInfo) map[string]source.FileInfo {
	m := make(map[string]source.FileInfo, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

func deleteStale(
	ctx context.Context,
	sourceURI string,
	fsMap map[string]source.FileInfo,
	dbFiles map[string]string,
	progress *Progress,
) error {
	for path := range dbFiles {
		if strings.Contains(path, api.AttachmentSep) {
			continue // never soft-delete synthetic attachment paths
		}
		if _, exists := fsMap[path]; !exists {
			if err := store.SoftDelete(ctx, path); err != nil {
				fmt.Fprintf(os.Stderr, "    soft-delete %s: %v\n", path, err)
				progress.addError(fmt.Sprintf("soft-delete %s: %v", path, err))
				continue
			}
			fmt.Printf("    deleted %s\n", path)
			progress.incDeleted()
		}
	}
	return nil
}

func indexNew(
	ctx context.Context,
	pl pipeline.Pipeline,
	fsFiles []source.FileInfo,
	dbFiles map[string]string,
	progress *Progress,
) error {
	for _, fi := range fsFiles {
		existingHash, inDB := dbFiles[fi.Path]
		if inDB && existingHash == fi.Hash {
			progress.incSkipped()
			continue
		}
		if err := indexFile(ctx, pl, fi, progress); err != nil {
			fmt.Fprintf(os.Stderr, "    %s: %v\n", fi.Path, err)
			progress.addError(fmt.Sprintf("%s: %v", fi.Path, err))
		}
	}
	return nil
}

func indexFile(
	ctx context.Context,
	pl pipeline.Pipeline,
	fi source.FileInfo,
	progress *Progress,
) error {
	// dedup check
	canonicalPath, isDupe, err := store.FindByHash(ctx, fi.Hash)
	if err != nil {
		return fmt.Errorf("hash check: %w", err)
	}
	if isDupe && canonicalPath != fi.Path {
		cp := canonicalPath
		f := store.UpsertFile{
			Path:           fi.Path,
			Source:         fi.SourceURI,
			CanonicalPath:  &cp,
			ContentHash:    fi.Hash,
			Size:           fi.Size,
			MimeType:       fi.MimeType,
			Modality:       modalityOrDefault(fi.Ext),
			FileName:       fi.Name,
			FileExt:        fi.Ext,
			FileCreatedAt:  &fi.CreatedAt,
			FileModifiedAt: &fi.ModifiedAt,
		}
		if err := store.Upsert(ctx, f); err != nil {
			return fmt.Errorf("dupe upsert: %w", err)
		}
		fmt.Printf("    duplicate %s -> %s\n", fi.Path, canonicalPath)
		progress.incIndexed()
		return nil
	}

	result, err := pl.ReadAndProcessFile(ctx, fi)
	if err != nil {
		return fmt.Errorf("pipeline: %w", err)
	}
	if result.Skipped {
		fmt.Printf("    skipped %s (%s)\n", fi.Path, result.SkipReason)
		progress.incSkipped()
		return nil
	}

	for _, f := range result.Files {
		if err := store.Upsert(ctx, f); err != nil {
			return fmt.Errorf("upsert chunk %d: %w", f.ChunkIndex, err)
		}
	}
	if err := store.DeleteStaleChunks(ctx, fi.Path, pl.EmbedModel, len(result.Files)); err != nil {
		fmt.Fprintf(os.Stderr, "    stale chunks %s: %v\n", fi.Path, err)
	}

	fmt.Printf("    indexed %s (%s, %d chunks)\n",
		fi.Path, result.Files[0].Modality, len(result.Files))
	progress.incIndexed()
	return nil
}

func modalityOrDefault(ext string) string {
	m, ok := pipeline.Modality(ext)
	if !ok {
		return "unknown"
	}
	return m
}

func handleEvents(
	ctx context.Context,
	src source.Source,
	pl pipeline.Pipeline,
	events <-chan source.Event,
) {
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
				result, err := pl.ReadAndProcessFile(ctx, fi)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  pipeline %s: %v\n", e.Path, err)
					continue
				}
				if result.Skipped {
					continue
				}
				for _, f := range result.Files {
					if err := store.Upsert(ctx, f); err != nil {
						fmt.Fprintf(os.Stderr, "  upsert %s chunk %d: %v\n",
							e.Path, f.ChunkIndex, err)
					}
				}
				if err := store.DeleteStaleChunks(ctx, e.Path,
					pl.EmbedModel, len(result.Files)); err != nil {
					fmt.Fprintf(os.Stderr, "  stale chunks %s: %v\n", e.Path, err)
				}
				fmt.Printf("  %s %s (%s, %d chunks)\n",
					e.Kind, e.Path, result.Files[0].Modality, len(result.Files))
			}
		}
	}
}
