package assets_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/harleenquinzell/nodin/internal/assets"
)

func TestDownload_NewFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("image data"))
	}))
	defer srv.Close()

	syncDir := t.TempDir()
	relPath, err := assets.Download(context.Background(), srv.Client(), srv.URL+"/photo.png", "block-id-123", syncDir)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join("assets", "block-id-123.png")
	if relPath != want {
		t.Errorf("relPath = %q, want %q", relPath, want)
	}

	data, err := os.ReadFile(filepath.Join(syncDir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "image data" {
		t.Errorf("file contents = %q, want %q", data, "image data")
	}
}

func TestDownload_ExistingFileSkipped(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		_, _ = w.Write([]byte("image data"))
	}))
	defer srv.Close()

	syncDir := t.TempDir()

	// First download.
	_, err := assets.Download(context.Background(), srv.Client(), srv.URL+"/photo.png", "block-id-456", syncDir)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}

	// Second download — should be skipped.
	_, err = assets.Download(context.Background(), srv.Client(), srv.URL+"/photo.png", "block-id-456", syncDir)
	if err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 HTTP call (idempotent), got %d", callCount)
	}
}

func TestDownload_BadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	syncDir := t.TempDir()
	_, err := assets.Download(context.Background(), srv.Client(), srv.URL+"/missing.png", "block-id-789", syncDir)
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}

	// No partial file should exist.
	if _, statErr := os.Stat(filepath.Join(syncDir, "assets", "block-id-789.png")); !os.IsNotExist(statErr) {
		t.Error("partial file should not exist after failed download")
	}
}
