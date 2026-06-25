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

// resolvedCategoryFor returns the category for a path: the agent-supplied
// override when valid, otherwise the path heuristic.
func resolvedCategoryFor(category, path string) fileCategory {
	if category != "" {
		c := fileCategory(category)
		if _, ok := categoryMetaByID[c]; ok {
			return c
		}
	}
	return categorizeFile(path)
}

// groupItem is the grouping-relevant projection of a file (changed or
// additional), so the grouping logic can be shared across file kinds.
type groupItem struct {
	path       string
	category   string // agent override ("" = heuristic)
	groupLabel string // agent group label ("" = category group)
	groupOrder int
	sortIndex  int
	importRank int // intra-changeset import order (0 if unknown); dependencies first
	churn      int
}

// groupBucket accumulates items for one group during grouping.
type groupBucket[T any] struct {
	label   string
	icon    string
	order   int
	isAgent bool
	items   []T
}

// groupItemsBy reorders items into groups and returns the flattened slice plus a
// map from display index to the header shown before that item.
//
// Grouping key is the agent group label when present, otherwise the resolved
// category. Agent-labeled groups sort first (by group order), then category
// groups in the fixed category order. Within a group, items sort by agent sort
// index, then intra-changeset import order (dependencies first), then churn
// descending, then path.
func groupItemsBy[T any](items []T, proj func(T) groupItem) ([]T, map[int]string) {
	groups := map[string]*groupBucket[T]{}
	var keys []string

	for _, it := range items {
		gi := proj(it)
		var key string
		if gi.groupLabel != "" {
			key = "label:" + gi.groupLabel
			if groups[key] == nil {
				groups[key] = &groupBucket[T]{label: gi.groupLabel, icon: "", order: gi.groupOrder, isAgent: true}
				keys = append(keys, key)
			}
			b := groups[key]
			if gi.groupOrder < b.order { // group order = smallest among its files
				b.order = gi.groupOrder
			}
			b.items = append(b.items, it)
		} else {
			cat := resolvedCategoryFor(gi.category, gi.path)
			key = "cat:" + string(cat)
			if groups[key] == nil {
				meta := cat.meta()
				groups[key] = &groupBucket[T]{label: meta.label, icon: meta.icon, order: categoryIndex(cat)}
				keys = append(keys, key)
			}
			groups[key].items = append(groups[key].items, it)
		}
	}

	ordered := make([]*groupBucket[T], 0, len(keys))
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

	flat := make([]T, 0, len(items))
	headers := map[int]string{}
	for _, b := range ordered {
		sort.SliceStable(b.items, func(i, j int) bool {
			x, y := proj(b.items[i]), proj(b.items[j])
			if x.sortIndex != y.sortIndex {
				return x.sortIndex < y.sortIndex
			}
			if x.importRank != y.importRank {
				return x.importRank < y.importRank // dependencies (lower rank) first
			}
			if x.churn != y.churn {
				return x.churn > y.churn // bigger churn first
			}
			return x.path < y.path
		})
		headers[len(flat)] = fmt.Sprintf("%s %s  %d", b.icon, b.label, len(b.items))
		flat = append(flat, b.items...)
	}
	return flat, headers
}

// groupFiles reorders changed files for the grouped view.
func groupFiles(files []types.ChangedFile) ([]types.ChangedFile, map[int]string) {
	return groupItemsBy(files, func(f types.ChangedFile) groupItem {
		return groupItem{
			path:       f.Path,
			category:   f.Category,
			groupLabel: f.GroupLabel,
			groupOrder: f.GroupOrder,
			sortIndex:  f.SortIndex,
			importRank: f.ImportOrder,
			churn:      f.Additions + f.Deletions,
		}
	})
}

// groupAdditionalFiles reorders agent-attached additional files for the grouped
// view, using the same grouping rules (additional files carry no churn).
func groupAdditionalFiles(files []types.AdditionalFile) ([]types.AdditionalFile, map[int]string) {
	return groupItemsBy(files, func(f types.AdditionalFile) groupItem {
		return groupItem{
			path:       f.Name,
			category:   f.Category,
			groupLabel: f.GroupLabel,
			groupOrder: f.GroupOrder,
			sortIndex:  f.SortIndex,
		}
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
