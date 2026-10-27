package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// LoadConfig loads configuration from XDG-compliant paths.
// It checks ~/.config/monocle/config.json first, then .monocle/config.json in cwd.
func LoadConfig() (*types.Config, error) {
	cfg := DefaultConfig()
	for i, path := range configFiles() {
		if data, err := os.ReadFile(path); err == nil {
			applyConfigFile(cfg, data, i > 0) //nolint:errcheck
		}
	}
	return cfg, nil
}

// applyConfigFile reads one config file over cfg. A project's file (the
// repository's own .monocle/config.json: the repo under review, whoever wrote
// it) sets the other settings only. Every field that names something to run
// (GlobalOnlyKeys: an editor, a viewer, a hook command, an editor's arguments)
// is the global config's alone, so a repository never chooses what Monocle runs.
func applyConfigFile(cfg *types.Config, data []byte, project bool) error {
	if !project {
		return json.Unmarshal(data, cfg)
	}
	kept := keepGlobalOnly(cfg)
	err := json.Unmarshal(data, cfg)
	kept(cfg)
	return err
}

// GlobalOnlyKeys are the config's keys that name something to run: a program
// (editor, markdown_viewer, media_viewer), a shell command (the walkthrough_*
// hooks, cursor_command, comments_command, related_editor_add and _preview,
// related_open), or arguments handed to one (related_editor_args). Only the
// global config sets them. ProjectKeys are the rest; a test holds every field
// of types.Config to one list or the other, so a new one is placed on purpose.
var GlobalOnlyKeys = []string{
	"editor", "markdown_viewer", "media_viewer",
	"walkthrough_on_stop", "walkthrough_view_status", "walkthrough_layout_reset",
	"walkthrough_navigate", "walkthrough_ask", "walkthrough_resolve",
	"cursor_command", "comments_command",
	"related_editor_args", "related_editor_add", "related_editor_preview", "related_open",
}

// ProjectKeys are the settings a project's file may set: how the review looks
// and behaves, nothing that runs.
var ProjectKeys = []string{
	"ignore_patterns", "keybindings", "diff_style", "sidebar_style", "theme", "layout", "wrap", "tab_size",
	"context_lines", "full_file_diff", "highlight_color", "editor_mode", "editor_focus", "review_format",
	"auto_focus_mode", "mouse", "min_diff_width", "comment_expand", "comment_expand_delay",
	"mark_reviewed_on_submit", "review_tracking", "idle_timeout",
}

// ConfigKey is a field's key in the config file.
func ConfigKey(f reflect.StructField) string {
	k, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return k
}

// keepGlobalOnly copies cfg's GlobalOnlyKeys fields (deeply: a slice is cloned,
// as decoding into cfg may reuse its array) and returns what puts them back.
func keepGlobalOnly(cfg *types.Config) func(*types.Config) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	saved := map[int]reflect.Value{}
	for i := 0; i < t.NumField(); i++ {
		if !slices.Contains(GlobalOnlyKeys, ConfigKey(t.Field(i))) {
			continue
		}
		f := v.Field(i)
		c := reflect.New(f.Type()).Elem()
		if f.Kind() == reflect.Slice && !f.IsNil() {
			c.Set(reflect.AppendSlice(reflect.MakeSlice(f.Type(), 0, f.Len()), f))
		} else {
			c.Set(f)
		}
		saved[i] = c
	}
	return func(dst *types.Config) {
		d := reflect.ValueOf(dst).Elem()
		for i, c := range saved {
			d.Field(i).Set(c)
		}
	}
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
	for i, path := range configFiles() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := applyConfigFile(cfg, data, i > 0); err != nil {
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
