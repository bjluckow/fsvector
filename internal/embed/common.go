package embed

import "time"

const (
	HotswapTimeout = 10 * time.Minute
)

type HealthResponse struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Dim    int    `json:"dim"`
}
