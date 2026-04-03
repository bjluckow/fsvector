package api

import "time"

type SearchRequest struct {
	Query          string  `json:"query"`
	Mode           string  `json:"mode"`            // hybrid | vector | fulltext
	SemanticWeight float64 `json:"semantic_weight"` // 0 = use server default
	FTSWeight      float64 `json:"fts_weight"`
	Modality       string  `json:"modality"`
	Ext            string  `json:"ext"`
	Source         string  `json:"source"`
	Since          string  `json:"since"`
	Before         string  `json:"before"`
	MinSize        string  `json:"min_size"`
	MaxSize        string  `json:"max_size"`
	MinScore       float64 `json:"min_score"`
	Limit          int     `json:"limit"`
	Page           int     `json:"page"`
}

type SearchResult struct {
	Path       string     `json:"path"`
	Modality   string     `json:"modality"`
	Ext        string     `json:"ext"`
	Size       int64      `json:"size"`
	Score      float64    `json:"score"`
	NormScore  float64    `json:"norm_score"`
	IndexedAt  time.Time  `json:"indexed_at"`
	ModifiedAt *time.Time `json:"modified_at"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
}
