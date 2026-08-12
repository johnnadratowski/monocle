package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The keybinding docs live in four places — the in-app help (H), README.md,
// docs/reference/keybindings.mdx and docs/configuration/keybindings.mdx — and
// nothing but diligence kept them in step with the code. It didn't: a
// `dismiss_outdated` action was documented as configurable for months without
// existing, README's action list had drifted ~25 entries behind, and `q` was
// missing from both key tables. These tests make the drift fail the build
// instead.

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// backticked pulls every `code span` out of a document. Presence is all these
// tests check — placement and wording stay a human matter.
func backticked(doc string) map[string]bool {
	out := map[string]bool{}
	for _, m := range regexp.MustCompile("`([^`\n]+)`").FindAllStringSubmatch(doc, -1) {
		out[m[1]] = true
	}
	return out
}

// normalizeKey folds the spelling differences between code and prose: the code
// stores "ctrl+g" and "enter", the docs write "Ctrl+G", "Enter" and the arrow
// glyphs. Single characters keep their case, because `n` and `N` are different
// bindings.
func normalizeKey(k string) string {
	if alias, ok := map[string]string{"←": "left", "→": "right", "↑": "up", "↓": "down"}[k]; ok {
		return alias
	}
	if len([]rune(k)) > 1 {
		return strings.ToLower(k)
	}
	return k
}

func TestDocsListEveryKeyAction(t *testing.T) {
	canonical := map[string]bool{}
	for _, a := range ActionNames() {
		canonical[a] = true
	}

	// README lists the actions in one sentence; the config page has a table row
	// each. Scoping to those regions keeps unrelated snake_case prose — MCP tool
	// names, config field names — from reading as action names.
	scope := map[string]func(string) string{
		"README.md": func(doc string) string {
			i := strings.Index(doc, "Available action names:")
			if i < 0 {
				return ""
			}
			return doc[i : i+strings.Index(doc[i:], "\n")]
		},
		// Only the table's first column — later columns name config fields
		// (`editor_mode`, `media_viewer`) that are not keybinding actions.
		"docs/configuration/keybindings.mdx": func(doc string) string {
			var b strings.Builder
			for _, m := range regexp.MustCompile(`(?m)^\| (`+"`"+`[a-z_]+`+"`"+`) \|`).FindAllStringSubmatch(doc, -1) {
				b.WriteString(m[1] + "\n")
			}
			return b.String()
		},
	}

	for _, rel := range []string{"README.md", "docs/configuration/keybindings.mdx"} {
		t.Run(rel, func(t *testing.T) {
			region := scope[rel](repoFile(t, rel))
			if region == "" {
				t.Fatalf("%s no longer contains the action-name list", rel)
			}
			documented := backticked(region)
			var missing, unknown []string
			for a := range canonical {
				if !documented[a] {
					missing = append(missing, a)
				}
			}
			// Only flag unknown names that LOOK like actions, so ordinary prose
			// in snake_case (config field names, file paths) doesn't trip this.
			for d := range documented {
				if canonical[d] || !regexp.MustCompile(`^[a-z]+(_[a-z]+)+$`).MatchString(d) {
					continue
				}
				unknown = append(unknown, d)
			}
			sort.Strings(missing)
			sort.Strings(unknown)
			if len(missing) > 0 {
				t.Errorf("configurable actions missing from %s: %v", rel, missing)
			}
			if len(unknown) > 0 {
				t.Errorf("%s documents actions that don't exist: %v", rel, unknown)
			}
		})
	}
}

// hardcodedKeys are the bindings the KeyMap doesn't own — they can't be
// rebound, so they have to be listed here to be checked. Adding one to the code
// without adding it here is the one hole left, and it is a small one.
var hardcodedKeys = []string{
	"h", "l", "left", "right", // horizontal diff scrolling
	"I",         // connection info
	"space",     // expand/collapse a comment
	"d",         // delete a comment (on a comment line)
	"esc",       // close a modal / leave visual or search mode
	"shift+tab", // reverse pane focus
	"ctrl+y",    // copy the review to the clipboard
}

func TestDocsMentionEveryDefaultKey(t *testing.T) {
	want := map[string]bool{}
	km := reflect.ValueOf(DefaultKeyMap())
	for i := 0; i < km.NumField(); i++ {
		f := km.Field(i)
		if f.Kind() != reflect.Slice {
			continue // FocusPaneN is a map; 1/2 are covered by hardcodedKeys' peers
		}
		for j := 0; j < f.Len(); j++ {
			want[normalizeKey(f.Index(j).String())] = true
		}
	}
	for _, k := range hardcodedKeys {
		want[normalizeKey(k)] = true
	}
	// Aliases the docs deliberately fold into their primary key: the arrow
	// equivalents of j/k, and the raw " " spelling of the space bar.
	for _, skip := range []string{"up", "down", " "} {
		delete(want, skip)
	}

	// The register/unregister wizard is a separate TUI. Its keys are documented
	// on the reference page but deliberately left out of README's table, which
	// covers the review TUI.
	wizardOnly := map[string]bool{"backspace": true}

	for _, rel := range []string{"README.md", "docs/reference/keybindings.mdx"} {
		t.Run(rel, func(t *testing.T) {
			documented := map[string]bool{}
			for k := range backticked(repoFile(t, rel)) {
				documented[normalizeKey(k)] = true
				// Compound cells like `Ctrl+d`/`u` or `j`/`k`.
				for _, part := range strings.Split(k, "/") {
					documented[normalizeKey(strings.TrimSpace(part))] = true
				}
			}
			var missing []string
			for k := range want {
				if rel == "README.md" && wizardOnly[k] {
					continue
				}
				if !documented[k] {
					missing = append(missing, k)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("keys bound in the code but undocumented in %s: %v", rel, missing)
			}
		})
	}
}

func TestHelpModalListsEveryDefaultKey(t *testing.T) {
	// The H modal renders Label(km.X) for most bindings, so instead of parsing
	// the rendered output this checks the source references every KeyMap field
	// that has a default — the field name is what would be forgotten.
	src := repoFile(t, "internal/tui/help.go")
	km := reflect.ValueOf(DefaultKeyMap())
	typ := km.Type()

	// Wizard bindings belong to the register TUI, which has no H modal.
	skip := map[string]bool{"WizardAdvance": true, "WizardBack": true, "WizardToggle": true}

	var missing []string
	for i := 0; i < km.NumField(); i++ {
		name := typ.Field(i).Name
		if skip[name] || km.Field(i).Kind() != reflect.Slice || km.Field(i).Len() == 0 {
			continue
		}
		if !strings.Contains(src, "km."+name) {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("bindings missing from the H help modal: %v", missing)
	}
}

func TestDocsListEveryCommand(t *testing.T) {
	for _, rel := range []string{"README.md", "docs/reference/keybindings.mdx"} {
		t.Run(rel, func(t *testing.T) {
			doc := repoFile(t, rel)
			var missing []string
			for _, c := range commandNames {
				// `:ref` and `:theme` take an argument, so the docs spell them
				// as `:ref <rev>` / `:theme [name]`.
				if !strings.Contains(doc, ":"+c) {
					missing = append(missing, ":"+c)
				}
			}
			if len(missing) > 0 {
				t.Errorf("commands missing from %s: %v", rel, missing)
			}
		})
	}
}
