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

// groupedFile pairs a changed file with its resolved category for display.
type groupedFile struct {
	file     types.ChangedFile
	category fileCategory
}

// groupFilesByCategory returns files reordered into category groups (in
// categoryOrder), and a map from the resulting display index to the group header
// label shown before that file. Within a group, files are sorted by path. Empty
// categories are skipped.
func groupFilesByCategory(files []types.ChangedFile) ([]types.ChangedFile, map[int]string) {
	buckets := map[fileCategory][]types.ChangedFile{}
	for _, f := range files {
		c := categorizeFile(f.Path)
		buckets[c] = append(buckets[c], f)
	}
	ordered := make([]types.ChangedFile, 0, len(files))
	headers := map[int]string{}
	for _, cat := range categoryOrder {
		group := buckets[cat]
		if len(group) == 0 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].Path < group[j].Path })
		meta := cat.meta()
		headers[len(ordered)] = fmt.Sprintf("%s %s  %d", meta.icon, meta.label, len(group))
		ordered = append(ordered, group...)
	}
	return ordered, headers
}
