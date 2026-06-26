package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestAutoToggleSidebar_KeepsWhenFiltered(t *testing.T) {
	// A review filter narrowed the lists to empty — the sidebar must stay put.
	m := appModel{}
	m.sidebar.reviewFilter = "reviewed"
	if changed := m.autoToggleSidebar(); changed {
		t.Error("autoToggleSidebar should report no change while a filter is active")
	}
	if m.sidebarHidden || m.focus == focusMain {
		t.Error("sidebar should not auto-hide / lose focus while a review filter is active")
	}
}

func TestAutoToggleSidebar_HidesWhenGenuinelyEmpty(t *testing.T) {
	// No filter and no items at all — auto-hide is correct.
	m := appModel{focus: focusSidebar}
	m.sidebar.reviewFilter = ""
	if changed := m.autoToggleSidebar(); !changed {
		t.Error("autoToggleSidebar should hide a genuinely empty sidebar")
	}
	if !m.sidebarHidden {
		t.Error("sidebar should be hidden when genuinely empty")
	}
}

func TestSidebarSectionRulesAndWorkstreamSpacing(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.width = 44
	m.height = 40
	m.groupMode = true
	m.contentItems = []types.ContentItem{{ID: "p1", Title: "Plan"}}
	m.files = []types.ChangedFile{
		{Path: "search/ui.tsx", Workstream: "Search", WorkstreamOrder: 0, GroupLabel: "UI"},
		{Path: "payments/api.go", Workstream: "Payments", WorkstreamOrder: 1, GroupLabel: "Backend"},
	}
	m.applyReviewedFilter()
	m.rebuildGroups()

	out := stripANSISeq(m.View())

	// A horizontal rule separates the Artifacts and Files sections.
	if !strings.Contains(out, "───") {
		t.Errorf("expected a horizontal rule between sections, got:\n%s", out)
	}
	if !strings.Contains(out, "Search") || !strings.Contains(out, "Payments") {
		t.Error("expected both workstream headers to render")
	}
	// A blank line separates the two top-level workstreams: find the Payments
	// header line and confirm the preceding rendered line is empty.
	lines := strings.Split(out, "\n")
	payIdx := -1
	for i, ln := range lines {
		if strings.Contains(ln, "Payments") {
			payIdx = i
		}
	}
	if payIdx <= 0 {
		t.Fatal("Payments workstream header not found")
	}
	if strings.TrimSpace(lines[payIdx-1]) != "" {
		t.Errorf("expected a blank line before the Payments workstream, prev line: %q", lines[payIdx-1])
	}
}

// stripANSISeq removes ANSI escape sequences for plain-text assertions.
func stripANSISeq(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && (r == 'm'):
			inEsc = false
		case inEsc:
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestCycleReviewFilter_TrackingEnabled(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true

	if m.reviewFilter != "" {
		t.Fatalf("expected empty initial filter, got %q", m.reviewFilter)
	}
	m.cycleReviewFilter()
	if m.reviewFilter != "unreviewed" {
		t.Errorf("expected 'unreviewed', got %q", m.reviewFilter)
	}
	m.cycleReviewFilter()
	if m.reviewFilter != "reviewed" {
		t.Errorf("expected 'reviewed', got %q", m.reviewFilter)
	}
	m.cycleReviewFilter()
	if m.reviewFilter != "" {
		t.Errorf("expected empty after full cycle, got %q", m.reviewFilter)
	}
}

func TestCycleReviewFilter_TrackingDisabled(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = false

	m.cycleReviewFilter()
	if m.reviewFilter != "" {
		t.Errorf("expected filter to stay empty when tracking disabled, got %q", m.reviewFilter)
	}
}

func TestApplyReviewedFilter_UnreviewedOnly(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true
	m.reviewFilter = "unreviewed"

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: false},
		{Path: "b.go", Reviewed: true},
		{Path: "c.go", Reviewed: false},
	}
	m.contentItems = []types.ContentItem{
		{ID: "p1", Reviewed: false},
		{ID: "p2", Reviewed: true},
	}
	m.additionalFiles = []types.AdditionalFile{
		{Path: "extra.go", Reviewed: true},
	}

	m.applyReviewedFilter()

	if len(m.files) != 2 {
		t.Errorf("expected 2 unreviewed files, got %d", len(m.files))
	}
	if len(m.contentItems) != 1 {
		t.Errorf("expected 1 unreviewed content item, got %d", len(m.contentItems))
	}
	if len(m.additionalFiles) != 0 {
		t.Errorf("expected 0 unreviewed additional files, got %d", len(m.additionalFiles))
	}
}

func TestApplyReviewedFilter_ReviewedOnly(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true
	m.reviewFilter = "reviewed"

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: false},
		{Path: "b.go", Reviewed: true},
	}

	m.applyReviewedFilter()

	if len(m.files) != 1 {
		t.Errorf("expected 1 reviewed file, got %d", len(m.files))
	}
	if m.files[0].Path != "b.go" {
		t.Errorf("expected b.go, got %s", m.files[0].Path)
	}
}

func TestApplyReviewedFilter_NoFilter(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true
	m.reviewFilter = ""

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: false},
		{Path: "b.go", Reviewed: true},
	}

	m.applyReviewedFilter()

	if len(m.files) != 2 {
		t.Errorf("expected all 2 files with no filter, got %d", len(m.files))
	}
}

func TestNextUnreviewed_FindsNext(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true
	m.width = 80
	m.height = 40

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: true},
		{Path: "b.go", Reviewed: false},
		{Path: "c.go", Reviewed: true},
	}
	m.cursor = 0

	cmd := m.nextUnreviewed()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1, got %d", m.cursor)
	}
}

func TestNextUnreviewed_NoneLeft(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = true

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: true},
		{Path: "b.go", Reviewed: true},
	}
	m.cursor = 0

	cmd := m.nextUnreviewed()
	if cmd != nil {
		t.Error("expected nil when all files are reviewed")
	}
}

func TestNextUnreviewed_TrackingDisabled(t *testing.T) {
	keys := DefaultKeyMap()
	m := newSidebarModel(&keys)
	m.reviewTracking = false

	m.files = []types.ChangedFile{
		{Path: "a.go", Reviewed: false},
		{Path: "b.go", Reviewed: false},
	}
	m.cursor = 0

	cmd := m.nextUnreviewed()
	if cmd != nil {
		t.Error("expected nil when tracking is disabled")
	}
}

func TestReviewedKeyNoop_TrackingDisabled(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{ReviewTracking: false},
		session: &types.ReviewSession{
			ID: "test",
			ChangedFiles: []types.ChangedFile{
				{Path: "a.go", Reviewed: false},
			},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40
	m.sidebar.files = engine.session.ChangedFiles

	// The r key should be a no-op
	result, _ := m.Update(keyPress('r'))
	app := result.(appModel)

	// File should remain unreviewed
	if len(app.sidebar.files) > 0 && app.sidebar.files[0].Reviewed {
		t.Error("expected file to remain unreviewed when tracking is disabled")
	}
}

func TestFilterKeyNoop_TrackingDisabled(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{ReviewTracking: false},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	// The / key should be a no-op
	result, _ := m.Update(keyPress('/'))
	app := result.(appModel)

	if app.sidebar.reviewFilter != "" {
		t.Errorf("expected filter to stay empty, got %q", app.sidebar.reviewFilter)
	}
}

func TestReviewedKeyWorks_TrackingEnabled(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{ReviewTracking: true},
		session: &types.ReviewSession{
			ID:           "test",
			FileStatuses: make(map[string]bool),
			ChangedFiles: []types.ChangedFile{
				{Path: "a.go", Reviewed: false},
			},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40
	m.sidebar.files = engine.session.ChangedFiles

	// The / key should cycle the filter
	result, _ := m.Update(keyPress('/'))
	app := result.(appModel)

	if app.sidebar.reviewFilter != "unreviewed" {
		t.Errorf("expected filter to be 'unreviewed', got %q", app.sidebar.reviewFilter)
	}
}
