package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
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

type Frame struct {
	Index       int
	TimestampMs int64
	Data        []byte
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

func (c *Client) ConvertToText(ctx context.Context, filename string, data []byte) ([]byte, error) {
	return c.postFile(ctx, "/convert/text", filename, data,
		map[string]string{"target_format": "txt"})
}

func (c *Client) ConvertToImage(ctx context.Context, filename string, data []byte) ([]byte, error) {
	return c.postFile(ctx, "/convert/image", filename, data,
		map[string]string{"target_format": "jpg"})
}

func (c *Client) NormalizeAudio(ctx context.Context, filename string, data []byte) ([]byte, error) {
	return c.postFile(ctx, "/convert/audio", filename, data, nil)
}

func (c *Client) ExtractVideoAudio(ctx context.Context, filename string, data []byte) ([]byte, error) {
	return c.postFile(ctx, "/convert/video/audio", filename, data, nil)
}

func (c *Client) ExtractVideoFrames(ctx context.Context, filename string, data []byte, fps float64) ([]Frame, error) {
	body, contentType, err := buildMultipart(filename, data,
		map[string]string{"fps": strconv.FormatFloat(fps, 'f', 2, 64)})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/convert/video/frames", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("convertsvc extract frames: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("convertsvc extract frames: status %d: %s", resp.StatusCode, b)
	}

	// parse multipart response
	mediaType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		return nil, fmt.Errorf("convertsvc extract frames: unexpected content-type: %s", mediaType)
	}

	mr := multipart.NewReader(resp.Body, params["boundary"])
	var frames []Frame
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("convertsvc extract frames: read part: %w", err)
		}

		frameData, err := io.ReadAll(part)
		if err != nil {
			return nil, err
		}

		index, _ := strconv.Atoi(part.Header.Get("X-Frame-Index"))
		tsMs, _ := strconv.ParseInt(part.Header.Get("X-Timestamp-Ms"), 10, 64)

		frames = append(frames, Frame{
			Index:       index,
			TimestampMs: tsMs,
			Data:        frameData,
		})
	}

	return frames, nil
}

// postFile is a helper for simple single-file-in/bytes-out requests.
func (c *Client) postFile(ctx context.Context, path, filename string, data []byte, fields map[string]string) ([]byte, error) {
	body, contentType, err := buildMultipart(filename, data, fields)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("convertsvc %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("convertsvc %s: status %d: %s", path, resp.StatusCode, b)
	}

	return io.ReadAll(resp.Body)
}

// buildMultipart builds a multipart/form-data body with a file and optional fields.
func buildMultipart(filename string, data []byte, fields map[string]string) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}

	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	mw.Close()

	return buf.Bytes(), mw.FormDataContentType(), nil
}
