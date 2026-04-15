package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type VisionClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewVisionClient(baseURL string) *VisionClient {
	return &VisionClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

type VisionHealth struct {
	Status       string `json:"status"`
	CaptionModel string `json:"caption_model"`
	OCR          bool   `json:"ocr"`
}

type CaptionResponse struct {
	Caption string `json:"caption"`
}

type OCRResponse struct {
	Text string `json:"text"`
}

func (c *VisionClient) Health(ctx context.Context) (*VisionHealth, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("visionsvc health: %w", err)
	}
	defer resp.Body.Close()

	var h VisionHealth
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("visionsvc health decode: %w", err)
	}
	return &h, nil
}

func (c *VisionClient) Caption(ctx context.Context, filename string, data []byte) (*CaptionResponse, error) {
	body, contentType, err := buildMultipart(filename, data, nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/caption", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("visionsvc caption: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("visionsvc caption: status %d: %s", resp.StatusCode, b)
	}

	var r CaptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("visionsvc caption decode: %w", err)
	}
	return &r, nil
}

func (c *VisionClient) OCR(ctx context.Context, filename string, data []byte) (*OCRResponse, error) {
	body, contentType, err := buildMultipart(filename, data, nil)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/ocr", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("visionsvc ocr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("visionsvc ocr: status %d: %s", resp.StatusCode, b)
	}

	var r OCRResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("visionsvc ocr decode: %w", err)
	}
	return &r, nil
}
