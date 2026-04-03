package search

type SearchMode string

const (
	SearchModeHybrid   SearchMode = "hybrid"
	SearchModeVector   SearchMode = "vector"
	SearchModeFullText SearchMode = "fulltext"
)

// SearchConfig holds tunable search parameters.
// Loaded once from config, optionally overridden per-request.
type SearchConfig struct {
	FTSWeight   float64
	FTSScale    float64
	FTSMinBoost float64
	DefaultMode SearchMode
}

func (c SearchConfig) SemanticWeight() float64 {
	return 1.0 - c.FTSWeight
}

var DefaultSearchConfig = SearchConfig{
	FTSWeight:   0.5,
	FTSScale:    10.0,
	FTSMinBoost: 0.3,
	DefaultMode: SearchModeHybrid,
}
