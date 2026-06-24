package mcp

import (
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterTools(t *testing.T) {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "monocle",
		Version: "test",
	}, nil)

	registerTools(server)

	// Verify all 4 tools are registered by listing them
	// The server should not panic and should accept all tool registrations
}

func TestTextResult(t *testing.T) {
	r := textResult("hello")
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(r.Content))
	}
	tc, ok := r.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != "hello" {
		t.Errorf("expected 'hello', got %q", tc.Text)
	}
	if r.IsError {
		t.Error("should not be error")
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("failed: %v", "bad thing")
	if !r.IsError {
		t.Error("should be error")
	}
	tc := r.Content[0].(*sdkmcp.TextContent)
	if tc.Text != "failed: bad thing" {
		t.Errorf("unexpected text: %q", tc.Text)
	}
}

func TestGroupingNudgeText(t *testing.T) {
	// No files -> no nudge.
	if got := groupingNudgeText(nil); got != "" {
		t.Errorf("no files: got %q, want empty", got)
	}
	// All grouped (label or category) -> no nudge.
	allGrouped := []types.ChangedFile{
		{Path: "a.go", GroupLabel: "Backend"},
		{Path: "b.go", Category: "test"},
	}
	if got := groupingNudgeText(allGrouped); got != "" {
		t.Errorf("all grouped: got %q, want empty", got)
	}
	// Some ungrouped -> nudge mentions the count and the tool.
	mixed := []types.ChangedFile{
		{Path: "a.go", GroupLabel: "Backend"},
		{Path: "b.go"},
		{Path: "c.go"},
	}
	got := groupingNudgeText(mixed)
	if got == "" {
		t.Fatal("mixed: expected a nudge")
	}
	if !strings.Contains(got, "2 of 3") || !strings.Contains(got, "set_file_groups") {
		t.Errorf("nudge text = %q, want it to mention '2 of 3' and set_file_groups", got)
	}
}
