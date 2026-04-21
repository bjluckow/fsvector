package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type EmbedClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewEmbedClient(baseURL string, httpClient *http.Client) *EmbedClient {
	return &EmbedClient{
		BaseURL: baseURL,
		HTTP:    httpClient,
	}
}

type EmbedHealth struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Dim    int    `json:"dim"`
}

type textEmbedRequest struct {
	Texts []string `json:"texts"`
}

type textEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type imageEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

type imageBatchEmbedResponse struct {
	Embeddings []*[]float32 `json:"embeddings"` // nullable per-item
}

func (c *EmbedClient) Health(ctx context.Context) (*EmbedHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc health: %w", err)
	}
	defer resp.Body.Close()

	var h EmbedHealth
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("embedsvc health decode: %w", err)
	}
	return &h, nil
}

// EmbedDim returns the current embedding dimension from embedsvc.
func (c *EmbedClient) EmbedDim(ctx context.Context) (int, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return 0, fmt.Errorf("embed dim: %w", err)
	}
	return health.Dim, nil
}

// EmbedModel returns the current embedding model name.
func (c *EmbedClient) EmbedModel(ctx context.Context) (string, error) {
	health, err := c.Health(ctx)
	if err != nil {
		return "", fmt.Errorf("embed model: %w", err)
	}
	return health.Model, nil
}

// EmbedTexts returns one embedding vector per input string.
func (c *EmbedClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(textEmbedRequest{Texts: texts})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/embed/text", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedsvc text: status %d: %s", resp.StatusCode, b)
	}

	var r textEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedsvc text decode: %w", err)
	}
	return r.Embeddings, nil
}

// EmbedImage returns a single embedding vector for the provided image bytes.
func (c *EmbedClient) EmbedImage(ctx context.Context, filename string, data []byte) ([]float32, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/embed/image", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedsvc image: status %d: %s", resp.StatusCode, b)
	}

	var r imageEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedsvc image decode: %w", err)
	}
	return r.Embedding, nil
}

// EmbedImageBatch sends multiple images to /embed/image/batch and returns
// embeddings parallel to the input. Nil entries indicate per-image failures
// on the service side.
func (c *EmbedClient) EmbedImageBatch(ctx context.Context, images []FileInput) ([][]float32, error) {
	body, contentType, err := buildMultipartBatch("files", images)
	if err != nil {
		return nil, fmt.Errorf("embedsvc image batch: build multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/embed/image/batch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	var r imageBatchEmbedResponse
	if err := doJSON(c.HTTP, req, &r); err != nil {
		return nil, fmt.Errorf("embedsvc image batch: %w", err)
	}

	// convert []*[]float32 → [][]float32 (nil stays nil)
	out := make([][]float32, len(r.Embeddings))
	for i, v := range r.Embeddings {
		if v != nil {
			out[i] = *v
		}
	}
	return out, nil
}
