package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/josephschmitt/monocle/internal/types"
)

// LoadConfig loads configuration from XDG-compliant paths.
// It checks ~/.config/monocle/config.json first, then .monocle/config.json in cwd.
func LoadConfig() (*types.Config, error) {
	cfg := DefaultConfig()
	for _, path := range configFiles() {
		if data, err := os.ReadFile(path); err == nil {
			json.Unmarshal(data, cfg) //nolint:errcheck
		}
	}
	return cfg, nil
}

// configFiles are the files a config is read from, later ones overriding
// earlier: the global config, then the project's.
func configFiles() []string {
	return []string{configPath(), filepath.Join(".monocle", "config.json")}
}

// readConfig builds the config from configFiles as LoadConfig does, but fails
// on a file that does not parse rather than skipping it: re-reading a file
// caught half-written must not swap every setting back to its default.
func readConfig() (*types.Config, error) {
	cfg := DefaultConfig()
	for _, path := range configFiles() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return cfg, nil
}

// fileStamp is enough of a file's state to tell it changed: whether it is
// there, its size and its modification time.
type fileStamp struct {
	exists bool
	size   int64
	mod    int64 // UnixNano
}

// stampConfigFiles stamps each of configFiles, in order.
func stampConfigFiles() []fileStamp {
	files := configFiles()
	out := make([]fileStamp, len(files))
	for i, path := range files {
		if info, err := os.Stat(path); err == nil {
			out[i] = fileStamp{exists: true, size: info.Size(), mod: info.ModTime().UnixNano()}
		}
	}
	return out
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *types.Config {
	return &types.Config{
		IgnorePatterns: []string{},
		DiffStyle:      "unified",
		SidebarStyle:   "flat",
		Theme:          "dark",
		Layout:         "auto",
		TabSize:        4,
		ContextLines:   3,
		ReviewFormat: types.ReviewFormatConfig{
			IncludeSnippets: true,
			MaxSnippetLines: 10,
			IncludeSummary:  true,
		},
		MinDiffWidth:   80,
		ReviewTracking: true,
		EditorMode:     "terminal",
	}
}

// SaveConfig writes the configuration to the global config path.
func SaveConfig(cfg *types.Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func configPath() string {
	cfgDir := os.Getenv("XDG_CONFIG_HOME")
	if cfgDir == "" {
		home, _ := os.UserHomeDir()
		cfgDir = filepath.Join(home, ".config")
	}
	return filepath.Join(cfgDir, "monocle", "config.json")
}
