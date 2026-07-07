package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/josephschmitt/monocle/internal/db"
)

// copyMediaToStorage copies a media source file into Monocle's managed media
// directory (per session, per content id) so it remains viewable even if the
// agent later removes the original. It returns the absolute stored path.
func copyMediaToStorage(sessionID, id, source string) (string, error) {
	src, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", source)
	}

	destDir := filepath.Join(db.MediaDir(), sanitizePathSegment(sessionID), sanitizePathSegment(id))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	dest := filepath.Join(destDir, filepath.Base(source))
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}
	return dest, nil
}

// sanitizePathSegment reduces a string to a safe single path segment: path
// separators and dot-runs become dashes so ids/session-ids can't escape the
// media directory.
func sanitizePathSegment(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, "..", "-")
	s = strings.Trim(s, ". ")
	if s == "" {
		return "item"
	}
	return s
}
