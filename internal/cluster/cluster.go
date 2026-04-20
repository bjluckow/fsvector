package cluster

import (
	"context"
	"fmt"
	"sort"

	"github.com/bjluckow/fsvector/pkg/api"
)

type CategoryMatch struct {
	Name  string
	Score float64
}

type FileResult struct {
	Row       api.ExportRow
	Matches   []CategoryMatch
	AllScores map[string]float64
}

func Run(ctx context.Context, client *api.Client, cf *CategoriesFile) ([]FileResult, error) {
	// 1. collect all unique labels
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

	// 4. group chunks by path, score each against all categories
	type pathScore struct {
		scores map[string]float64
		row    api.ExportRow
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
		if row.ChunkIndex == 0 {
			ps.row = row
		}

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

	// 5. assign categories
	results := make([]FileResult, 0, len(pathMap))
	for _, ps := range pathMap {
		result := FileResult{
			Row:       ps.row,
			AllScores: ps.scores,
		}

		for _, cat := range cf.Categories {
			score := ps.scores[cat.Name]
			if score >= cat.Threshold {
				result.Matches = append(result.Matches, CategoryMatch{
					Name:  cat.Name,
					Score: score,
				})
			}
		}

		sort.Slice(result.Matches, func(i, j int) bool {
			return result.Matches[i].Score > result.Matches[j].Score
		})

		if len(result.Matches) > cf.Global.TopCategories {
			result.Matches = result.Matches[:cf.Global.TopCategories]
		}

		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return topScore(results[i]) > topScore(results[j])
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
