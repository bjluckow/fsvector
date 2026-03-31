package search

// Normalize computes min-max normalized scores within each modality group
// and populates the NormScore field on each result.
//
// A NormScore of 1.0 means this result scored highest within its modality.
// A NormScore of 0.0 means it scored lowest within its modality.
//
// If a modality has only one result, its NormScore is set to 1.0.
func Normalize(results []SearchResult) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// find min and max score per modality
	type minMax struct {
		min, max float64
		count    int
	}
	stats := map[string]*minMax{}

	for _, r := range results {
		if s, ok := stats[r.Modality]; ok {
			if r.Score < s.min {
				s.min = r.Score
			}
			if r.Score > s.max {
				s.max = r.Score
			}
			s.count++
		} else {
			stats[r.Modality] = &minMax{min: r.Score, max: r.Score, count: 1}
		}
	}

	// apply normalization
	for i, r := range results {
		s := stats[r.Modality]
		if s.count == 1 || s.max == s.min {
			results[i].NormScore = 1.0
			continue
		}
		results[i].NormScore = (r.Score - s.min) / (s.max - s.min)
	}

	return results
}