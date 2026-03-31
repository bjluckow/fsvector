package embed

type HealthResponse struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Dim    int    `json:"dim"`
}
