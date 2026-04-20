 `internal/cluster/` is fscluster-specific — nothing in fsvector
imports it. It imports only `pkg/api` and stdlib.

---

## Categories File
```yaml
# categories.yaml

global:
  threshold: 0.40        # default threshold if not set per category
  top_categories: 2      # max categories a file can belong to
  uncategorized: true    # create uncategorized/ folder

categories:
  - name: roof damage
    threshold: 0.45
    labels:
      - roof damage
      - damaged roof
      - missing shingles
      - roof leak
      - storm damage
    path_signals:
      - roof
      - storm
      - damage

  - name: mold exposure
    threshold: 0.50
    labels:
      - mold
      - black mold
      - mildew
      - fungal growth
    path_signals:
      - mold
      - health

  - name: water intrusion
    threshold: 0.45
    labels:
      - water damage
      - water intrusion
      - flooding
      - leak
      - moisture
    path_signals:
      - water
      - flood
      - leak
```

---

## internal/cluster/categories.go
```go
package cluster

import (
	"os"

	"gopkg.in/yaml.v3"
)

type GlobalConfig struct {
	Threshold      float64 `yaml:"threshold"`
	TopCategories  int     `yaml:"top_categories"`
	Uncategorized  bool    `yaml:"uncategorized"`
}

type Category struct {
	Name        string   `yaml:"name"`
	Threshold   float64  `yaml:"threshold"`   // 0 = use global
	Labels      []string `yaml:"labels"`
	PathSignals []string `yaml:"path_signals"`
}

type CategoriesFile struct {
	Global     GlobalConfig `yaml:"global"`
	Categories []Category   `yaml:"categories"`
}

func LoadCategories(path string) (*CategoriesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf CategoriesFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	// apply defaults
	if cf.Global.Threshold == 0 {
		cf.Global.Threshold = 0.40
	}
	if cf.Global.TopCategories == 0 {
		cf.Global.TopCategories = 2
	}
	// apply global threshold to categories that don't have one
	for i := range cf.Categories {
		if cf.Categories[i].Threshold == 0 {
			cf.Categories[i].Threshold = cf.Global.Threshold
		}
	}
	return &cf, nil
}
```

---

## internal/cluster/cosine.go
```go
package cluster

import "math"

// CosineSimilarity computes the cosine similarity between two vectors.
// Assumes vectors are already L2-normalized (as CLIP outputs are).
// For normalized vectors, cosine similarity = dot product.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}

// PathBoost returns an additive score boost if the file path contains
// any of the given signal tokens.
func PathBoost(path string, signals []string) float64 {
	lower := strings.ToLower(path)
	for _, signal := range signals {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return 0.1
		}
	}
	return 0
}
```

---

## internal/cluster/cluster.go
```go
package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bjluckow/fsvector/pkg/api"
)

// CategoryMatch is a file's score for a single category.
type CategoryMatch struct {
	Name  string
	Score float64
}

// FileResult holds all category scores for a single file.
type FileResult struct {
	Row        api.ExportRow
	Matches    []CategoryMatch // sorted by score desc, filtered by threshold
	AllScores  map[string]float64 // raw scores for all categories (for CSV)
}

// Run performs zero-shot clustering against the daemon API.
func Run(ctx context.Context, client *api.Client, cf *CategoriesFile) ([]FileResult, error) {
	// 1. collect all unique labels across all categories
	allLabels := collectLabels(cf.Categories)
	fmt.Printf("  embedding %d labels across %d categories...\n",
		len(allLabels), len(cf.Categories))

	// 2. embed all labels in one request
	embedResp, err := client.EmbedText(ctx, allLabels)
	if err != nil {
		return nil, fmt.Errorf("embed labels: %w", err)
	}

	labelVectors := make(map[string][]float32, len(allLabels))
	for i, label := range allLabels {
		labelVectors[label] = embedResp.Embeddings[i]
	}

	// 3. stream all file embeddings
	fmt.Println("  streaming file embeddings...")
	var rows []api.ExportRow
	err = client.ExportStream(ctx, api.ListRequest{}, func(row api.ExportRow) error {
		if len(row.Embedding) > 0 {
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	fmt.Printf("  scoring %d chunks across %d categories...\n",
		len(rows), len(cf.Categories))

	// 4. score each file against each category
	// group chunks by path first — take max score per path per category
	type pathScore struct {
		scores map[string]float64 // category name -> best score
		row    api.ExportRow      // representative row (chunk_index=0 preferred)
	}
	pathMap := make(map[string]*pathScore)

	for _, row := range rows {
		ps, ok := pathMap[row.Path]
		if !ok {
			ps = &pathScore{
				scores: make(map[string]float64),
				row:    row,
			}
			pathMap[row.Path] = ps
		}
		// prefer chunk_index=0 as representative row
		if row.ChunkIndex == 0 {
			ps.row = row
		}

		// score this chunk against all categories
		for _, cat := range cf.Categories {
			best := ps.scores[cat.Name]
			for _, label := range cat.Labels {
				lv, ok := labelVectors[label]
				if !ok {
					continue
				}
				score := CosineSimilarity(row.Embedding, lv)
				score += PathBoost(row.Path, cat.PathSignals)
				if score > best {
					best = score
				}
			}
			ps.scores[cat.Name] = best
		}
	}

	// 5. assign categories to each file
	results := make([]FileResult, 0, len(pathMap))
	for _, ps := range pathMap {
		result := FileResult{
			Row:       ps.row,
			AllScores: ps.scores,
		}

		// collect matches above threshold
		for _, cat := range cf.Categories {
			score := ps.scores[cat.Name]
			if score >= cat.Threshold {
				result.Matches = append(result.Matches, CategoryMatch{
					Name:  cat.Name,
					Score: score,
				})
			}
		}

		// sort matches by score desc
		sort.Slice(result.Matches, func(i, j int) bool {
			return result.Matches[i].Score > result.Matches[j].Score
		})

		// cap at top_categories
		if len(result.Matches) > cf.Global.TopCategories {
			result.Matches = result.Matches[:cf.Global.TopCategories]
		}

		results = append(results, result)
	}

	// sort results by top score desc
	sort.Slice(results, func(i, j int) bool {
		si := topScore(results[i])
		sj := topScore(results[j])
		return si > sj
	})

	return results, nil
}

func collectLabels(categories []Category) []string {
	seen := make(map[string]bool)
	var labels []string
	for _, cat := range categories {
		for _, label := range cat.Labels {
			if !seen[label] {
				seen[label] = true
				labels = append(labels, label)
			}
		}
	}
	return labels
}

func topScore(r FileResult) float64 {
	if len(r.Matches) == 0 {
		return 0
	}
	return r.Matches[0].Score
}
```

---

## internal/cluster/output.go
```go
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
	Download bool   // download S3 files locally
	Modality string // filter output by modality
}

// Write writes clustering results to the output directory.
// Local files → symlinks
// S3 files    → manifest.json (+ download if --download)
func Write(results []FileResult, cf *CategoriesFile, cfg OutputConfig) error {
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return err
	}

	// create category directories
	categoryNames := make([]string, len(cf.Categories))
	for i, cat := range cf.Categories {
		categoryNames[i] = cat.Name
		dir := filepath.Join(cfg.Dir, slugify(cat.Name))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if cf.Global.Uncategorized {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, "uncategorized"), 0755); err != nil {
			return err
		}
	}

	// separate S3 and local files
	s3Manifests := make(map[string][]ManifestEntry) // category -> entries
	localFiles := make(map[string][]string)          // category -> paths

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
			entry := ManifestEntry{
				Path:     result.Row.Path,
				Modality: result.Row.Modality,
				Score:    match.Score,
			}

			if strings.HasPrefix(result.Row.Path, "s3://") {
				s3Manifests[catSlug] = append(s3Manifests[catSlug], entry)
			} else {
				localFiles[catSlug] = append(localFiles[catSlug], result.Row.Path)
			}
		}
	}

	// write local symlinks
	for catSlug, paths := range localFiles {
		dir := filepath.Join(cfg.Dir, catSlug)
		for _, path := range paths {
			linkName := filepath.Join(dir, filepath.Base(path))
			// handle duplicate filenames
			linkName = deduplicatePath(linkName)
			if err := os.Symlink(path, linkName); err != nil && !os.IsExist(err) {
				fmt.Fprintf(os.Stderr, "  symlink %s: %v\n", path, err)
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
		fmt.Printf("  wrote %s (%d files)\n", manifestPath, len(entries))
	}

	// write results.csv
	if err := writeCSV(results, cf, filepath.Join(cfg.Dir, "results.csv")); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}

	return nil
}

type ManifestEntry struct {
	Path     string  `json:"path"`
	Modality string  `json:"modality"`
	Score    float64 `json:"score"`
}

func writeCSV(results []FileResult, cf *CategoriesFile, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// header
	header := []string{"path", "modality", "top_category", "top_score", "uncategorized"}
	for _, cat := range cf.Categories {
		header = append(header, slugify(cat.Name)+"_score")
	}
	w.Write(header)

	// rows
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
			score := result.AllScores[cat.Name]
			row = append(row, fmt.Sprintf("%.4f", score))
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
```

---

## cmd/fscluster/main.go
```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bjluckow/fsvector/internal/cluster"
	"github.com/bjluckow/fsvector/pkg/api"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	categoriesFile string
	outputDir      string
	download       bool
	modality       string
	daemonHost     string
)

var rootCmd = &cobra.Command{
	Use:   "fscluster",
	Short: "Zero-shot file clustering using semantic similarity",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// load categories
		cf, err := cluster.LoadCategories(categoriesFile)
		if err != nil {
			return fmt.Errorf("load categories: %w", err)
		}
		fmt.Printf("loaded %d categories\n", len(cf.Categories))

		// create API client
		client := api.NewClient(viper.GetString("daemon.host"))

		// verify daemon is reachable
		if _, err := client.Health(ctx); err != nil {
			return fmt.Errorf("daemon unreachable at %s: %w",
				viper.GetString("daemon.host"), err)
		}

		// run clustering
		fmt.Println("clustering...")
		results, err := cluster.Run(ctx, client, cf)
		if err != nil {
			return fmt.Errorf("cluster: %w", err)
		}
		fmt.Printf("clustered %d files\n", len(results))

		// write output
		if err := cluster.Write(results, cf, cluster.OutputConfig{
			Dir:      outputDir,
			Download: download,
			Modality: modality,
		}); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		// print summary
		printSummary(results, cf)
		return nil
	},
}

func init() {
	rootCmd.Flags().StringVarP(&categoriesFile, "categories", "c",
		"categories.yaml", "path to categories YAML file")
	rootCmd.Flags().StringVarP(&outputDir, "output", "o",
		"sorted", "output directory")
	rootCmd.Flags().BoolVar(&download, "download", false,
		"download S3 files locally (warning: requires disk space)")
	rootCmd.Flags().StringVarP(&modality, "modality", "m", "",
		"filter by modality (text, image, audio, video)")
	rootCmd.Flags().StringVarP(&daemonHost, "host", "H",
		"http://localhost:8080", "fsvectord daemon address")

	viper.BindPFlag("daemon.host", rootCmd.Flags().Lookup("host"))
	viper.BindEnv("daemon.host", "DAEMON_HOST")
	viper.SetDefault("daemon.host", "http://localhost:8080")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func printSummary(results []cluster.FileResult, cf *cluster.CategoriesFile) {
	// count per category
	counts := make(map[string]int)
	uncategorized := 0
	for _, r := range results {
		if len(r.Matches) == 0 {
			uncategorized++
			continue
		}
		for _, m := range r.Matches {
			counts[m.Name]++
		}
	}

	fmt.Println("\n── summary ──────────────────────────────")
	for _, cat := range cf.Categories {
		fmt.Printf("  %-25s %d files\n", cat.Name, counts[cat.Name])
	}
	if cf.Global.Uncategorized {
		fmt.Printf("  %-25s %d files\n", "uncategorized", uncategorized)
	}
	fmt.Printf("  %-25s %d files\n", "total", len(results))
}
```

---

## Makefile

Add to existing Makefile:
```makefile
build:
	go build -o bin/fsvectord ./cmd/fsvectord
	go build -o bin/fsvector ./cmd/fsvector
	go build -o bin/fscluster ./cmd/fscluster
```

---

## Milestones

### M-fscluster.1 — core clustering
**Goal:** `fscluster` runs against local indexed files, assigns
categories, writes CSV.

**Verify:**
```bash
make build

# create test categories
cat > categories.yaml << EOF
global:
  threshold: 0.40
  top_categories: 2
  uncategorized: true
categories:
  - name: dogs
    labels:
      - dog
      - puppy
      - canine
  - name: documents
    labels:
      - document
      - resume
      - report
EOF

./bin/fscluster --categories categories.yaml --output sorted/
# → sorted/dogs/ contains dog.webp symlink
# → sorted/documents/ contains PDF symlinks
# → sorted/results.csv has full scoring matrix
```

---

### M-fscluster.2 — S3 manifests
**Goal:** S3 files produce manifest.json per category instead of
symlinks.

**Verify:**
```bash
# with S3 source indexed
./bin/fscluster --categories categories.yaml --output sorted/
# → sorted/dogs/manifest.json contains s3:// paths
# → cat sorted/dogs/manifest.json | jq '.[].path'
```

---

### M-fscluster.3 — path signals
**Goal:** Path tokens boost scores for matching categories.

**Verify:**
```bash
# files in a folder named "roof" should score higher for roof damage
# than identical files in a neutral folder
```

---

### M-fscluster.4 — download flag
**Goal:** `--download` fetches S3 files into category folders.

**Verify:**
```bash
./bin/fscluster --categories categories.yaml --output sorted/ --download
# → warns about disk usage
# → prompts for confirmation
# → downloads files into category folders
```

---

## What fscluster Does Not Include

- Re-ranking by RRF across categories — future
- Interactive threshold tuning UI — future
- Incremental clustering (only new files) — future
- Email thread clustering — emailsorter
- Export to case management software — future