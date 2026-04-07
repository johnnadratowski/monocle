package adapters

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// FindRepoRoot walks up from startDir looking for a .git entry (dir or file,
// to handle worktrees/submodules). Returns the directory containing .git, or
// the absolute startDir if none found.
func FindRepoRoot(startDir string) string {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return startDir
	}

	dir := abs
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding .git
			return abs
		}
		dir = parent
	}
}

// IsGitRepo returns true if dir is inside a Git repository.
func IsGitRepo(dir string) bool {
	root := FindRepoRoot(dir)
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

// ResolveSocketPath returns the engine socket path, checking MONOCLE_SOCKET
// first, then deriving from the current working directory's repo root.
func ResolveSocketPath() string {
	if p := os.Getenv("MONOCLE_SOCKET"); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return DefaultSocketPath(FindRepoRoot(cwd))
}

// DefaultSocketPath computes a deterministic socket path from a directory.
// Returns /tmp/monocle-<sha256_first12>.sock (30 chars, well within the
// ~104-byte macOS socket path limit).
func DefaultSocketPath(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	sum := sha256.Sum256([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:12]
	return "/tmp/monocle-" + hash + ".sock"
}
