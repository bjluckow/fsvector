package indexer

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/store"
)

type DiffResult struct {
	ToProcess []model.SourceFile
	Skipped   int
	Dupes     int
	Errors    []string
}

// DiffFiles compares source files against the DB, identifies
// files that need processing, handles dedup, and returns the diff.
func DiffFiles(ctx context.Context, fsFiles []model.SourceFile, dbFiles map[string]string) DiffResult {
	var result DiffResult

	for _, sf := range fsFiles {
		existingHash, inDB := dbFiles[sf.Path]
		if inDB && existingHash == sf.Hash {
			store.UnDelete(ctx, sf.Path)
			result.Skipped++
			continue
		}

		canonicalPath, isDupe, err := store.FindByHash(ctx, sf.Hash)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("hash check %s: %v", sf.Path, err))
			continue
		}
		if isDupe && canonicalPath != sf.Path {
			cp := canonicalPath
			if _, err := store.UpsertFile(ctx, model.File{
				Path:          sf.Path,
				Source:        sf.SourceURI,
				CanonicalPath: &cp,
				Modality:      string(modalityOrDefault(sf.Ext)),
				Name:          sf.Name,
				Ext:           sf.Ext,
				MimeType:      sf.MimeType,
				Size:          sf.Size,
				ContentHash:   sf.Hash,
				CreatedAt:     sf.CreatedAt,
				ModifiedAt:    sf.ModifiedAt,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("dupe %s: %v", sf.Path, err))
			} else {
				result.Dupes++
			}
			continue
		}

		result.ToProcess = append(result.ToProcess, sf)
	}

	return result
}
