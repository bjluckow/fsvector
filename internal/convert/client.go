package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

type HealthResponse struct {
	Status   string   `json:"status"`
	Backends []string `json:"backends"`
}

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("convertsvc health: %w", err)
	}
	defer resp.Body.Close()

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("convertsvc health decode: %w", err)
	}
	return &h, nil
}

// Convert sends a file to convertsvc and returns the converted bytes.
func (c *Client) Convert(ctx context.Context, filename string, data []byte, targetFormat string) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := mw.WriteField("target_format", targetFormat); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/convert", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("convertsvc convert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("convertsvc convert: status %d: %s", resp.StatusCode, b)
	}

	return io.ReadAll(resp.Body)
}

// errorResponse is used to decode error details from convertsvc
type errorResponse struct {
	Detail string `json:"detail"`
}

func parseError(b []byte) string {
	var e errorResponse
	if err := json.Unmarshal(b, &e); err == nil && e.Detail != "" {
		return e.Detail
	}
	return string(b)
}
