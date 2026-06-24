package tui

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestCategorizeFile(t *testing.T) {
	cases := []struct {
		path string
		want fileCategory
	}{
		{"internal/core/engine.go", categoryCode},
		{"src/components/Chart.tsx", categoryCode},
		{"internal/core/engine_test.go", categoryTest},
		{"src/components/Chart.test.tsx", categoryTest},
		{"api/__tests__/goals.ts", categoryTest},
		{"tests/integration/flow.py", categoryTest},
		{"test_users.py", categoryTest},
		{"README.md", categoryDocs},
		{"docs/configuration/config-file.mdx", categoryDocs},
		{"LICENSE", categoryDocs},
		{"config/settings.yaml", categoryConfig},
		{"tsconfig.json", categoryConfig},
		{".eslintrc", categoryConfig},
		{"Makefile", categoryBuild},
		{"Dockerfile", categoryBuild},
		{".github/workflows/ci.yml", categoryBuild},
		{"go.mod", categoryBuild},
	}
	for _, c := range cases {
		if got := categorizeFile(c.path); got != c.want {
			t.Errorf("categorizeFile(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestGroupFilesByCategoryOrderAndHeaders(t *testing.T) {
	files := []types.ChangedFile{
		{Path: "README.md"},                    // docs
		{Path: "internal/core/engine.go"},      // code
		{Path: "Makefile"},                     // build
		{Path: "internal/core/engine_test.go"}, // test
		{Path: "internal/api/handler.go"},      // code
		{Path: "config.yaml"},                  // config
	}
	ordered, headers := groupFilesByCategory(files)

	// Order must follow categoryOrder: code, test, config, docs, build.
	wantPaths := []string{
		"internal/api/handler.go",      // code (sorted within group)
		"internal/core/engine.go",      // code
		"internal/core/engine_test.go", // test
		"config.yaml",                  // config
		"README.md",                    // docs
		"Makefile",                     // build
	}
	if len(ordered) != len(wantPaths) {
		t.Fatalf("ordered len = %d, want %d", len(ordered), len(wantPaths))
	}
	for i, p := range wantPaths {
		if ordered[i].Path != p {
			t.Errorf("ordered[%d] = %q, want %q", i, ordered[i].Path, p)
		}
	}
	// A header marks the start of each non-empty group: indices 0,2,3,4,5.
	for _, idx := range []int{0, 2, 3, 4, 5} {
		if _, ok := headers[idx]; !ok {
			t.Errorf("expected a group header at display index %d", idx)
		}
	}
	// No header inside the code group's second file.
	if _, ok := headers[1]; ok {
		t.Errorf("did not expect a header at index 1 (mid code group)")
	}
}

func TestSidebarViewModeCycle(t *testing.T) {
	km := DefaultKeyMap()
	s := newSidebarModel(&km)
	s.focused = true
	s.files = []types.ChangedFile{
		{Path: "a.go"}, {Path: "a_test.go"}, {Path: "README.md"},
	}

	cycle := func() {
		out, _ := s.Update(keyPress(rune(km.TreeMode[0][0])))
		s = out
	}

	// flat -> tree
	cycle()
	if !s.treeMode || s.groupMode {
		t.Fatalf("after 1 cycle: treeMode=%v groupMode=%v, want tree", s.treeMode, s.groupMode)
	}
	// tree -> grouped
	cycle()
	if s.treeMode || !s.groupMode {
		t.Fatalf("after 2 cycles: treeMode=%v groupMode=%v, want grouped", s.treeMode, s.groupMode)
	}
	if len(s.groupedFiles) != len(s.files) {
		t.Errorf("grouped files = %d, want %d", len(s.groupedFiles), len(s.files))
	}
	if len(s.groupHeaderAt) == 0 {
		t.Error("expected group headers in grouped mode")
	}
	// grouped -> flat
	cycle()
	if s.treeMode || s.groupMode {
		t.Fatalf("after 3 cycles: treeMode=%v groupMode=%v, want flat", s.treeMode, s.groupMode)
	}
}

func TestGroupedSelectedFileMatchesDisplayOrder(t *testing.T) {
	km := DefaultKeyMap()
	s := newSidebarModel(&km)
	s.files = []types.ChangedFile{
		{Path: "z.go"}, {Path: "a_test.go"}, {Path: "a.go"},
	}
	s.groupMode = true
	s.rebuildGroups()
	// Display order: code (a.go, z.go), then test (a_test.go).
	s.cursor = 0
	if f := s.selectedFile(); f == nil || f.Path != "a.go" {
		t.Errorf("cursor 0 selectedFile = %v, want a.go", f)
	}
	s.cursor = 2
	if f := s.selectedFile(); f == nil || f.Path != "a_test.go" {
		t.Errorf("cursor 2 selectedFile = %v, want a_test.go", f)
	}
}
