package search

func Normalize(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	min, max := results[0].Score, results[0].Score
	for _, r := range results {
		if r.Score < min {
			min = r.Score
		}
		if r.Score > max {
			max = r.Score
		}
	}

	for i, r := range results {
		if max == min {
			results[i].NormScore = 1.0
			continue
		}
		results[i].NormScore = (r.Score - min) / (max - min)
	}

	return results
}
