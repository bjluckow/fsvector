package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type ImageClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewImageClient(baseURL string) *ImageClient {
	return &ImageClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

type imageEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (c *ImageClient) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc-image health: %w", err)
	}
	defer resp.Body.Close()

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("embedsvc-image health decode: %w", err)
	}
	return &h, nil
}

func (c *ImageClient) EmbedImage(ctx context.Context, filename string, data []byte) ([]float32, error) {
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
		return nil, fmt.Errorf("embedsvc-image embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("embedsvc-image is loading a new model, try again shortly")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedsvc-image: status %d: %s", resp.StatusCode, b)
	}

	var r imageEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("embedsvc-image decode: %w", err)
	}
	return r.Embedding, nil
}

func (c *ImageClient) SwapModel(ctx context.Context, model string) (*HealthResponse, error) {
	// use a long-lived client for model swaps — loading can take several minutes
	swapClient := &http.Client{Timeout: HotswapTimeout}

	body, err := json.Marshal(ModelSwapRequest{Model: model})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/model", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := swapClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedsvc-image swap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedsvc-image swap: status %d: %s", resp.StatusCode, b)
	}

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("embedsvc-image swap decode: %w", err)
	}
	return &h, nil
}
