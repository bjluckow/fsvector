package api

import "time"

type ListRequest struct {
	Modality       string `json:"modality"`
	Ext            string `json:"ext"`
	Source         string `json:"source"`
	Since          string `json:"since"`
	Before         string `json:"before"`
	IncludeDeleted bool   `json:"deleted"`
	Limit          int    `json:"limit"`
	Page           int    `json:"page"`
}

type ListResponse struct {
	Files []FileItem `json:"files"`
}

type FileItem struct {
	Path        string     `json:"path"`
	Modality    string     `json:"modality"`
	Ext         string     `json:"ext"`
	Size        int64      `json:"size"`
	IndexedAt   time.Time  `json:"indexed_at"`
	ModifiedAt  *time.Time `json:"modified_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	IsDuplicate bool       `json:"is_duplicate"`
}

type FileDetail struct {
	Path          string     `json:"path"`
	Source        string     `json:"source"`
	CanonicalPath *string    `json:"canonical_path,omitempty"`
	ContentHash   string     `json:"content_hash"`
	Size          int64      `json:"size"`
	MimeType      string     `json:"mime_type"`
	Modality      string     `json:"modality"`
	Ext           string     `json:"ext"`
	EmbedModel    string     `json:"embed_model"`
	ChunkCount    int        `json:"chunk_count"`
	IndexedAt     time.Time  `json:"indexed_at"`
	ModifiedAt    *time.Time `json:"modified_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
}

type StatsResponse struct {
	Model      string `json:"model"`
	Total      int    `json:"total"`
	Text       int    `json:"text"`
	Image      int    `json:"image"`
	Audio      int    `json:"audio"`
	Video      int    `json:"video"`
	Deleted    int    `json:"deleted"`
	Duplicates int    `json:"duplicates"`
}
