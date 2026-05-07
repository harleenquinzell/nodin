package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

const (
	singlePartMaxBytes int64 = 5 * 1024 * 1024 // 5 MiB
	chunkSize          int64 = 5 * 1024 * 1024
)

// UploadFile uploads r to Notion's file upload API and returns the upload ID.
// Files <= 5 MiB use single-part; larger files use multi-part.
// The returned ID can be referenced in block content as a file_upload.
func (c *Client) UploadFile(ctx context.Context, name, mimeType string, r io.Reader, size int64) (string, error) {
	if size <= singlePartMaxBytes {
		return c.uploadSinglePart(ctx, name, mimeType, r)
	}
	return c.uploadMultiPart(ctx, name, mimeType, r, size)
}

type fileUploadInit struct {
	ID        string `json:"id"`
	UploadURL string `json:"upload_url"`
}

func (c *Client) initFileUpload(ctx context.Context, name, mimeType, mode string, numParts int) (*fileUploadInit, error) {
	body := map[string]any{
		"filename":     name,
		"content_type": mimeType,
		"mode":         mode,
	}
	if numParts > 0 {
		body["number_of_parts"] = numParts
	}
	data, err := c.do(ctx, "POST", "/file_uploads", body)
	if err != nil {
		return nil, fmt.Errorf("init file upload: %w", err)
	}
	var resp fileUploadInit
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse file upload init: %w", err)
	}
	return &resp, nil
}

func (c *Client) uploadSinglePart(ctx context.Context, name, mimeType string, r io.Reader) (string, error) {
	resp, err := c.initFileUpload(ctx, name, mimeType, "single_part", 0)
	if err != nil {
		return "", err
	}

	if _, err := sendMultipartChunk(ctx, resp.UploadURL, name, r); err != nil {
		return "", fmt.Errorf("single-part upload: %w", err)
	}
	return resp.ID, nil
}

func (c *Client) uploadMultiPart(ctx context.Context, name, mimeType string, r io.Reader, size int64) (string, error) {
	numParts := int((size + chunkSize - 1) / chunkSize)
	resp, err := c.initFileUpload(ctx, name, mimeType, "multi_part", numParts)
	if err != nil {
		return "", err
	}

	for part := 1; part <= numParts; part++ {
		chunk := make([]byte, chunkSize)
		n, readErr := io.ReadFull(r, chunk)
		if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
			return "", fmt.Errorf("read chunk %d: %w", part, readErr)
		}

		sendURL := fmt.Sprintf("%s/send?part_number=%d", resp.UploadURL, part)
		if _, err := sendMultipartChunk(ctx, sendURL, name, bytes.NewReader(chunk[:n])); err != nil {
			return "", fmt.Errorf("send chunk %d: %w", part, err)
		}

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	if _, err := c.do(ctx, "POST", "/file_uploads/"+resp.ID+"/complete", nil); err != nil {
		return "", fmt.Errorf("complete upload: %w", err)
	}
	return resp.ID, nil
}

// sendMultipartChunk POSTs r to url as multipart/form-data with field "file".
// The upload URL is a presigned endpoint (not the Notion API), so no auth header is added.
func sendMultipartChunk(ctx context.Context, url, filename string, r io.Reader) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, r); err != nil {
		return "", err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()
	body, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", httpResp.StatusCode, body)
	}
	return string(body), nil
}
