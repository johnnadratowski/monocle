package core

import (
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestTextDiff_IdenticalContent(t *testing.T) {
	hunks, err := TextDiff("hello\nworld\n", "hello\nworld\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 0 {
		t.Fatalf("expected no hunks for identical content, got %d", len(hunks))
	}
}

func TestTextDiff_SimpleChange(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\nmodified\nline3\n"

	hunks, err := TextDiff(old, new)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) == 0 {
		t.Fatal("expected hunks for changed content")
	}

	// Should have removed and added lines
	var hasRemoved, hasAdded bool
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == types.DiffLineRemoved {
				hasRemoved = true
			}
			if l.Kind == types.DiffLineAdded {
				hasAdded = true
			}
		}
	}
	if !hasRemoved || !hasAdded {
		t.Fatal("expected both removed and added lines")
	}
}

func TestTextDiff_Addition(t *testing.T) {
	old := "line1\nline2\n"
	new := "line1\nline2\nline3\nline4\n"

	hunks, err := TextDiff(old, new)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) == 0 {
		t.Fatal("expected hunks for added content")
	}

	var addedCount int
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == types.DiffLineAdded {
				addedCount++
			}
		}
	}
	if addedCount < 2 {
		t.Fatalf("expected at least 2 added lines, got %d", addedCount)
	}
}

func TestTextDiff_EmptyOld(t *testing.T) {
	hunks, err := TextDiff("", "new content\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) == 0 {
		t.Fatal("expected hunks when old content is empty")
	}
}

func TestTextDiff_EmptyNew(t *testing.T) {
	hunks, err := TextDiff("old content\n", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) == 0 {
		t.Fatal("expected hunks when new content is empty")
	}
}
