package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// FileInput represents a named file payload for batch requests.
type FileInput struct {
	Filename string
	Data     []byte
}

// buildMultipart builds a multipart/form-data body with a single file and optional fields.
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

// buildMultipartBatch builds a multipart/form-data body with multiple files
// under the same field name (e.g. "files"), matching FastAPI's list[UploadFile] = File(...).
func buildMultipartBatch(fieldName string, files []FileInput) ([]byte, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	for _, f := range files {
		part, err := mw.CreateFormFile(fieldName, f.Filename)
		if err != nil {
			return nil, "", fmt.Errorf("create form file %s: %w", f.Filename, err)
		}
		if _, err := part.Write(f.Data); err != nil {
			return nil, "", fmt.Errorf("write %s: %w", f.Filename, err)
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return buf.Bytes(), mw.FormDataContentType(), nil
}

// doJSON executes an HTTP request and JSON-decodes the response into dst.
// Returns an error if the status is not 200 OK.
func doJSON(httpClient *http.Client, req *http.Request, dst any) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}
