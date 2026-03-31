package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type TextClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewTextClient(baseURL string) *TextClient {
	return &TextClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

type textEmbedRequest struct {
	Texts []string `json:"texts"`
}

type textEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type ModelSwapRequest struct {
	Model string `json:"model"`
}

func (c *TextClient) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc-text health: %w", err)
	}
	defer resp.Body.Close()

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("embedsvc-text health decode: %w", err)
	}
	return &h, nil
}

func (c *TextClient) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
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
		return nil, fmt.Errorf("embedsvc-text embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("embedsvc-text is loading a new model, try again shortly")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedsvc-text: status %d: %s", resp.StatusCode, b)
	}

	var r textEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedsvc-text decode: %w", err)
	}
	return r.Embeddings, nil
}
