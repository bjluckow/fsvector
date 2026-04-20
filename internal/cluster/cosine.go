package cluster

import "strings"

// CosineSimilarity computes dot product of two vectors.
// CLIP embeddings are L2-normalized so dot product == cosine similarity.
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

// PathBoost returns an additive score boost if the file path
// contains any of the given signal tokens.
func PathBoost(path string, signals []string) float64 {
	lower := strings.ToLower(path)
	for _, signal := range signals {
		if strings.Contains(lower, strings.ToLower(signal)) {
			return 0.1
		}
	}
	return 0
}
