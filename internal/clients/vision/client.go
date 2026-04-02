package vision

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

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("visionsvc health: %w", err)
	}
	defer resp.Body.Close()

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, fmt.Errorf("visionsvc health decode: %w", err)
	}
	return &h, nil
}

func (c *Client) Caption(ctx context.Context, filename string, data []byte) (*CaptionResponse, error) {
	body, contentType, err := buildMultipart(filename, data)
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

func (c *Client) OCR(ctx context.Context, filename string, data []byte) (*OCRResponse, error) {
	body, contentType, err := buildMultipart(filename, data)
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

func buildMultipart(filename string, data []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	mw.Close()

	return buf.Bytes(), mw.FormDataContentType(), nil
}
