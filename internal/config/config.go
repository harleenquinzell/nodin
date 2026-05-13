package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds all nodin configuration. Loading order: defaults → file → env vars.
type Config struct {
	Token          string
	TokenFile      string
	RootPageID     string
	SyncDir        string
	RPS            int
	Concurrency    int
	AutoCommit     bool
	DownloadAssets bool
}

// toml struct uses *bool so absent fields don't override defaults.
type tomlAuth struct {
	Token     string `toml:"token"`
	TokenFile string `toml:"token_file"`
}

type tomlSync struct {
	RootPageID     string `toml:"root_page_id"`
	SyncDir        string `toml:"sync_dir"`
	RPS            int    `toml:"rate_limit_rps"`
	Concurrency    int    `toml:"concurrency"`
	AutoCommit     *bool  `toml:"auto_commit"`
	DownloadAssets *bool  `toml:"download_assets"`
}

type tomlFile struct {
	Auth tomlAuth `toml:"auth"`
	Sync tomlSync `toml:"sync"`
}

// LocalConfigName is the filename nodin looks for when discovering a workspace config.
const LocalConfigName = ".nodin.toml"

// DefaultPath returns ~/.config/nodin/config.toml.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(home, ".config", "nodin", "config.toml")
}

// Discover walks up from the current directory looking for a .nodin.toml file.
// Falls back to DefaultPath if none is found.
func Discover() string {
	cwd, err := os.Getwd()
	if err != nil {
		return DefaultPath()
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, LocalConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
	return DefaultPath()
}

// Load reads config from path (empty → Discover) then applies env overrides.
// Missing file is not an error; callers should call Validate before use.
// When a local .nodin.toml is discovered and sync_dir is not set, it defaults
// to the directory containing the config file.
func Load(path string) (*Config, error) {
	local := path == ""
	if path == "" {
		path = Discover()
	}

	c := &Config{
		RPS:            3,
		Concurrency:    4,
		AutoCommit:     true,
		DownloadAssets: true,
	}

	if _, err := os.Stat(path); err == nil {
		if err := loadFile(c, path); err != nil {
			return nil, err
		}
	}

	applyEnv(c)

	// For a discovered local config, default sync_dir to the directory
	// containing the config file so each workspace is self-contained.
	if local && c.SyncDir == "" && path != DefaultPath() {
		if abs, err := filepath.Abs(filepath.Dir(path)); err == nil {
			c.SyncDir = abs
		}
	}

	return c, nil
}

func loadFile(c *Config, path string) error {
	var raw tomlFile
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	if raw.Auth.Token != "" {
		c.Token = raw.Auth.Token
	}
	if raw.Auth.TokenFile != "" {
		c.TokenFile = expandHome(raw.Auth.TokenFile)
	}
	if raw.Sync.RootPageID != "" {
		c.RootPageID = raw.Sync.RootPageID
	}
	if raw.Sync.SyncDir != "" {
		c.SyncDir = expandHome(raw.Sync.SyncDir)
	}
	if raw.Sync.RPS > 0 {
		c.RPS = raw.Sync.RPS
	}
	if raw.Sync.Concurrency > 0 {
		c.Concurrency = raw.Sync.Concurrency
	}
	if raw.Sync.AutoCommit != nil {
		c.AutoCommit = *raw.Sync.AutoCommit
	}
	if raw.Sync.DownloadAssets != nil {
		c.DownloadAssets = *raw.Sync.DownloadAssets
	}
	return nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("NODIN_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("NODIN_ROOT_PAGE_ID"); v != "" {
		c.RootPageID = v
	}
	if v := os.Getenv("NODIN_SYNC_DIR"); v != "" {
		c.SyncDir = expandHome(v)
	}
	if v := os.Getenv("NODIN_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.RPS = n
		}
	}
	if v := os.Getenv("NODIN_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Concurrency = n
		}
	}
	if v := os.Getenv("NODIN_AUTO_COMMIT"); v != "" {
		c.AutoCommit = parseBool(v)
	}
	if v := os.Getenv("NODIN_DOWNLOAD_ASSETS"); v != "" {
		c.DownloadAssets = parseBool(v)
	}
}

// Validate returns an error if any required or structural field is invalid.
// It does not check whether SyncDir exists on disk; that is nodin doctor's job.
func (c *Config) Validate() error {
	token, err := c.ResolvedToken()
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("token is required; set NODIN_TOKEN env or [auth] token in config")
	}
	if c.RootPageID == "" {
		return fmt.Errorf("root_page_id is required; set NODIN_ROOT_PAGE_ID env or [sync] root_page_id in config")
	}
	if !isValidUUID(c.RootPageID) {
		return fmt.Errorf("root_page_id %q is not a valid UUID", c.RootPageID)
	}
	if c.SyncDir != "" && !filepath.IsAbs(c.SyncDir) {
		return fmt.Errorf("sync_dir must be an absolute path, got %q", c.SyncDir)
	}
	if c.RPS < 1 {
		return fmt.Errorf("rate_limit_rps must be >= 1")
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("concurrency must be >= 1")
	}
	return nil
}

// ResolvedToken returns the token string, reading TokenFile if necessary.
func (c *Config) ResolvedToken() (string, error) {
	if c.Token != "" {
		return c.Token, nil
	}
	if c.TokenFile != "" {
		data, err := os.ReadFile(c.TokenFile)
		if err != nil {
			return "", fmt.Errorf("read token_file %s: %w", c.TokenFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", nil
}

// Write serializes c to a TOML file at path, creating parent directories.
// The file is written with 0600 permissions since it may contain a token.
func Write(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	autoCommit := c.AutoCommit
	dlAssets := c.DownloadAssets

	raw := tomlFile{
		Auth: tomlAuth{
			Token:     c.Token,
			TokenFile: c.TokenFile,
		},
		Sync: tomlSync{
			RootPageID:     c.RootPageID,
			SyncDir:        c.SyncDir,
			RPS:            c.RPS,
			Concurrency:    c.Concurrency,
			AutoCommit:     &autoCommit,
			DownloadAssets: &dlAssets,
		},
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	defer func() { _ = f.Close() }()

	return toml.NewEncoder(f).Encode(raw)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes"
}

// isValidUUID accepts hyphenated and unhyphenated Notion UUIDs.
func isValidUUID(s string) bool {
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return false
	}
	for _, ch := range s {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}
