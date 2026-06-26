package tui

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestTitleBarShowsVersions(t *testing.T) {
	// Matching versions: show the client version, no mismatch warning.
	m := appModel{width: 120, clientVersion: "1.2.3", serverVersion: "1.2.3"}
	out := m.renderTitleBar()
	if !strings.Contains(out, "monocle") {
		t.Error("title bar should contain the app name")
	}
	if !strings.Contains(out, "1.2.3") {
		t.Error("title bar should show the client version")
	}
	if strings.Contains(out, "server") {
		t.Error("matching versions should not show a server mismatch warning")
	}
}

func TestTitleBarVersionMismatch(t *testing.T) {
	m := appModel{width: 120, clientVersion: "1.2.3", serverVersion: "1.0.0"}
	out := m.renderTitleBar()
	if !strings.Contains(out, "server 1.0.0") {
		t.Errorf("mismatch should surface the server version, got: %q", out)
	}
	if !strings.Contains(out, "⚠") {
		t.Error("mismatch should show a warning icon")
	}
}

func TestReviewMetaChurnAndName(t *testing.T) {
	m := appModel{width: 200, headHash: "abc1234"}
	m.sidebar.contentItems = []types.ContentItem{{ID: "p1", Title: "Refactor auth"}}
	m.sidebar.files = []types.ChangedFile{
		{Path: "a.go", Additions: 10, Deletions: 2},
		{Path: "b.go", Additions: 5, Deletions: 1},
	}
	out := m.renderTitleBar()
	for _, want := range []string{"Refactor auth", "+15", "-3", "2 files", "abc1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("title bar should contain %q, got: %q", want, out)
		}
	}
}
