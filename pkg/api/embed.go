package api

type EmbedTextRequest struct {
	Texts []string `json:"texts"`
}

type EmbedTextResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type EmbedImageResponse struct {
	Embedding []float32 `json:"embedding"`
}
