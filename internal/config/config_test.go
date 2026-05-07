package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/harleenquinzell/nodin/internal/config"
)

func TestDefaults(t *testing.T) {
	c, err := config.Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if c.RPS != 3 {
		t.Errorf("RPS = %d, want 3", c.RPS)
	}
	if c.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want 4", c.Concurrency)
	}
	if !c.AutoCommit {
		t.Error("AutoCommit = false, want true")
	}
	if !c.DownloadAssets {
		t.Error("DownloadAssets = false, want true")
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	original := &config.Config{
		Token:          "secret_test_abc123",
		RootPageID:     "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:        dir,
		RPS:            5,
		Concurrency:    8,
		AutoCommit:     false,
		DownloadAssets: false,
	}

	if err := config.Write(path, original); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Token != original.Token {
		t.Errorf("Token: got %q, want %q", loaded.Token, original.Token)
	}
	if loaded.RootPageID != original.RootPageID {
		t.Errorf("RootPageID: got %q, want %q", loaded.RootPageID, original.RootPageID)
	}
	if loaded.RPS != original.RPS {
		t.Errorf("RPS: got %d, want %d", loaded.RPS, original.RPS)
	}
	if loaded.Concurrency != original.Concurrency {
		t.Errorf("Concurrency: got %d, want %d", loaded.Concurrency, original.Concurrency)
	}
	if loaded.AutoCommit != original.AutoCommit {
		t.Errorf("AutoCommit: got %v, want %v", loaded.AutoCommit, original.AutoCommit)
	}
	if loaded.DownloadAssets != original.DownloadAssets {
		t.Errorf("DownloadAssets: got %v, want %v", loaded.DownloadAssets, original.DownloadAssets)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := config.Write(path, &config.Config{
		Token:          "secret_from_file",
		RootPageID:     "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:        dir,
		RPS:            3,
		Concurrency:    4,
		AutoCommit:     true,
		DownloadAssets: true,
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NODIN_TOKEN", "secret_from_env")

	c, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Token != "secret_from_env" {
		t.Errorf("got %q, want env token", c.Token)
	}
}

func TestTokenFile(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.txt")
	if err := os.WriteFile(tokenPath, []byte("secret_from_file\n"), 0600); err != nil {
		t.Fatal(err)
	}

	c := &config.Config{
		TokenFile:   tokenPath,
		RootPageID:  "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:     dir,
		RPS:         3,
		Concurrency: 1,
		AutoCommit:  true,
	}

	token, err := c.ResolvedToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "secret_from_file" {
		t.Errorf("got %q, want %q", token, "secret_from_file")
	}
}

func TestValidate_MissingToken(t *testing.T) {
	c := &config.Config{
		RootPageID:  "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:     t.TempDir(),
		RPS:         3,
		Concurrency: 1,
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for missing token, got nil")
	}
}

func TestValidate_InvalidUUID(t *testing.T) {
	c := &config.Config{
		Token:       "secret_xxx",
		RootPageID:  "not-a-uuid",
		SyncDir:     t.TempDir(),
		RPS:         3,
		Concurrency: 1,
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for invalid UUID, got nil")
	}
}

func TestValidate_RPSZero(t *testing.T) {
	c := &config.Config{
		Token:       "secret_xxx",
		RootPageID:  "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:     t.TempDir(),
		RPS:         0,
		Concurrency: 1,
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for RPS=0, got nil")
	}
}

func TestValidate_RelativeSyncDir(t *testing.T) {
	c := &config.Config{
		Token:       "secret_xxx",
		RootPageID:  "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:     "relative/path",
		RPS:         3,
		Concurrency: 1,
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for relative sync_dir, got nil")
	}
}

func TestConfigFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := config.Write(path, &config.Config{
		Token:       "secret_test",
		RootPageID:  "3589c940-0284-81d3-b435-fcf079d89792",
		SyncDir:     dir,
		RPS:         3,
		Concurrency: 4,
		AutoCommit:  true,
	}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("config file permissions = %o, want 0600", perm)
	}
}
