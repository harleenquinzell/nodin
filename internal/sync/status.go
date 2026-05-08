package sync

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/state"
)

// FileStatus is the sync state of a tracked file.
type FileStatus int

const (
	FileClean      FileStatus = iota // local matches last-synced checksum
	FileModified                     // local differs from last-synced checksum
	FileConflicted                   // local contains unresolved conflict markers
	FileDeleted                      // local file no longer exists
)

func (s FileStatus) String() string {
	switch s {
	case FileModified:
		return "modified"
	case FileConflicted:
		return "conflicted"
	case FileDeleted:
		return "deleted"
	default:
		return "clean"
	}
}

// StatusEntry holds the status of one tracked file.
type StatusEntry struct {
	LocalPath string
	NotionID  string
	Status    FileStatus
}

// Status returns the sync status of all tracked pages in the index.
// Entries are sorted by LocalPath.
func Status(cfg *config.Config, store *state.Store) ([]StatusEntry, error) {
	idx, err := store.ReadIndex()
	if err != nil {
		return nil, err
	}

	entries := make([]StatusEntry, 0, len(idx))
	for notionID, entry := range idx {
		if entry.Type != "page" {
			continue
		}
		absPath := filepath.Join(cfg.SyncDir, entry.LocalPath)
		data, readErr := os.ReadFile(absPath)

		var status FileStatus
		switch {
		case readErr != nil:
			status = FileDeleted
		case strings.Contains(string(data), "<<<<<<<"):
			status = FileConflicted
		case checksum(string(data)) != entry.Checksum:
			status = FileModified
		default:
			status = FileClean
		}

		entries = append(entries, StatusEntry{
			LocalPath: entry.LocalPath,
			NotionID:  notionID,
			Status:    status,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].LocalPath < entries[j].LocalPath
	})

	return entries, nil
}
