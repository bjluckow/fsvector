package clients

import (
	"bytes"
	"mime/multipart"
)

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
