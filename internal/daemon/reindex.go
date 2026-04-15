package daemon

import (
	"context"
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/pipeline"
	"github.com/bjluckow/fsvector/internal/source"
	"github.com/bjluckow/fsvector/internal/store"
)

// Reindex diffs the source against the DB and brings the index into sync.
func Reindex(ctx context.Context, pCfg pipeline.Config, src source.Source, progress *Progress) error {
	progress.start()
	defer progress.finish()

	fmt.Printf("  reindexing %s\n", src.URI())

	fsFiles, err := src.Walk(ctx)
	if err != nil {
		return fmt.Errorf("walk: %w", err)
	}

	progress.setTotal(len(fsFiles))

	fsMap := buildFilepathMap(fsFiles)

	dbFiles, err := store.LivePaths(ctx)
	if err != nil {
		return fmt.Errorf("live paths: %w", err)
	}

	if err := deleteStale(ctx, fsMap, dbFiles, progress); err != nil {
		return err
	}

	if err := indexNew(ctx, pCfg, fsFiles, dbFiles, progress); err != nil {
		return err
	}

	fmt.Printf("  reindexing done: %d indexed, %d deleted, %d unchanged\n",
		progress.Indexed, progress.Deleted, progress.Skipped)
	return nil
}

// buildFilepathMap builds a path-keyed map from a slice of FileInfo.
func buildFilepathMap(files []source.FileInfo) map[string]source.FileInfo {
	m := make(map[string]source.FileInfo, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

// deleteStale soft-deletes DB rows for files no longer in the source.
func deleteStale(
	ctx context.Context,
	fsMap map[string]source.FileInfo,
	dbFiles map[string]string,
	progress *Progress,
) error {
	for path := range dbFiles {
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

// indexNew processes files that are new or changed since last reindex.
func indexNew(
	ctx context.Context,
	pCfg pipeline.Config,
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

		if err := indexFile(ctx, pCfg, fi, progress); err != nil {
			fmt.Fprintf(os.Stderr, "    %s: %v\n", fi.Path, err)
			progress.addError(fmt.Sprintf("%s: %v", fi.Path, err))
		}
	}
	return nil
}

// indexFile processes and upserts a single file.
func indexFile(
	ctx context.Context,
	pCfg pipeline.Config,
	fi source.FileInfo,
	progress *Progress,
) error {
	// dedup check
	if canonicalPath, isDupe, err := store.FindByHash(ctx, fi.Hash); err != nil {
		return fmt.Errorf("hash check: %w", err)
	} else if isDupe && canonicalPath != fi.Path {
		modality, _ := pipeline.Modality(fi.Ext)
		f := store.File{
			Path:           fi.Path,
			Source:         pCfg.Source,
			ContentHash:    fi.Hash,
			Size:           fi.Size,
			MimeType:       fi.MimeType,
			Modality:       modality,
			FileName:       fi.Name,
			FileExt:        fi.Ext,
			FileCreatedAt:  &fi.CreatedAt,
			FileModifiedAt: &fi.ModifiedAt,
			EmbedModel:     pCfg.EmbedModel,
		}
		if err := store.Upsert(ctx, f); err != nil {
			return fmt.Errorf("dupe upsert: %w", err)
		}
		fmt.Printf("    duplicate %s -> %s\n", fi.Path, canonicalPath)
		progress.incIndexed()
		return nil
	}

	result, err := pipeline.Process(ctx, pCfg, fi)
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
	if err := store.DeleteStaleChunks(ctx, fi.Path, pCfg.EmbedModel, len(result.Files)); err != nil {
		fmt.Fprintf(os.Stderr, "    stale chunks %s: %v\n", fi.Path, err)
	}

	fmt.Printf("    indexed %s (%s, %d chunks)\n", fi.Path, result.Files[0].Modality, len(result.Files))
	progress.incIndexed()
	return nil
}
