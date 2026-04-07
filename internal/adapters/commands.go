package adapters

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed commands.json
var commandsJSON []byte

// CommandDef describes a slash command that wraps an MCP tool call.
type CommandDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// loadCommands parses the embedded command definitions.
func loadCommands() ([]CommandDef, error) {
	var defs []CommandDef
	if err := json.Unmarshal(commandsJSON, &defs); err != nil {
		return nil, fmt.Errorf("parse commands.json: %w", err)
	}
	return defs, nil
}

// CommandNames returns the names of all defined commands.
func CommandNames() []string {
	defs, _ := loadCommands()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

// InstallMarkdownCommands writes command files in markdown format (Claude Code, OpenCode).
// Each command becomes dir/<name>.md with YAML frontmatter.
func InstallMarkdownCommands(dir string) error {
	defs, err := loadCommands()
	if err != nil {
		return err
	}
	for _, cmd := range defs {
		content := fmt.Sprintf("---\ndescription: %s\n---\n\n%s\n", cmd.Description, cmd.Body)
		dest := filepath.Join(dir, cmd.Name+".md")
		if err := WriteFileAtomic(dest, []byte(content)); err != nil {
			return fmt.Errorf("write command %s: %w", cmd.Name, err)
		}
	}
	return nil
}

// InstallTOMLCommands writes command files in TOML format (Gemini CLI).
// Each command becomes dir/<name>.toml with description and prompt fields.
func InstallTOMLCommands(dir string) error {
	defs, err := loadCommands()
	if err != nil {
		return err
	}
	for _, cmd := range defs {
		// Escape any triple quotes in body for TOML multi-line strings
		body := strings.ReplaceAll(cmd.Body, `"""`, `\"\"\"`)
		content := fmt.Sprintf("description = %q\nprompt = \"\"\"\n%s\n\"\"\"\n", cmd.Description, body)
		dest := filepath.Join(dir, cmd.Name+".toml")
		if err := WriteFileAtomic(dest, []byte(content)); err != nil {
			return fmt.Errorf("write command %s: %w", cmd.Name, err)
		}
	}
	return nil
}

// RemoveCommands removes installed command files with the given extension.
func RemoveCommands(dir string, ext string) {
	defs, _ := loadCommands()
	for _, cmd := range defs {
		_ = RemoveFileIfExists(filepath.Join(dir, cmd.Name+ext))
	}
	// Remove dir if empty
	_ = os.Remove(dir)
}

// CommandPaths returns the paths of command files that would be installed.
func CommandPaths(dir string, ext string) []string {
	defs, _ := loadCommands()
	paths := make([]string, len(defs))
	for i, d := range defs {
		paths[i] = filepath.Join(dir, d.Name+ext)
	}
	return paths
}
