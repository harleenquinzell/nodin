package assets

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// Upload uploads the file at localPath to Notion and returns the file upload ID.
// The ID can be embedded in a block's content as a file_upload reference.
func Upload(ctx context.Context, client *notion.Client, localPath string) (string, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", localPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", localPath, err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(localPath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return client.UploadFile(ctx, filepath.Base(localPath), mimeType, f, info.Size())
}
