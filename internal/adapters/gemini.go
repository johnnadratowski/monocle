package adapters

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GeminiAdapter handles Monocle registration for Google Gemini CLI.
type GeminiAdapter struct {
	Mode IntegrationMode
}

func (a *GeminiAdapter) Name() string  { return "gemini" }
func (a *GeminiAdapter) Label() string { return "Gemini CLI" }

func (a *GeminiAdapter) ConfigPaths(global bool) []string {
	if a.Mode == ModeMCPTools {
		paths := []string{geminiConfigPath(global)}
		paths = append(paths, CommandPaths(geminiCommandsDir(global), ".toml")...)
		return paths
	}
	paths := []string{geminiPolicyPath(global)}
	paths = append(paths, SkillPaths(geminiSkillsDir(global))...)
	return paths
}

func (a *GeminiAdapter) HasConfig(global bool) bool {
	// Check for skill files
	dir := geminiSkillsDir(global)
	for _, name := range SkillNames {
		if _, err := os.Stat(filepath.Join(dir, name, "SKILL.md")); err == nil {
			return true
		}
	}
	// Also detect legacy MCP config
	return hasMCPServersEntry(geminiConfigPath(global))
}

func (a *GeminiAdapter) Register(global bool) error {
	// Clean up legacy MCP config if present
	_ = unconfigureMCPServersJSON(geminiConfigPath(global))

	if a.Mode == ModeMCPTools {
		if err := configureMCPServersJSON(geminiConfigPath(global), ResolveCommand(global), []string{"serve-mcp"}); err != nil {
			return fmt.Errorf("configure mcp: %w", err)
		}
		return InstallTOMLCommands(geminiCommandsDir(global))
	}

	// Remove legacy command files
	cmdDir := geminiCommandsDir(global)
	for _, name := range SkillNames {
		_ = RemoveFileIfExists(filepath.Join(cmdDir, name+".toml"))
	}

	if err := configureGeminiPolicy(geminiPolicyPath(global)); err != nil {
		return fmt.Errorf("configure policy: %w", err)
	}

	return InstallSkills(geminiSkillsDir(global))
}

func (a *GeminiAdapter) Unregister(global bool) error {
	// Remove legacy MCP config if present
	_ = unconfigureMCPServersJSON(geminiConfigPath(global))

	_ = unconfigureGeminiPolicy(geminiPolicyPath(global))

	RemoveSkills(geminiSkillsDir(global))
	RemoveCommands(geminiCommandsDir(global), ".toml")

	return nil
}

// Detect returns true if Gemini CLI appears to be installed.
func (a *GeminiAdapter) Detect() bool {
	if _, err := exec.LookPath("gemini"); err == nil {
		return true
	}
	if _, err := os.Stat(".gemini"); err == nil {
		return true
	}
	return false
}

func geminiSkillsDir(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".gemini", "skills")
		}
	}
	return filepath.Join(".gemini", "skills")
}

func geminiPolicyPath(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".gemini", "policies", "monocle.toml")
		}
	}
	return filepath.Join(".gemini", "policies", "monocle.toml")
}

const geminiMonoclePolicy = `[[rule]]
toolName = "run_shell_command"
commandPrefix = "monocle"
decision = "allow"
`

func configureGeminiPolicy(path string) error {
	return WriteFileAtomic(path, []byte(geminiMonoclePolicy))
}

func unconfigureGeminiPolicy(path string) error {
	return RemoveFileIfExists(path)
}

func geminiConfigPath(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".gemini", "settings.json")
		}
	}
	return filepath.Join(".gemini", "settings.json")
}

func geminiCommandsDir(global bool) string {
	if global {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, ".gemini", "commands")
		}
	}
	return filepath.Join(".gemini", "commands")
}
