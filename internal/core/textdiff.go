package core

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/josephschmitt/monocle/internal/types"
)

// TextDiff computes a unified diff between two text strings and returns parsed hunks.
// It shells out to the system `diff` command and feeds the output through parseDiff.
func TextDiff(oldContent, newContent string) ([]types.DiffHunk, error) {
	return TextDiffContext(oldContent, newContent, 0)
}

// TextDiffContext is like TextDiff but controls the unchanged-context lines
// around each hunk: 0 uses diff's default (3), a negative value shows the full
// file. Used for the full-file diff modifier in review-base (snapshot) mode.
func TextDiffContext(oldContent, newContent string, contextLines int) ([]types.DiffHunk, error) {
	if oldContent == newContent {
		return nil, nil
	}

	oldFile, err := os.CreateTemp("", "monocle-diff-old-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file for old content: %w", err)
	}
	defer os.Remove(oldFile.Name())

	newFile, err := os.CreateTemp("", "monocle-diff-new-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file for new content: %w", err)
	}
	defer os.Remove(newFile.Name())

	if _, err := oldFile.WriteString(oldContent); err != nil {
		return nil, fmt.Errorf("write old content: %w", err)
	}
	oldFile.Close()

	if _, err := newFile.WriteString(newContent); err != nil {
		return nil, fmt.Errorf("write new content: %w", err)
	}
	newFile.Close()

	ctxArg := "-U3"
	if contextLines < 0 {
		ctxArg = fmt.Sprintf("-U%d", fullFileContextLines)
	} else if contextLines > 0 {
		ctxArg = fmt.Sprintf("-U%d", contextLines)
	}
	out, err := exec.Command("diff", ctxArg, oldFile.Name(), newFile.Name()).Output()
	if err != nil {
		// diff exits with code 1 when files differ — that's expected
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Use the output
		} else {
			return nil, fmt.Errorf("diff command: %w", err)
		}
	}

	hunks := parseDiff(string(out))
	return hunks, nil
}
