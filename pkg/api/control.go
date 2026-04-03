package api

import "time"

type HealthResponse struct {
	Status string `json:"status"`
}

type ReindexResponse struct {
	Status string `json:"status"`
}

type ProgressSnapshot struct {
	Running    bool       `json:"running"`
	Total      int        `json:"total"`
	Indexed    int        `json:"indexed"`
	Deleted    int        `json:"deleted"`
	Skipped    int        `json:"skipped"`
	Errors     []string   `json:"errors"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type StatusResponse struct {
	Status    string           `json:"status"`
	Source    string           `json:"source"`
	StartedAt time.Time        `json:"started_at"`
	Reindex   ProgressSnapshot `json:"reindex"`
}
