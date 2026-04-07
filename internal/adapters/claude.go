package adapters

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MonocleClaudePermissions are the permission entries added to .claude/settings.json.
var MonocleClaudePermissions = []string{
	"Bash(monocle review:*)",
	"Skill(get-feedback)",
	"Skill(get-feedback-wait)",
	"Skill(review-plan)",
	"Skill(review-plan-wait)",
}

// IntegrationMode describes how an agent integrates with Monocle.
type IntegrationMode string

const (
	// ModeMCPTools uses MCP tools for all operations (recommended for Claude).
	ModeMCPTools IntegrationMode = "mcp-tools"

	// ModeSkills uses skill files and CLI commands with MCP channel notifications.
	ModeSkills IntegrationMode = "skills"
)

// ClaudeAdapter handles Claude Code registration.
// Set Mode before calling Register to control the integration style.
type ClaudeAdapter struct {
	// Mode controls the integration style. Set before calling Register.
	// ModeMCPTools (default): MCP tools + channels, no skills.
	// ModeSkills: channels only + skills + bash permissions.
	Mode IntegrationMode
}

func (a *ClaudeAdapter) Name() string  { return "claude" }
func (a *ClaudeAdapter) Label() string { return "Claude Code" }

// ConfigPaths returns the files written by Register.
func (a *ClaudeAdapter) ConfigPaths(global bool) []string {
	paths := []string{mcpJSONPath(global)}
	if a.effectiveMode() == ModeSkills {
		paths = append(paths, claudeSettingsPath(global))
		paths = append(paths, SkillPaths(claudeSkillsDir(global))...)
	} else {
		paths = append(paths, CommandPaths(claudeCommandsDir(global), ".md")...)
	}
	return paths
}

// HasConfig returns true if monocle is configured at the given scope via .mcp.json or Claude Code plugin.
func (a *ClaudeAdapter) HasConfig(global bool) bool {
	if hasMCPServersEntry(mcpJSONPath(global)) {
		return true
	}
	// Plugin config is always user-level (global)
	if global {
		return a.HasPluginConfig()
	}
	return false
}

// Detect returns true if Claude Code appears to be installed.
func (a *ClaudeAdapter) Detect() bool {
	if _, err := exec.LookPath("claude"); err == nil {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if info, err := os.Stat(filepath.Join(home, ".claude")); err == nil && info.IsDir() {
		return true
	}
	return false
}

// Register adds monocle to .mcp.json and optionally configures permissions and skills.
// In MCP tools mode: configures MCP server with tools + channels, no skills or bash permissions.
// In skills mode: configures MCP server with channels only, installs skills and bash permissions.
func (a *ClaudeAdapter) Register(global bool) error {
	if err := a.configureMCP(global); err != nil {
		return fmt.Errorf("configure mcp: %w", err)
	}

	if a.effectiveMode() == ModeSkills {
		if err := configureClaudeSettings(claudeSettingsPath(global)); err != nil {
			return fmt.Errorf("configure settings: %w", err)
		}
		if err := InstallSkills(claudeSkillsDir(global)); err != nil {
			return fmt.Errorf("install skills: %w", err)
		}
	} else {
		if err := InstallMarkdownCommands(claudeCommandsDir(global)); err != nil {
			return fmt.Errorf("install commands: %w", err)
		}
	}
	return nil
}

// Unregister removes monocle from .mcp.json, removes permissions, skills, and commands.
func (a *ClaudeAdapter) Unregister(global bool) error {
	if err := a.unconfigureMCP(global); err != nil {
		return fmt.Errorf("unconfigure mcp: %w", err)
	}
	if err := unconfigureClaudeSettings(claudeSettingsPath(global)); err != nil {
		return fmt.Errorf("unconfigure settings: %w", err)
	}
	RemoveSkills(claudeSkillsDir(global))
	RemoveCommands(claudeCommandsDir(global), ".md")
	return nil
}

// HasMCPConfig checks if monocle is correctly configured in any .mcp.json (global or local).
// Returns true for both serve-mcp and legacy serve-mcp-channel entries.
func (a *ClaudeAdapter) HasMCPConfig() bool {
	for _, global := range []bool{true, false} {
		if hasMCPServersEntry(mcpJSONPath(global)) {
			return true
		}
	}
	return false
}

// HasPluginConfig checks if monocle is installed as a Claude Code plugin.
// Returns true if ~/.claude/plugins/installed_plugins.json contains a monocle@* entry
// with user scope (global) or project scope matching the current working directory.
func (a *ClaudeAdapter) HasPluginConfig() bool {
	path := installedPluginsPath()
	data, err := ReadJSONFile(path)
	if err != nil {
		return false
	}
	plugins, ok := data["plugins"].(map[string]any)
	if !ok {
		return false
	}

	cwd, _ := os.Getwd()
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	for key, val := range plugins {
		if !strings.HasPrefix(key, "monocle@") {
			continue
		}
		entries, ok := val.([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			scope, _ := entry["scope"].(string)
			if scope == "user" {
				return true
			}
			if scope == "project" {
				projectPath, _ := entry["projectPath"].(string)
				if resolved, err := filepath.EvalSymlinks(projectPath); err == nil {
					projectPath = resolved
				}
				if projectPath != "" && cwd != "" && strings.HasPrefix(cwd, projectPath) {
					return true
				}
			}
		}
	}
	return false
}

// NeedsRegister returns true if monocle is not configured via .mcp.json or Claude Code plugin.
// This includes cases where an old-style config exists (e.g., pointing to bun/node directly).
func (a *ClaudeAdapter) NeedsRegister() bool {
	return !a.HasMCPConfig() && !a.HasPluginConfig()
}

// RegisterDetails returns info about what was registered.
func (a *ClaudeAdapter) RegisterDetails(global bool) []string {
	return []string{fmt.Sprintf("mcp → %s", mcpJSONPath(global))}
}

// effectiveMode returns the integration mode, defaulting to MCP tools.
func (a *ClaudeAdapter) effectiveMode() IntegrationMode {
	if a.Mode == "" {
		return ModeMCPTools
	}
	return a.Mode
}

// configureMCP adds monocle to .mcp.json.
func (a *ClaudeAdapter) configureMCP(global bool) error {
	args := []string{"serve-mcp", "--experimental-channels"}
	if a.effectiveMode() == ModeSkills {
		args = []string{"serve-mcp", "--experimental-channels-only"}
	}
	return configureMCPServersJSON(mcpJSONPath(global), ResolveCommand(global), args)
}

// unconfigureMCP removes monocle from .mcp.json.
func (a *ClaudeAdapter) unconfigureMCP(global bool) error {
	return unconfigureMCPServersJSON(mcpJSONPath(global))
}

// configureMCPServersJSON adds monocle to a JSON file with an "mcpServers" key.
func configureMCPServersJSON(path, command string, args []string) error {
	data, err := ReadJSONFile(path)
	if err != nil {
		return err
	}

	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
		data["mcpServers"] = servers
	}

	anyArgs := make([]any, len(args))
	for i, a := range args {
		anyArgs[i] = a
	}
	servers["monocle"] = map[string]any{
		"command": command,
		"args":    anyArgs,
	}

	return WriteJSONFile(path, data)
}

// unconfigureMCPServersJSON removes monocle from a JSON file with an "mcpServers" key.
func unconfigureMCPServersJSON(path string) error {
	data, err := ReadJSONFile(path)
	if err != nil {
		return err
	}

	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		return nil
	}

	delete(servers, "monocle")

	if len(servers) == 0 {
		delete(data, "mcpServers")
	}

	if len(data) == 0 {
		return RemoveFileIfExists(path)
	}

	return WriteJSONFile(path, data)
}

// hasMCPServersEntry checks if a JSON file has a monocle entry under "mcpServers"
// with a recognized serve-mcp or serve-mcp-channel subcommand.
func hasMCPServersEntry(path string) bool {
	data, err := ReadJSONFile(path)
	if err != nil {
		return false
	}
	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		return false
	}
	entry, ok := servers["monocle"].(map[string]any)
	if !ok {
		return false
	}
	args, _ := entry["args"].([]any)
	if len(args) > 0 {
		if arg, ok := args[0].(string); ok {
			switch arg {
			case "serve-mcp", "serve-mcp-channel":
				return true
			}
		}
	}
	return false
}

// installedPluginsPath returns the path to Claude Code's installed plugins registry.
func installedPluginsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
}

// claudeSettingsPath returns the path for .claude/settings.json.
func claudeSettingsPath(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "settings.json")
		}
		return filepath.Join(home, ".claude", "settings.json")
	}
	return filepath.Join(".claude", "settings.json")
}

// configureClaudeSettings adds monocle permissions to .claude/settings.json.
func configureClaudeSettings(path string) error {
	data, err := ReadJSONFile(path)
	if err != nil {
		return err
	}

	perms, ok := data["permissions"].(map[string]any)
	if !ok {
		perms = map[string]any{}
		data["permissions"] = perms
	}

	allowRaw, _ := perms["allow"].([]any)
	existing := make(map[string]bool, len(allowRaw))
	for _, v := range allowRaw {
		if s, ok := v.(string); ok {
			existing[s] = true
		}
	}

	for _, perm := range MonocleClaudePermissions {
		if !existing[perm] {
			allowRaw = append(allowRaw, perm)
		}
	}

	perms["allow"] = allowRaw
	return WriteJSONFile(path, data)
}

// unconfigureClaudeSettings removes monocle permissions from .claude/settings.json.
func unconfigureClaudeSettings(path string) error {
	data, err := ReadJSONFile(path)
	if err != nil {
		return err
	}

	perms, ok := data["permissions"].(map[string]any)
	if !ok {
		return nil
	}

	allowRaw, ok := perms["allow"].([]any)
	if !ok {
		return nil
	}

	remove := make(map[string]bool, len(MonocleClaudePermissions))
	for _, perm := range MonocleClaudePermissions {
		remove[perm] = true
	}

	var filtered []any
	for _, v := range allowRaw {
		if s, ok := v.(string); ok && remove[s] {
			continue
		}
		filtered = append(filtered, v)
	}

	if len(filtered) == 0 {
		delete(perms, "allow")
	} else {
		perms["allow"] = filtered
	}
	if len(perms) == 0 {
		delete(data, "permissions")
	}

	if len(data) == 0 {
		return RemoveFileIfExists(path)
	}
	return WriteJSONFile(path, data)
}

// claudeSkillsDir returns the directory for Claude Code skill files.
func claudeSkillsDir(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "skills")
		}
		return filepath.Join(home, ".claude", "skills")
	}
	return filepath.Join(".claude", "skills")
}

// claudeCommandsDir returns the directory for Claude Code command files.
func claudeCommandsDir(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".claude", "commands")
		}
		return filepath.Join(home, ".claude", "commands")
	}
	return filepath.Join(".claude", "commands")
}

// mcpJSONPath returns the path for .mcp.json.
// If global is true, returns ~/.mcp.json; otherwise ./.mcp.json.
func mcpJSONPath(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return ".mcp.json"
		}
		return filepath.Join(home, ".mcp.json")
	}
	return ".mcp.json"
}

