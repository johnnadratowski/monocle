package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeChannelRegister(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}

	// Not registered initially
	if adapter.HasMCPConfig() {
		t.Fatal("should not have MCP config initially")
	}
	if !adapter.NeedsRegister() {
		t.Fatal("should need register initially")
	}

	// Register
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Verify .mcp.json exists with monocle entry
	mcpData, err := os.ReadFile(filepath.Join(projDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	var mcpConfig map[string]any
	if err := json.Unmarshal(mcpData, &mcpConfig); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	servers, ok := mcpConfig["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers should exist in .mcp.json")
	}
	entry, ok := servers["monocle"].(map[string]any)
	if !ok {
		t.Fatal("monocle should be in mcpServers")
	}

	// Verify the entry points to monocle serve-mcp-channel
	command, _ := entry["command"].(string)
	if command != "monocle" {
		t.Fatalf("command should be 'monocle', got %q", command)
	}
	args, _ := entry["args"].([]any)
	if len(args) != 1 || args[0] != "serve-mcp-channel" {
		t.Fatalf("args should be ['serve-mcp-channel'], got %v", args)
	}

	// Should no longer need registration
	if adapter.NeedsRegister() {
		t.Fatal("should not need register after Register()")
	}
}

func TestClaudeChannelRegister_Global(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(true); err != nil {
		t.Fatalf("register global failed: %v", err)
	}

	// Verify global .mcp.json exists
	mcpData, err := os.ReadFile(filepath.Join(homeDir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read ~/.mcp.json: %v", err)
	}
	var mcpConfig map[string]any
	if err := json.Unmarshal(mcpData, &mcpConfig); err != nil {
		t.Fatalf("parse .mcp.json: %v", err)
	}
	servers, ok := mcpConfig["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("mcpServers should exist in ~/.mcp.json")
	}
	if _, ok := servers["monocle"]; !ok {
		t.Fatal("monocle should be in mcpServers")
	}
}

func TestHasMCPConfig_NoFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if adapter.HasMCPConfig() {
		t.Fatal("should return false when no .mcp.json exists")
	}
}

func TestHasMCPConfig_GlobalExists(t *testing.T) {
	dir := t.TempDir()

	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write global .mcp.json with monocle entry
	mcpData := map[string]any{
		"mcpServers": map[string]any{
			"monocle": map[string]any{"command": "monocle", "args": []any{"serve-mcp-channel"}},
		},
	}
	data, _ := json.Marshal(mcpData)
	os.WriteFile(filepath.Join(homeDir, ".mcp.json"), data, 0644)

	adapter := &ClaudeAdapter{}
	if !adapter.HasMCPConfig() {
		t.Fatal("should return true when global .mcp.json has monocle")
	}
}

func TestHasMCPConfig_LocalExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write local .mcp.json with monocle entry
	mcpData := map[string]any{
		"mcpServers": map[string]any{
			"monocle": map[string]any{"command": "monocle", "args": []any{"serve-mcp-channel"}},
		},
	}
	data, _ := json.Marshal(mcpData)
	os.WriteFile(filepath.Join(projDir, ".mcp.json"), data, 0644)

	adapter := &ClaudeAdapter{}
	if !adapter.HasMCPConfig() {
		t.Fatal("should return true when local .mcp.json has monocle")
	}
}

func TestHasMCPConfig_OldStyleReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Old-style config pointing to bun + channel.ts
	mcpData := map[string]any{
		"mcpServers": map[string]any{
			"monocle": map[string]any{
				"command": "bun",
				"args":    []any{"${HOME}/.config/monocle/channel.ts"},
			},
		},
	}
	data, _ := json.Marshal(mcpData)
	os.WriteFile(filepath.Join(projDir, ".mcp.json"), data, 0644)

	adapter := &ClaudeAdapter{}
	if adapter.HasMCPConfig() {
		t.Fatal("should return false for old-style bun config — needs re-registration")
	}
}

func TestNeedsRegister_NoConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if !adapter.NeedsRegister() {
		t.Fatal("should need register when no .mcp.json exists")
	}
}

func TestNeedsRegister_Registered(t *testing.T) {
	dir := t.TempDir()

	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Create .mcp.json with monocle entry
	mcpData := map[string]any{
		"mcpServers": map[string]any{
			"monocle": map[string]any{"command": "monocle", "args": []any{"serve-mcp-channel"}},
		},
	}
	data, _ := json.Marshal(mcpData)
	os.WriteFile(filepath.Join(homeDir, ".mcp.json"), data, 0644)

	adapter := &ClaudeAdapter{}
	if adapter.NeedsRegister() {
		t.Fatal("should not need register when MCP config exists")
	}
}

func TestClaudeChannelRegister_Idempotent(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("second register: %v", err)
	}

	if adapter.NeedsRegister() {
		t.Fatal("should not need register after double Register()")
	}
}

func TestClaudeChannelUnregister(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := adapter.Unregister(false); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	if adapter.HasMCPConfig() {
		t.Fatal("should not have MCP config after unregister")
	}

	// .mcp.json should be removed (was only entry)
	if _, err := os.Stat(filepath.Join(projDir, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal(".mcp.json should be removed after unregister")
	}
}

func TestClaudeRegister_AddsPermissions(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	data, err := ReadJSONFile(filepath.Join(projDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	perms, ok := data["permissions"].(map[string]any)
	if !ok {
		t.Fatal("permissions key should exist")
	}
	allow, ok := perms["allow"].([]any)
	if !ok {
		t.Fatal("permissions.allow should exist")
	}

	allowSet := make(map[string]bool)
	for _, v := range allow {
		if s, ok := v.(string); ok {
			allowSet[s] = true
		}
	}
	for _, perm := range MonocleClaudePermissions {
		if !allowSet[perm] {
			t.Errorf("missing permission: %s", perm)
		}
	}
}

func TestClaudeRegister_PreservesExistingPermissions(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write existing settings with custom permissions
	settingsPath := filepath.Join(projDir, ".claude", "settings.json")
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(ls:*)", "Bash(cat:*)"},
		},
		"hooks": map[string]any{"test": "value"},
	}
	if err := WriteJSONFile(settingsPath, existing); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	// Verify existing permissions are preserved
	perms := data["permissions"].(map[string]any)
	allow := perms["allow"].([]any)
	allowSet := make(map[string]bool)
	for _, v := range allow {
		if s, ok := v.(string); ok {
			allowSet[s] = true
		}
	}
	if !allowSet["Bash(ls:*)"] {
		t.Error("existing Bash(ls:*) permission should be preserved")
	}
	if !allowSet["Bash(cat:*)"] {
		t.Error("existing Bash(cat:*) permission should be preserved")
	}

	// Verify hooks are preserved
	if _, ok := data["hooks"]; !ok {
		t.Error("hooks key should be preserved")
	}
}

func TestClaudeRegister_PermissionsIdempotent(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("second register: %v", err)
	}

	data, err := ReadJSONFile(filepath.Join(projDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	allow := data["permissions"].(map[string]any)["allow"].([]any)

	// Count monocle permissions — should not be duplicated
	count := 0
	for _, v := range allow {
		s, _ := v.(string)
		for _, perm := range MonocleClaudePermissions {
			if s == perm {
				count++
			}
		}
	}
	if count != len(MonocleClaudePermissions) {
		t.Errorf("expected %d monocle permissions, got %d (duplicates?)", len(MonocleClaudePermissions), count)
	}
}

func TestClaudeUnregister_RemovesPermissions(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := adapter.Unregister(false); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	// settings.json should be removed (was only monocle permissions)
	settingsPath := filepath.Join(projDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatal("settings.json should be removed when only monocle permissions existed")
	}
}

func TestClaudeUnregister_PreservesOtherPermissions(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write settings with monocle + custom permissions
	settingsPath := filepath.Join(projDir, ".claude", "settings.json")
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(ls:*)", "Bash(monocle review:*)", "Skill(get-feedback)"},
		},
	}
	if err := WriteJSONFile(settingsPath, existing); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Register first (adds all monocle perms), then unregister
	adapter := &ClaudeAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := adapter.Unregister(false); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	perms := data["permissions"].(map[string]any)
	allow := perms["allow"].([]any)

	if len(allow) != 1 {
		t.Fatalf("expected 1 remaining permission, got %d: %v", len(allow), allow)
	}
	if allow[0] != "Bash(ls:*)" {
		t.Errorf("expected Bash(ls:*) to remain, got %v", allow[0])
	}
}

func writeInstalledPlugins(t *testing.T, homeDir string, plugins map[string]any) {
	t.Helper()
	pluginsDir := filepath.Join(homeDir, ".claude", "plugins")
	os.MkdirAll(pluginsDir, 0755)
	data := map[string]any{"version": 2, "plugins": plugins}
	raw, _ := json.Marshal(data)
	os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), raw, 0644)
}

func TestHasPluginConfig_UserScope(t *testing.T) {
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	writeInstalledPlugins(t, homeDir, map[string]any{
		"monocle@monocle": []any{
			map[string]any{"scope": "user"},
		},
	})

	adapter := &ClaudeAdapter{}
	if !adapter.HasPluginConfig() {
		t.Fatal("should return true for user-scoped monocle plugin")
	}
}

func TestHasPluginConfig_ProjectScope_Matching(t *testing.T) {
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	writeInstalledPlugins(t, homeDir, map[string]any{
		"monocle@monocle": []any{
			map[string]any{"scope": "project", "projectPath": projDir},
		},
	})

	adapter := &ClaudeAdapter{}
	if !adapter.HasPluginConfig() {
		t.Fatal("should return true for project-scoped plugin matching cwd")
	}
}

func TestHasPluginConfig_ProjectScope_NonMatching(t *testing.T) {
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	writeInstalledPlugins(t, homeDir, map[string]any{
		"monocle@monocle": []any{
			map[string]any{"scope": "project", "projectPath": "/some/other/project"},
		},
	})

	adapter := &ClaudeAdapter{}
	if adapter.HasPluginConfig() {
		t.Fatal("should return false for project-scoped plugin with non-matching path")
	}
}

func TestHasPluginConfig_NoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	adapter := &ClaudeAdapter{}
	if adapter.HasPluginConfig() {
		t.Fatal("should return false when no installed_plugins.json exists")
	}
}

func TestNeedsRegister_PluginRegistered(t *testing.T) {
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	os.MkdirAll(homeDir, 0755)
	t.Setenv("HOME", homeDir)

	origDir, _ := os.Getwd()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// No .mcp.json, but plugin is registered
	writeInstalledPlugins(t, homeDir, map[string]any{
		"monocle@monocle": []any{
			map[string]any{"scope": "user"},
		},
	})

	adapter := &ClaudeAdapter{}
	if adapter.NeedsRegister() {
		t.Fatal("should not need register when plugin is installed")
	}
}

func TestWriteChannelBundle(t *testing.T) {
	path, err := WriteChannelBundle()
	if err != nil {
		t.Fatalf("WriteChannelBundle: %v", err)
	}

	// Verify the file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("bundle should not be empty")
	}

	// Verify idempotency - second call should return same path
	path2, err := WriteChannelBundle()
	if err != nil {
		t.Fatalf("second WriteChannelBundle: %v", err)
	}
	if path != path2 {
		t.Fatalf("paths should be the same: %q vs %q", path, path2)
	}

	// Clean up
	os.Remove(path)
}
