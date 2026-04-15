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

type TranscribeClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewTranscribeClient(baseURL string) *TranscribeClient {
	return &TranscribeClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

type TranscribeHealth struct {
	Status   string `json:"status"`
	Model    string `json:"model"`
	Language string `json:"language"`
}

type TranscribeResponse struct {
	Text            string  `json:"text"`
	Language        string  `json:"language"`
	DurationSeconds float64 `json:"duration_seconds"`
}

func (c *TranscribeClient) Health(ctx context.Context) (*TranscribeHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcribesvc health: %w", err)
	}
	defer resp.Body.Close()

	var h TranscribeHealth
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("transcribesvc health decode: %w", err)
	}
	return &h, nil
}

func (c *TranscribeClient) Transcribe(ctx context.Context, filename string, data []byte) (*TranscribeResponse, error) {
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
		c.BaseURL+"/transcribe", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transcribesvc transcribe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("transcribesvc transcribe: status %d: %s", resp.StatusCode, b)
	}

	var r TranscribeResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("transcribesvc transcribe decode: %w", err)
	}
	return &r, nil
}
