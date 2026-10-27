package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DiffStyle != "unified" {
		t.Errorf("DiffStyle: got %q, want %q", cfg.DiffStyle, "unified")
	}
	if cfg.SidebarStyle != "flat" {
		t.Errorf("SidebarStyle: got %q, want %q", cfg.SidebarStyle, "flat")
	}
	if cfg.Layout != "auto" {
		t.Errorf("Layout: got %q, want %q", cfg.Layout, "auto")
	}
	if cfg.TabSize != 4 {
		t.Errorf("TabSize: got %d, want %d", cfg.TabSize, 4)
	}
	if cfg.ContextLines != 3 {
		t.Errorf("ContextLines: got %d, want %d", cfg.ContextLines, 3)
	}
	if cfg.Wrap != false {
		t.Errorf("Wrap: got %v, want %v", cfg.Wrap, false)
	}
	if cfg.IgnorePatterns == nil {
		t.Error("IgnorePatterns: got nil, want empty slice")
	}
	if len(cfg.IgnorePatterns) != 0 {
		t.Errorf("IgnorePatterns: got %d elements, want 0", len(cfg.IgnorePatterns))
	}
	if !cfg.ReviewTracking {
		t.Error("ReviewTracking: got false, want true (on by default)")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Chdir to a temp dir so project-level config doesn't interfere.
	projDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}

	cfg := &types.Config{
		IgnorePatterns: []string{"vendor/", "node_modules/"},
		DiffStyle:      "side-by-side",
		SidebarStyle:   "tree",
		Layout:         "wide",
		Wrap:           true,
		TabSize:        2,
		ContextLines:   5,
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if loaded.DiffStyle != "side-by-side" {
		t.Errorf("DiffStyle: got %q, want %q", loaded.DiffStyle, "side-by-side")
	}
	if loaded.SidebarStyle != "tree" {
		t.Errorf("SidebarStyle: got %q, want %q", loaded.SidebarStyle, "tree")
	}
	if loaded.Layout != "wide" {
		t.Errorf("Layout: got %q, want %q", loaded.Layout, "wide")
	}
	if loaded.Wrap != true {
		t.Errorf("Wrap: got %v, want %v", loaded.Wrap, true)
	}
	if loaded.TabSize != 2 {
		t.Errorf("TabSize: got %d, want %d", loaded.TabSize, 2)
	}
	if loaded.ContextLines != 5 {
		t.Errorf("ContextLines: got %d, want %d", loaded.ContextLines, 5)
	}
	if len(loaded.IgnorePatterns) != 2 {
		t.Fatalf("IgnorePatterns: got %d elements, want 2", len(loaded.IgnorePatterns))
	}
	if loaded.IgnorePatterns[0] != "vendor/" {
		t.Errorf("IgnorePatterns[0]: got %q, want %q", loaded.IgnorePatterns[0], "vendor/")
	}
	if loaded.IgnorePatterns[1] != "node_modules/" {
		t.Errorf("IgnorePatterns[1]: got %q, want %q", loaded.IgnorePatterns[1], "node_modules/")
	}
}

func TestLoadConfig_GlobalOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Chdir to a temp dir so project-level config doesn't interfere.
	projDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}

	// Write a global config that only overrides DiffStyle.
	globalDir := filepath.Join(tmpDir, "monocle")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	partial := map[string]string{"diff_style": "split"}
	data, err := json.Marshal(partial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// DiffStyle should be overridden.
	if cfg.DiffStyle != "split" {
		t.Errorf("DiffStyle: got %q, want %q", cfg.DiffStyle, "split")
	}

	// Other fields should retain defaults.
	defaults := DefaultConfig()
	if cfg.SidebarStyle != defaults.SidebarStyle {
		t.Errorf("SidebarStyle: got %q, want %q", cfg.SidebarStyle, defaults.SidebarStyle)
	}
	if cfg.Layout != defaults.Layout {
		t.Errorf("Layout: got %q, want %q", cfg.Layout, defaults.Layout)
	}
	if cfg.TabSize != defaults.TabSize {
		t.Errorf("TabSize: got %d, want %d", cfg.TabSize, defaults.TabSize)
	}
	if cfg.ContextLines != defaults.ContextLines {
		t.Errorf("ContextLines: got %d, want %d", cfg.ContextLines, defaults.ContextLines)
	}
	if cfg.Wrap != defaults.Wrap {
		t.Errorf("Wrap: got %v, want %v", cfg.Wrap, defaults.Wrap)
	}
}

func TestLoadConfig_ProjectOverride(t *testing.T) {
	// Point XDG_CONFIG_HOME to a nonexistent dir so no global config loads.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	// Create a temp dir to act as the project root with .monocle/config.json.
	projDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}

	monocleDir := filepath.Join(projDir, ".monocle")
	if err := os.MkdirAll(monocleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	partial := map[string]int{"tab_size": 8}
	data, err := json.Marshal(partial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(monocleDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// TabSize should be overridden by project config.
	if cfg.TabSize != 8 {
		t.Errorf("TabSize: got %d, want %d", cfg.TabSize, 8)
	}

	// Other fields should retain defaults.
	defaults := DefaultConfig()
	if cfg.DiffStyle != defaults.DiffStyle {
		t.Errorf("DiffStyle: got %q, want %q", cfg.DiffStyle, defaults.DiffStyle)
	}
	if cfg.SidebarStyle != defaults.SidebarStyle {
		t.Errorf("SidebarStyle: got %q, want %q", cfg.SidebarStyle, defaults.SidebarStyle)
	}
	if cfg.Layout != defaults.Layout {
		t.Errorf("Layout: got %q, want %q", cfg.Layout, defaults.Layout)
	}
}

func TestLoadConfig_NoFiles(t *testing.T) {
	// Point XDG_CONFIG_HOME to a nonexistent dir.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	// Chdir to an empty temp dir so no .monocle/config.json exists.
	projDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(projDir); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	defaults := DefaultConfig()
	if cfg.DiffStyle != defaults.DiffStyle {
		t.Errorf("DiffStyle: got %q, want %q", cfg.DiffStyle, defaults.DiffStyle)
	}
	if cfg.SidebarStyle != defaults.SidebarStyle {
		t.Errorf("SidebarStyle: got %q, want %q", cfg.SidebarStyle, defaults.SidebarStyle)
	}
	if cfg.Layout != defaults.Layout {
		t.Errorf("Layout: got %q, want %q", cfg.Layout, defaults.Layout)
	}
	if cfg.TabSize != defaults.TabSize {
		t.Errorf("TabSize: got %d, want %d", cfg.TabSize, defaults.TabSize)
	}
	if cfg.ContextLines != defaults.ContextLines {
		t.Errorf("ContextLines: got %d, want %d", cfg.ContextLines, defaults.ContextLines)
	}
	if cfg.Wrap != defaults.Wrap {
		t.Errorf("Wrap: got %v, want %v", cfg.Wrap, defaults.Wrap)
	}
}

// projectAndGlobal writes a global config and a project's .monocle/config.json
// (the project's in a fresh working directory) and loads with each loader.
func projectAndGlobal(t *testing.T, global, project string) map[string]*types.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "monocle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "monocle", "config.json"), []byte(global), 0o644); err != nil {
		t.Fatal(err)
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".monocle", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".monocle", "config.json"), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	out := map[string]*types.Config{}
	for name, load := range map[string]func() (*types.Config, error){"LoadConfig": LoadConfig, "readConfig (the live re-read)": readConfig} {
		cfg, err := load()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = cfg
	}
	return out
}

// fieldByKey is the config's field with that key, as a value.
func fieldByKey(t *testing.T, cfg *types.Config, key string) reflect.Value {
	t.Helper()
	v := reflect.ValueOf(cfg).Elem()
	for i := 0; i < v.NumField(); i++ {
		if ConfigKey(v.Type().Field(i)) == key {
			return v.Field(i)
		}
	}
	t.Fatalf("no config field %q", key)
	return reflect.Value{}
}

// A repository under review must not choose what Monocle runs: its own
// .monocle/config.json never sets an editor, a viewer, a hook command or an
// editor's arguments, in the first load or the live re-read; the global config
// (a walkthrough's own, under its XDG_CONFIG_HOME) still does. One case a field.
func TestLoadConfig_ProjectCannotSetWhatRuns(t *testing.T) {
	for _, key := range GlobalOnlyKeys {
		t.Run(key, func(t *testing.T) {
			val := func(who string) any {
				if key == "related_editor_args" {
					return []string{"-c", who}
				}
				return who + " command"
			}
			g, _ := json.Marshal(map[string]any{key: val("global")})
			p, _ := json.Marshal(map[string]any{key: val("project"), "tab_size": 8})
			for name, cfg := range projectAndGlobal(t, string(g), string(p)) {
				got := fieldByKey(t, cfg, key).Interface()
				if !reflect.DeepEqual(got, val("global")) {
					t.Errorf("%s: %s = %v, want the global config's", name, key, got)
				}
				if cfg.TabSize != 8 {
					t.Errorf("%s: the project's tab_size was dropped with it", name)
				}
			}
		})
	}
}

// With no global value, a project's is not taken either: the field stays empty.
func TestLoadConfig_ProjectAloneSetsNothingThatRuns(t *testing.T) {
	project := map[string]any{}
	for _, key := range GlobalOnlyKeys {
		if key == "related_editor_args" {
			project[key] = []string{"-c", "evil"}
		} else {
			project[key] = "touch /tmp/pwned"
		}
	}
	p, _ := json.Marshal(project)
	def := DefaultConfig()
	for name, cfg := range projectAndGlobal(t, "{}", string(p)) {
		for _, key := range GlobalOnlyKeys {
			if got, want := fieldByKey(t, cfg, key).Interface(), fieldByKey(t, def, key).Interface(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: a project alone set %s to %v (default %v)", name, key, got, want)
			}
		}
	}
}

// Every field of the config is placed on purpose: one that runs something is
// global-only, the rest a project may set. A new field fails here until it is
// put in one list or the other.
func TestEveryConfigFieldIsGlobalOnlyOrAProjects(t *testing.T) {
	typ := reflect.TypeOf(types.Config{})
	seen := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		key := ConfigKey(typ.Field(i))
		seen[key] = true
		inG, inP := slices.Contains(GlobalOnlyKeys, key), slices.Contains(ProjectKeys, key)
		if inG == inP {
			t.Errorf("config field %s (%s): in GlobalOnlyKeys %v, in ProjectKeys %v; it must be in exactly one (does it run anything?)",
				typ.Field(i).Name, key, inG, inP)
		}
	}
	for _, k := range append(append([]string{}, GlobalOnlyKeys...), ProjectKeys...) {
		if !seen[k] {
			t.Errorf("%s is listed but no config field has that key", k)
		}
	}
}
