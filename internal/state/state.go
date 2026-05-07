package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrCorrupt is returned when state files contain invalid data.
var ErrCorrupt = errors.New("state: corrupt data")

const (
	currentSchemaVersion = 1
	stateFileName        = "state.json"
	nodinDir             = ".nodin"
	snapshotsDir         = "snapshots"
)

// State is the persisted state of a sync directory.
type State struct {
	SchemaVersion int       `json:"schema_version"`
	RootPageID    string    `json:"root_page_id"`
	LastSync      time.Time `json:"last_sync,omitempty"`
}

// Store manages the .nodin/ directory inside a sync root.
type Store struct {
	syncDir string
	mu      sync.Mutex // guards index RMW from concurrent workers
}

// Open returns a Store for the given sync directory.
// It does not create .nodin/ yet; call Init for that.
func Open(syncDir string) *Store {
	return &Store{syncDir: syncDir}
}

// Init creates the .nodin/ directory and its required subdirectories.
// Safe to call on an already-initialised directory.
func (s *Store) Init() error {
	dirs := []string{
		filepath.Join(s.syncDir, nodinDir),
		filepath.Join(s.syncDir, nodinDir, snapshotsDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("init .nodin: %w", err)
		}
	}
	return nil
}

// ReadState returns the current state from .nodin/state.json.
// Returns a zero State (not an error) if the file does not exist yet.
func (s *Store) ReadState() (State, error) {
	path := s.statePath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{SchemaVersion: currentSchemaVersion}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	return st, nil
}

// WriteState atomically writes st to .nodin/state.json.
func (s *Store) WriteState(st State) error {
	st.SchemaVersion = currentSchemaVersion
	return writeJSON(s.statePath(), st)
}

// ReadSnapshot returns the content of .nodin/snapshots/<notionID>.md.
// Returns ("", nil) if the snapshot does not exist.
func (s *Store) ReadSnapshot(notionID string) (string, error) {
	data, err := os.ReadFile(s.snapshotPath(notionID))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read snapshot %s: %w", notionID, err)
	}
	return string(data), nil
}

// WriteSnapshot atomically writes content to .nodin/snapshots/<notionID>.md.
func (s *Store) WriteSnapshot(notionID, content string) error {
	return writeFile(s.snapshotPath(notionID), []byte(content), 0644)
}

// DeleteSnapshot removes .nodin/snapshots/<notionID>.md.
// Not an error if the file doesn't exist.
func (s *Store) DeleteSnapshot(notionID string) error {
	err := os.Remove(s.snapshotPath(notionID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) statePath() string {
	return filepath.Join(s.syncDir, nodinDir, stateFileName)
}

func (s *Store) snapshotPath(notionID string) string {
	return filepath.Join(s.syncDir, nodinDir, snapshotsDir, notionID+".md")
}

// writeJSON atomically writes v as indented JSON to path.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return writeFile(path, append(data, '\n'), 0644)
}

// writeFile atomically writes data to path using a temp-file + rename.
func writeFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}
