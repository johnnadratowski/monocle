package adapters

import (
	"fmt"
	"os"
)

// AgentAdapter handles MCP registration for a specific coding agent.
type AgentAdapter interface {
	// Name returns the agent identifier (claude, opencode, codex, gemini).
	Name() string
	// Label returns a human-readable name (e.g. "Claude Code").
	Label() string
	// ConfigPaths returns the file paths this adapter writes for display.
	ConfigPaths(global bool) []string
	// HasConfig returns true if monocle is already registered for this agent.
	HasConfig() bool
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
