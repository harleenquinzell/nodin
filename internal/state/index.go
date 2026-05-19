package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IndexEntry tracks one synced Notion object (page, database, or database entry).
type IndexEntry struct {
	NotionID  string    `json:"notion_id"`
	LocalPath string    `json:"local_path"`
	Checksum  string    `json:"checksum"` // sha256 hex of file at last sync
	Type      string    `json:"type"`     // "page" | "database" | "database_entry"
	LastSync  time.Time `json:"last_sync"`
}

// indexPath returns the path to the index.json file.
func (s *Store) indexPath() string {
	return filepath.Join(s.syncDir, ".nodin", "index.json")
}

// ReadIndex loads the index from disk. Returns an empty map if the file doesn't exist.
func (s *Store) ReadIndex() (map[string]IndexEntry, error) {
	data, err := os.ReadFile(s.indexPath())
	if os.IsNotExist(err) {
		return map[string]IndexEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var idx map[string]IndexEntry
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("%w: index: %v", ErrCorrupt, err)
	}
	if idx == nil {
		idx = map[string]IndexEntry{}
	}
	return idx, nil
}

// WriteIndex atomically writes the index to disk.
func (s *Store) WriteIndex(idx map[string]IndexEntry) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	return writeFile(s.indexPath(), data, 0644)
}

// DeleteEntry removes a single entry from the index under the mutex.
func (s *Store) DeleteEntry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.ReadIndex()
	if err != nil {
		return fmt.Errorf("delete entry %s: %w", id, err)
	}
	delete(idx, id)
	return s.WriteIndex(idx)
}

// UpdateEntry performs a read-modify-write on a single index entry under the mutex.
// fn receives the current entry (zero-value if absent) and returns the updated entry.
func (s *Store) UpdateEntry(id string, fn func(IndexEntry) IndexEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.ReadIndex()
	if err != nil {
		return fmt.Errorf("update entry %s: %w", id, err)
	}

	idx[id] = fn(idx[id])

	if err := s.WriteIndex(idx); err != nil {
		return fmt.Errorf("update entry %s: %w", id, err)
	}
	return nil
}
