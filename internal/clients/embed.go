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

func NewEmbedClient(baseURL string) *EmbedClient {
	return &EmbedClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
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
