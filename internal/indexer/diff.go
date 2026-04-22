package indexer

import (
	"context"
	"fmt"

	"github.com/bjluckow/fsvector/internal/model"
	"github.com/bjluckow/fsvector/internal/source"
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
func DiffFiles(ctx context.Context, fsFiles []source.FileInfo, dbFiles map[string]string) DiffResult {
	var result DiffResult

	for _, fi := range fsFiles {
		existingHash, inDB := dbFiles[fi.Path]
		if inDB && existingHash == fi.Hash {
			store.UnDelete(ctx, fi.Path)
			result.Skipped++
			continue
		}

		canonicalPath, isDupe, err := store.FindByHash(ctx, fi.Hash)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("hash check %s: %v", fi.Path, err))
			continue
		}
		if isDupe && canonicalPath != fi.Path {
			cp := canonicalPath
			if _, err := store.UpsertFile(ctx, model.File{
				Path:          fi.Path,
				Source:        fi.SourceURI,
				CanonicalPath: &cp,
				Modality:      string(modalityOrDefault(fi.Ext)),
				Name:          fi.Name,
				Ext:           fi.Ext,
				MimeType:      fi.MimeType,
				Size:          fi.Size,
				ContentHash:   fi.Hash,
				CreatedAt:     fi.CreatedAt,
				ModifiedAt:    fi.ModifiedAt,
			}); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("dupe %s: %v", fi.Path, err))
			} else {
				result.Dupes++
			}
			continue
		}

		result.ToProcess = append(result.ToProcess, fi.ToSourceFile())
	}

	return result
}
