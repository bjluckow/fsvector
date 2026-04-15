package cluster

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type OutputConfig struct {
	Dir      string
	Download bool
	Modality string
}

type ManifestEntry struct {
	Path     string  `json:"path"`
	Modality string  `json:"modality"`
	Score    float64 `json:"score"`
}

func Write(results []FileResult, cf *CategoriesFile, cfg OutputConfig) error {
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return err
	}

	// create category dirs
	for _, cat := range cf.Categories {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, slugify(cat.Name)), 0755); err != nil {
			return err
		}
	}
	if cf.Global.Uncategorized {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, "uncategorized"), 0755); err != nil {
			return err
		}
	}

	s3Manifests := make(map[string][]ManifestEntry)

	for _, result := range results {
		if cfg.Modality != "" && result.Row.Modality != cfg.Modality {
			continue
		}

		categories := result.Matches
		if len(categories) == 0 {
			if !cf.Global.Uncategorized {
				continue
			}
			categories = []CategoryMatch{{Name: "uncategorized", Score: 0}}
		}

		for _, match := range categories {
			catSlug := slugify(match.Name)

			if strings.HasPrefix(result.Row.Path, "s3://") {
				s3Manifests[catSlug] = append(s3Manifests[catSlug], ManifestEntry{
					Path:     result.Row.Path,
					Modality: result.Row.Modality,
					Score:    match.Score,
				})
			} else {
				dir := filepath.Join(cfg.Dir, catSlug)
				linkName := deduplicatePath(filepath.Join(dir, filepath.Base(result.Row.Path)))
				if err := os.Symlink(result.Row.Path, linkName); err != nil && !os.IsExist(err) {
					fmt.Fprintf(os.Stderr, "  symlink %s: %v\n", result.Row.Path, err)
				}
			}
		}
	}

	// write S3 manifests
	for catSlug, entries := range s3Manifests {
		manifestPath := filepath.Join(cfg.Dir, catSlug, "manifest.json")
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(manifestPath, data, 0644); err != nil {
			return err
		}
		fmt.Printf("  %s: %d files\n", catSlug, len(entries))
	}

	// write results.csv
	return writeCSV(results, cf, filepath.Join(cfg.Dir, "results.csv"))
}

func writeCSV(results []FileResult, cf *CategoriesFile, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"path", "modality", "top_category", "top_score", "uncategorized"}
	for _, cat := range cf.Categories {
		header = append(header, slugify(cat.Name)+"_score")
	}
	w.Write(header)

	for _, result := range results {
		topCat := ""
		topScore := 0.0
		uncategorized := len(result.Matches) == 0
		if len(result.Matches) > 0 {
			topCat = result.Matches[0].Name
			topScore = result.Matches[0].Score
		}

		row := []string{
			result.Row.Path,
			result.Row.Modality,
			topCat,
			fmt.Sprintf("%.4f", topScore),
			fmt.Sprintf("%v", uncategorized),
		}
		for _, cat := range cf.Categories {
			row = append(row, fmt.Sprintf("%.4f", result.AllScores[cat.Name]))
		}
		w.Write(row)
	}
	return nil
}

func slugify(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}

func deduplicatePath(path string) string {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
