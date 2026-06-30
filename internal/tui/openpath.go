package tui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// findFilePathInLine scans a line of text for a file path that resolves to an
// existing regular file and returns its absolute path plus a 1-based line number
// parsed from a trailing :N (or :N:M) suffix. Candidates are tried as an absolute
// path, then relative to baseDir (the directory of the file being viewed), then
// relative to repoRoot, then relative to the current working directory. Returns
// ok=false when no token on the line resolves to a file.
//
// The diff cursor is line-based, so this matches the first resolvable path on the
// line (e.g. an import, a quoted path, or a stack-trace frame like foo.go:42).
func findFilePathInLine(text, repoRoot, baseDir string) (path string, line int, ok bool) {
	for _, tok := range pathCandidates(text) {
		cand, ln := splitLineSuffix(tok)
		if len(cand) < 2 {
			continue
		}
		if resolved, found := resolveExistingFile(cand, repoRoot, baseDir); found {
			return resolved, ln, true
		}
	}
	return "", 0, false
}

// pathCandidates splits a line into path-like tokens, stripping the quotes,
// brackets, and surrounding punctuation that commonly wrap a path in source.
func pathCandidates(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case ' ', '\t', '"', '\'', '`', '(', ')', '[', ']', '{', '}', '<', '>',
			',', ';', '=', '|', '*':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		// Trim a trailing colon (compiler "file:line:col:" frames) and sentence
		// punctuation, but keep interior ':' for the line suffix.
		f = strings.TrimRight(f, ".:")
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// splitLineSuffix separates a trailing :line or :line:col from a path token,
// returning the path and the 1-based line number (0 when absent). It only treats
// trailing all-numeric ':'-segments as a location, so paths containing colons
// (and Windows-style drive prefixes) are left intact.
func splitLineSuffix(tok string) (string, int) {
	parts := strings.Split(tok, ":")
	if len(parts) < 2 {
		return tok, 0
	}
	end := len(parts)
	for end > 1 && isAllDigits(parts[end-1]) {
		end--
	}
	if end == len(parts) {
		return tok, 0 // no trailing numeric segment
	}
	base := strings.Join(parts[:end], ":")
	line, _ := strconv.Atoi(parts[end])
	return base, line
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveExistingFile returns the absolute path of the first candidate location
// that exists and is a regular file (not a directory).
func resolveExistingFile(cand, repoRoot, baseDir string) (string, bool) {
	cand = expandHome(cand)

	var tries []string
	if filepath.IsAbs(cand) {
		tries = []string{cand}
	} else {
		if baseDir != "" {
			tries = append(tries, filepath.Join(baseDir, cand))
		}
		if repoRoot != "" {
			tries = append(tries, filepath.Join(repoRoot, cand))
		}
		tries = append(tries, cand)
	}

	for _, p := range tries {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			return abs, true
		}
		return p, true
	}
	return "", false
}

// expandHome replaces a leading ~ (or ~/) with the user's home directory.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
