package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiRegister_CreatesPolicyFile(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &GeminiAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}

	policyPath := filepath.Join(projDir, ".gemini", "policies", "monocle.toml")
	content, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read policy: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, `toolName = "run_shell_command"`) {
		t.Error("policy should target run_shell_command tool")
	}
	if !strings.Contains(s, `commandPrefix = "monocle"`) {
		t.Error("policy should have monocle command prefix")
	}
	if !strings.Contains(s, `decision = "allow"`) {
		t.Error("policy should allow monocle commands")
	}
}

func TestGeminiRegister_PolicyIdempotent(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &GeminiAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("second register: %v", err)
	}

	policyPath := filepath.Join(projDir, ".gemini", "policies", "monocle.toml")
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("policy file should exist: %v", err)
	}
}

func TestGeminiUnregister_RemovesPolicyFile(t *testing.T) {
	setupTestSkills(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	os.MkdirAll(projDir, 0755)

	origDir, _ := os.Getwd()
	os.Chdir(projDir)
	defer os.Chdir(origDir)

	adapter := &GeminiAdapter{}
	if err := adapter.Register(false); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := adapter.Unregister(false); err != nil {
		t.Fatalf("unregister: %v", err)
	}

	policyPath := filepath.Join(projDir, ".gemini", "policies", "monocle.toml")
	if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
		t.Fatal("policy file should be removed after unregister")
	}
}
