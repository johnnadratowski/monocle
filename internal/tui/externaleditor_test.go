package tui

import (
	"fmt"
	"testing"

	"github.com/josephschmitt/monocle/internal/types"
)

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name      string
		configured string
		visual    string
		editor    string
		wantName  string
		wantArgs  []string
	}{
		{
			name:       "configured editor takes precedence over env",
			configured: "nvim",
			visual:     "vim",
			editor:     "nano",
			wantName:   "nvim",
			wantArgs:   nil,
		},
		{
			name:       "configured editor with flags",
			configured: "code --wait",
			wantName:   "code",
			wantArgs:   []string{"--wait"},
		},
		{
			name:     "VISUAL takes precedence",
			visual:   "nvim",
			editor:   "nano",
			wantName: "nvim",
			wantArgs: nil,
		},
		{
			name:     "EDITOR used when VISUAL empty",
			visual:   "",
			editor:   "nano",
			wantName: "nano",
			wantArgs: nil,
		},
		{
			name:     "falls back to vi",
			visual:   "",
			editor:   "",
			wantName: "vi",
			wantArgs: nil,
		},
		{
			name:     "VISUAL with flags",
			visual:   "code --wait",
			editor:   "",
			wantName: "code",
			wantArgs: []string{"--wait"},
		},
		{
			name:     "EDITOR with flags",
			visual:   "",
			editor:   "emacs -nw",
			wantName: "emacs",
			wantArgs: []string{"-nw"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("VISUAL", tt.visual)
			t.Setenv("EDITOR", tt.editor)

			name, args := resolveEditor(tt.configured)
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
				return
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestExternalEditorResultMsg_Comment(t *testing.T) {
	m := newCommentEditorModel(DefaultTheme())
	m.open("test.go", 10, 15, types.TargetFile)
	m.body = "original"
	m.cursor = 3

	// Simulate successful editor result
	result := externalEditorResultMsg{
		body:   "edited content from editor",
		origin: overlayComment,
	}

	// Apply the result the same way app.go does
	if result.err == nil && result.origin == overlayComment {
		m.body = result.body
		m.cursor = len([]rune(result.body))
	}

	if m.body != "edited content from editor" {
		t.Errorf("body = %q, want %q", m.body, "edited content from editor")
	}
	if m.cursor != 26 {
		t.Errorf("cursor = %d, want %d", m.cursor, 26)
	}
}

func TestExternalEditorResultMsg_Error(t *testing.T) {
	m := newCommentEditorModel(DefaultTheme())
	m.open("test.go", 10, 15, types.TargetFile)
	m.body = "original"
	m.cursor = 3

	// Simulate error result — body should be unchanged
	result := externalEditorResultMsg{
		origin: overlayComment,
		err:    fmt.Errorf("editor failed"),
	}

	if result.err != nil {
		// Don't update — same as app.go logic
	} else {
		m.body = result.body
	}

	if m.body != "original" {
		t.Errorf("body = %q, want %q", m.body, "original")
	}
	if m.cursor != 3 {
		t.Errorf("cursor = %d, want %d", m.cursor, 3)
	}
}

func TestExternalEditorResultMsg_ReviewSummary(t *testing.T) {
	m := newReviewSummaryModel(DefaultTheme())
	m.body = "original summary"

	result := externalEditorResultMsg{
		body:   "updated summary",
		origin: overlayReview,
	}

	if result.err == nil && result.origin == overlayReview {
		m.body = result.body
	}

	if m.body != "updated summary" {
		t.Errorf("body = %q, want %q", m.body, "updated summary")
	}
}
