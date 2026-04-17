package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentAdapter handles MCP registration for a specific coding agent.
type AgentAdapter interface {
	// Name returns the agent identifier (claude, opencode, codex, gemini).
	Name() string
	// Label returns a human-readable name (e.g. "Claude Code").
	Label() string
	// SetMode sets the integration mode (MCP tools or skills).
	SetMode(IntegrationMode)
	// ConfigPaths returns the file paths this adapter writes for display.
	ConfigPaths(global bool) []string
	// HasConfig returns true if monocle is already registered for this agent at the given scope.
	HasConfig(global bool) bool
	// Register writes the agent's config files.
	Register(global bool) error
	// Unregister removes the agent's config files.
	Unregister(global bool) error
}

// ValidAgentNames lists the accepted agent identifiers for the CLI.
var ValidAgentNames = []string{"claude", "opencode", "codex", "gemini"}

// AllAdapters returns an adapter for each supported agent.
func AllAdapters() []AgentAdapter {
	return []AgentAdapter{
		&ClaudeAdapter{},
		&OpenCodeAdapter{},
		&CodexAdapter{},
		&GeminiAdapter{},
	}
}

// GetAdapter returns the adapter for the named agent.
func GetAdapter(name string) (AgentAdapter, error) {
	for _, a := range AllAdapters() {
		if a.Name() == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("unknown agent %q (valid: claude, opencode, codex, gemini)", name)
}

// ResolveCommand returns the monocle binary path for config files.
// Local configs use "monocle" (resolved via PATH); global configs use the absolute path.
func ResolveCommand(global bool) string {
	if global {
		if exePath, err := os.Executable(); err == nil {
			return exePath
		}
	}
	return "monocle"
}

// ResolveHookCommand returns the monocle binary path to record in a
// settings.json hook entry. Unlike MCP-server invocations, which inherit
// the Claude Code session's PATH (including any PATH-extending SessionStart
// hooks), hook subprocesses spawned by Claude Code do NOT reliably inherit
// that environment. A PATH-resolved "monocle" can therefore pick up an
// older/global install that predates the hooks subcommand, which fails
// silently (exit 80, empty stdout) and looks exactly like "my hook isn't
// firing."
//
// Resolution:
//   - If the running binary sits inside the settings file's project root
//     (the directory containing .claude/), emit a repo-relative path like
//     "./bin/monocle" so the settings.json stays portable across machines
//     that each build to the same relative location.
//   - Otherwise emit the absolute path of the running binary.
//   - Global-scope configs always use the absolute path — a path relative
//     to $HOME would be misleading.
func ResolveHookCommand(settingsPath string, global bool) string {
	exePath, err := os.Executable()
	if err != nil {
		return "monocle"
	}
	absExe, err := filepath.Abs(exePath)
	if err != nil {
		return exePath
	}

	if global {
		return absExe
	}

	settingsAbs, err := filepath.Abs(settingsPath)
	if err != nil {
		return absExe
	}
	// settingsPath is typically ".claude/settings.json"; strip two levels
	// to get the directory that contains .claude/.
	projectRoot := filepath.Dir(filepath.Dir(settingsAbs))

	rel, err := filepath.Rel(projectRoot, absExe)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return absExe
	}
	return "./" + filepath.ToSlash(rel)
}
