package assets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var ErrDownloadFailed = errors.New("asset download failed")

// Download fetches the asset at url and writes it to <syncDir>/assets/<fileID><ext>.
// Returns the local path relative to syncDir. Idempotent: skips if the file already exists.
func Download(ctx context.Context, client *http.Client, url, fileID, syncDir string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}

	ext := guessExtension(url)
	fileName := fileID + ext
	assetsDir := filepath.Join(syncDir, "assets")
	localPath := filepath.Join(assetsDir, fileName)
	relPath := filepath.Join("assets", fileName)

	// Idempotent: skip if file already exists.
	if _, err := os.Stat(localPath); err == nil {
		return relPath, nil
	}

	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("build asset request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%w: HTTP %d for %s", ErrDownloadFailed, resp.StatusCode, url)
	}

	// Write to a temp file, then rename atomically.
	tmp := localPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", fmt.Errorf("create temp asset file: %w", err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write asset: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close asset file: %w", err)
	}
	if err := os.Rename(tmp, localPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename asset: %w", err)
	}

	return relPath, nil
}

// guessExtension returns a file extension from a URL (e.g. ".png").
func guessExtension(url string) string {
	// Strip query string.
	if idx := strings.Index(url, "?"); idx >= 0 {
		url = url[:idx]
	}
	if idx := strings.LastIndex(url, "."); idx >= 0 {
		ext := url[idx:]
		if len(ext) <= 5 && !strings.Contains(ext, "/") {
			return ext
		}
	}
	return ""
}
