package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
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

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var r HealthResponse
	if err := c.getJSON(ctx, "/health", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var r StatusResponse
	if err := c.getJSON(ctx, "/status", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) Reindex(ctx context.Context) (*ReindexResponse, error) {
	var r ReindexResponse
	if err := c.postJSON(ctx, "/reindex", nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	var r SearchResponse
	if err := c.postJSON(ctx, "/search", req, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) ListFiles(ctx context.Context, req ListRequest) (*ListResponse, error) {
	params := url.Values{}
	if req.Modality != "" {
		params.Set("modality", req.Modality)
	}
	if req.Ext != "" {
		params.Set("ext", req.Ext)
	}
	if req.Source != "" {
		params.Set("source", req.Source)
	}
	if req.Since != "" {
		params.Set("since", req.Since)
	}
	if req.Before != "" {
		params.Set("before", req.Before)
	}
	if req.IncludeDeleted {
		params.Set("deleted", "true")
	}
	if req.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", req.Limit))
	}
	if req.Page > 0 {
		params.Set("page", fmt.Sprintf("%d", req.Page))
	}

	path := "/files"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var r ListResponse
	if err := c.getJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) ShowFile(ctx context.Context, filePath string) (*FileDetail, error) {
	params := url.Values{}
	params.Set("path", filePath)
	var r FileDetail
	if err := c.getJSON(ctx, "/files?"+params.Encode(), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	var r StatsResponse
	if err := c.getJSON(ctx, "/stats", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) EmbedText(ctx context.Context, texts []string) (*EmbedTextResponse, error) {
	var r EmbedTextResponse
	if err := c.postJSON(ctx, "/embed/text", EmbedTextRequest{Texts: texts}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) EmbedImage(ctx context.Context, filename string, data []byte) (*EmbedImageResponse, error) {
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
		return nil, fmt.Errorf("embed image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed image: status %d: %s", resp.StatusCode, b)
	}

	var r EmbedImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (c *Client) ExportStream(ctx context.Context, req ListRequest, fn func(ExportRow) error) error {
	params := url.Values{}
	if req.Modality != "" {
		params.Set("modality", req.Modality)
	}
	if req.Ext != "" {
		params.Set("ext", req.Ext)
	}
	if req.Source != "" {
		params.Set("source", req.Source)
	}
	if req.IncludeDeleted {
		params.Set("deleted", "true")
	}

	path := "/export/files"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("export: status %d: %s", resp.StatusCode, b)
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var row ExportRow
		if err := dec.Decode(&row); err != nil {
			return fmt.Errorf("decode export row: %w", err)
		}
		if err := fn(row); err != nil {
			return err
		}
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: status %d: %s", path, resp.StatusCode, b)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
