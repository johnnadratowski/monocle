package adapters

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CodexAdapter handles MCP registration for OpenAI Codex CLI.
type CodexAdapter struct{}

func (a *CodexAdapter) Name() string  { return "codex" }
func (a *CodexAdapter) Label() string { return "Codex CLI" }

func (a *CodexAdapter) ConfigPaths(global bool) []string {
	return []string{codexConfigPath(global)}
}

func (a *CodexAdapter) HasConfig(global bool) bool {
	content, err := os.ReadFile(codexConfigPath(global))
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "[mcp_servers.monocle]")
}

func (a *CodexAdapter) Register(global bool) error {
	command := ResolveCommand(global)
	path := codexConfigPath(global)

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}

	existing := removeMonocleTOMLSection(string(content))

	block := fmt.Sprintf("\n[mcp_servers.monocle]\ncommand = %q\nargs = [\"serve-mcp-channel\"]\n", command)

	if len(existing) > 0 && !strings.HasSuffix(existing, "\n") {
		block = "\n" + block
	}

	return WriteFileAtomic(path, []byte(existing+block))
}

func (a *CodexAdapter) Unregister(global bool) error {
	path := codexConfigPath(global)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	cleaned := strings.TrimRight(removeMonocleTOMLSection(string(content)), "\n") + "\n"
	if strings.TrimSpace(cleaned) == "" {
		return RemoveFileIfExists(path)
	}
	return WriteFileAtomic(path, []byte(cleaned))
}

// removeMonocleTOMLSection strips the [mcp_servers.monocle] section and its
// key-value lines from a TOML document string.
func removeMonocleTOMLSection(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[mcp_servers.monocle]" {
			inSection = true
			continue
		}
		if inSection {
			if strings.HasPrefix(trimmed, "[") {
				inSection = false
				result = append(result, line)
			}
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// Detect returns true if Codex CLI appears to be installed.
func (a *CodexAdapter) Detect() bool {
	if _, err := exec.LookPath("codex"); err == nil {
		return true
	}
	if _, err := os.Stat(".codex"); err == nil {
		return true
	}
	return false
}

func codexConfigPath(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".codex", "config.toml")
		}
	}
	return filepath.Join(".codex", "config.toml")
}
