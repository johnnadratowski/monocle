package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// fileCategory is the fixed, icon-bearing classification of a changed file. It is
// orthogonal to an agent-supplied free-form group label: the category drives the
// icon and the heuristic grouping, while a group label (when present) drives the
// stack-layer narrative. See the grouped sidebar view.
type fileCategory string

const (
	categoryCode   fileCategory = "code"
	categoryTest   fileCategory = "test"
	categoryConfig fileCategory = "config"
	categoryDocs   fileCategory = "docs"
	categoryBuild  fileCategory = "build"
	categoryOther  fileCategory = "other"
)

// categoryOrder is the fixed display order of category groups: the code first
// (the substance of the change), then tests, config, docs, build, and anything
// uncategorized last.
var categoryOrder = []fileCategory{
	categoryCode, categoryTest, categoryConfig, categoryDocs, categoryBuild, categoryOther,
}

// categoryMeta holds the display label and icon for a category.
type categoryMeta struct {
	label string
	icon  string
}

var categoryMetaByID = map[fileCategory]categoryMeta{
	categoryCode:   {"Code", "󰅩"},
	categoryTest:   {"Tests", "󰙨"},
	categoryConfig: {"Config", ""},
	categoryDocs:   {"Docs", ""},
	categoryBuild:  {"Build", "󱌣"},
	categoryOther:  {"Other", ""},
}

func (c fileCategory) meta() categoryMeta {
	if m, ok := categoryMetaByID[c]; ok {
		return m
	}
	return categoryMetaByID[categoryOther]
}

// categorizeFile classifies a path into a fixed category using cheap, predictable
// path heuristics. Checks run most-specific first (tests before code, etc.).
func categorizeFile(path string) fileCategory {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	dir := "/" + strings.Trim(strings.ToLower(filepath.ToSlash(filepath.Dir(path))), "/") + "/"

	// Tests first — they often share an extension with code.
	if strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(dir, "/test/") || strings.Contains(dir, "/tests/") ||
		strings.Contains(dir, "/__tests__/") || strings.Contains(dir, "/spec/") {
		return categoryTest
	}

	// Build / CI / dependency manifests.
	switch base {
	case "makefile", "dockerfile", "go.mod", "go.sum", "package-lock.json",
		"yarn.lock", "pnpm-lock.yaml", "cargo.lock", ".goreleaser.yaml",
		".goreleaser.yml", "build.gradle", "pom.xml", "devbox.json", "lefthook.yml":
		return categoryBuild
	}
	if strings.Contains(dir, "/.github/") || strings.HasSuffix(lower, ".mk") {
		return categoryBuild
	}

	// Docs.
	switch ext {
	case ".md", ".mdx", ".rst", ".txt", ".adoc":
		return categoryDocs
	}
	if strings.Contains(dir, "/docs/") || strings.HasPrefix(base, "readme") ||
		strings.HasPrefix(base, "license") || strings.HasPrefix(base, "changelog") {
		return categoryDocs
	}

	// Config (extensions and well-known dotfiles).
	switch ext {
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf", ".env", ".properties":
		return categoryConfig
	}
	if strings.HasPrefix(base, ".") { // dotfiles like .eslintrc, .gitignore
		return categoryConfig
	}

	return categoryCode
}

// resolvedCategory returns the file's category: the agent-supplied override when
// valid, otherwise the path heuristic.
func resolvedCategory(f types.ChangedFile) fileCategory {
	if f.Category != "" {
		c := fileCategory(f.Category)
		if _, ok := categoryMetaByID[c]; ok {
			return c
		}
	}
	return categorizeFile(f.Path)
}

// fileChurnTotal is the file's churn used for the lines-changed fallback sort.
func fileChurnTotal(f types.ChangedFile) int { return f.Additions + f.Deletions }

// fileGroup is an ordered group of files with a header for the grouped view.
type fileGroup struct {
	key     string // grouping key (agent group label, or category id)
	label   string // rendered header label
	icon    string // header icon
	order   int    // primary sort order among groups
	isAgent bool   // true when formed from an agent-supplied group label
	files   []types.ChangedFile
}

// groupFiles reorders files for the grouped view and returns the flattened slice
// plus a map from display index to the header rendered before that file.
//
// Grouping key is the agent-supplied GroupLabel when present, otherwise the
// resolved category. Agent-labeled groups sort first by their GroupOrder, then
// category groups follow in the fixed categoryOrder. Within a group, files sort
// by the agent SortIndex when given, else by churn (lines changed) descending,
// then path.
func groupFiles(files []types.ChangedFile) ([]types.ChangedFile, map[int]string) {
	groups := map[string]*fileGroup{}
	var keys []string

	for _, f := range files {
		var key string
		var g *fileGroup
		if f.GroupLabel != "" {
			key = "label:" + f.GroupLabel
			if groups[key] == nil {
				g = &fileGroup{key: key, label: f.GroupLabel, icon: "", order: f.GroupOrder, isAgent: true}
				groups[key] = g
				keys = append(keys, key)
			}
			g = groups[key]
			// A group's order is the smallest GroupOrder among its files.
			if f.GroupOrder < g.order {
				g.order = f.GroupOrder
			}
		} else {
			cat := resolvedCategory(f)
			key = "cat:" + string(cat)
			if groups[key] == nil {
				meta := cat.meta()
				g = &fileGroup{key: key, label: meta.label, icon: meta.icon, order: categoryIndex(cat)}
				groups[key] = g
				keys = append(keys, key)
			}
			g = groups[key]
		}
		g.files = append(g.files, f)
	}

	// Order groups: agent-labeled first (by order), then category groups (by the
	// fixed category order). Stable on label for ties.
	ordered := make([]*fileGroup, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, groups[k])
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.isAgent != b.isAgent {
			return a.isAgent // agent groups first
		}
		if a.order != b.order {
			return a.order < b.order
		}
		return a.label < b.label
	})

	flat := make([]types.ChangedFile, 0, len(files))
	headers := map[int]string{}
	for _, g := range ordered {
		sortGroupFiles(g.files)
		headers[len(flat)] = fmt.Sprintf("%s %s  %d", g.icon, g.label, len(g.files))
		flat = append(flat, g.files...)
	}
	return flat, headers
}

// sortGroupFiles orders files within a group: agent SortIndex first (when any are
// set), else churn descending, then path.
func sortGroupFiles(files []types.ChangedFile) {
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.SortIndex != b.SortIndex {
			return a.SortIndex < b.SortIndex
		}
		if ca, cb := fileChurnTotal(a), fileChurnTotal(b); ca != cb {
			return ca > cb // bigger churn first
		}
		return a.Path < b.Path
	})
}

// categoryIndex returns a category's position in categoryOrder (offset so all
// category groups sort after agent groups). Unknown categories sort last.
func categoryIndex(c fileCategory) int {
	for i, cat := range categoryOrder {
		if cat == c {
			return i
		}
	}
	return len(categoryOrder)
}
