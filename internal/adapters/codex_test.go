package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexRegister(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &CodexAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	if !strings.Contains(string(content), "[mcp_servers.monocle]") {
		t.Fatal("expected [mcp_servers.monocle] section")
	}
	if !strings.Contains(string(content), `args = ["serve-mcp-channel"]`) {
		t.Fatal("expected serve-mcp-channel args")
	}
}

func TestCodexRegister_Idempotent(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &CodexAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("second register: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(projDir, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	count := strings.Count(string(content), "[mcp_servers.monocle]")
	if count != 1 {
		t.Fatalf("expected exactly 1 monocle section, got %d\n%s", count, content)
	}
}

func TestCodexRegister_OverwriteUpdatesConfig(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	codexDir := filepath.Join(projDir, ".codex")
	os.MkdirAll(codexDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write an old config with a stale command
	oldConfig := "[mcp_servers.monocle]\ncommand = \"old-monocle\"\nargs = [\"serve-mcp-channel\"]\n"
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(oldConfig), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	adapter := &CodexAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	s := string(content)
	if strings.Contains(s, "old-monocle") {
		t.Fatal("old command value should have been replaced")
	}
	count := strings.Count(s, "[mcp_servers.monocle]")
	if count != 1 {
		t.Fatalf("expected exactly 1 monocle section, got %d\n%s", count, s)
	}
}

func TestCodexRegister_PreservesOtherSections(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	codexDir := filepath.Join(projDir, ".codex")
	os.MkdirAll(codexDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	// Write config with monocle and another tool
	oldConfig := "[mcp_servers.other_tool]\ncommand = \"other\"\nargs = [\"run\"]\n\n[mcp_servers.monocle]\ncommand = \"old-monocle\"\nargs = [\"serve-mcp-channel\"]\n"
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(oldConfig), 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	adapter := &CodexAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(codexDir, "config.toml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "[mcp_servers.other_tool]") {
		t.Fatal("other_tool section should be preserved")
	}
	if !strings.Contains(s, `command = "other"`) {
		t.Fatal("other_tool command should be preserved")
	}
	if strings.Contains(s, "old-monocle") {
		t.Fatal("old monocle command should have been replaced")
	}
	count := strings.Count(s, "[mcp_servers.monocle]")
	if count != 1 {
		t.Fatalf("expected exactly 1 monocle section, got %d\n%s", count, s)
	}
}
