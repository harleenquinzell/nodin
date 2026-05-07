package state_test

import (
	"testing"
	"time"

	"github.com/harleenquinzell/nodin/internal/state"
)

func TestStateRoundTrip(t *testing.T) {
	s := state.Open(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	original := state.State{
		RootPageID: "3589c940-0284-81d3-b435-fcf079d89792",
		LastSync:   time.Now().UTC().Round(time.Second),
	}

	if err := s.WriteState(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.ReadState()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.RootPageID != original.RootPageID {
		t.Errorf("RootPageID: got %q, want %q", loaded.RootPageID, original.RootPageID)
	}
	if !loaded.LastSync.Equal(original.LastSync) {
		t.Errorf("LastSync: got %v, want %v", loaded.LastSync, original.LastSync)
	}
	if loaded.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d, want 1", loaded.SchemaVersion)
	}
}

func TestReadState_Missing(t *testing.T) {
	s := state.Open(t.TempDir())
	st, err := s.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	if !st.LastSync.IsZero() {
		t.Errorf("expected zero LastSync for missing state, got %v", st.LastSync)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := state.Open(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	id := "3589c940-0284-81d3-b435-fcf079d89792"
	content := "# Hello\n\nThis is the snapshot content.\n"

	if err := s.WriteSnapshot(id, content); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Errorf("snapshot mismatch:\ngot  %q\nwant %q", got, content)
	}
}

func TestReadSnapshot_Missing(t *testing.T) {
	s := state.Open(t.TempDir())
	got, err := s.ReadSnapshot("nonexistent-id")
	if err != nil {
		t.Fatalf("expected nil error for missing snapshot, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing snapshot, got %q", got)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	s := state.Open(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	id := "abc123"
	if err := s.WriteSnapshot(id, "content"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSnapshot(id); err != nil {
		t.Fatal(err)
	}

	got, err := s.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty after delete, got %q", got)
	}
}

func TestDeleteSnapshot_Missing(t *testing.T) {
	s := state.Open(t.TempDir())
	// Should not error even if snapshot never existed.
	if err := s.DeleteSnapshot("nonexistent"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
