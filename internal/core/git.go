package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// GitAPI defines the interface for git operations. Implemented by GitClient
// for production use and stubbed in tests.
type GitAPI interface {
	RepoRoot() string
	CurrentRef() (string, error)
	Diff(baseRef string) ([]types.ChangedFile, error)
	FileDiff(baseRef, path string, contextLines int) (*types.DiffResult, error)
	FileContent(ref, path string) (string, error)
	RecentCommits(n int) ([]LogEntry, error)
	ResolveRef(ref string) (string, error)
	HashObject(path string) (string, error)                   // writes blob to object store
	HashObjectDry(path string) (string, error)                // computes SHA without writing
	HashObjectsDry(paths []string) (map[string]string, error) // batched HashObjectDry
	CatFile(sha string) (string, error)
}

// GitClient wraps git operations for a repository.
type GitClient struct {
	repoRoot string
}

// NewGitClient creates a GitClient for the given repo root.
func NewGitClient(repoRoot string) *GitClient {
	return &GitClient{repoRoot: repoRoot}
}

// RepoRoot returns the repository root path.
func (g *GitClient) RepoRoot() string {
	return g.repoRoot
}

// Diff returns the list of changed files between baseRef and the working tree.
// It includes both tracked changes (from git diff) and untracked files.
func (g *GitClient) Diff(baseRef string) ([]types.ChangedFile, error) {
	out, err := g.run("diff", "--name-status", baseRef)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status: %w", err)
	}

	seen := make(map[string]bool)
	var files []types.ChangedFile
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}

		status := parseFileStatus(parts[0])
		path := parts[1]
		// Handle renames: R100\told\tnew
		if strings.HasPrefix(parts[0], "R") && len(parts) > 1 {
			tabParts := strings.SplitN(line, "\t", 3)
			if len(tabParts) == 3 {
				path = tabParts[2]
			}
		}

		seen[path] = true
		files = append(files, types.ChangedFile{
			Path:   path,
			Status: status,
		})
	}

	// Also include untracked files (new files not yet staged).
	untrackedOut, err := g.run("ls-files", "-o", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files untracked: %w", err)
	}
	for _, path := range strings.Split(strings.TrimSpace(untrackedOut), "\n") {
		if path == "" || seen[path] {
			continue
		}
		files = append(files, types.ChangedFile{
			Path:   path,
			Status: types.FileAdded,
		})
	}

	// Fill in per-file churn from numstat (binary files report "-" and stay 0).
	churn := g.numstat(baseRef)
	for i := range files {
		if c, ok := churn[files[i].Path]; ok {
			files[i].Additions = c.add
			files[i].Deletions = c.del
		}
	}

	return files, nil
}

type fileChurn struct{ add, del int }

// numstat returns added/deleted line counts per path from `git diff --numstat`.
// Binary files (reported as "-\t-") and any parse failures are omitted, leaving
// the file's churn at zero.
func (g *GitClient) numstat(baseRef string) map[string]fileChurn {
	out, err := g.run("diff", "--numstat", baseRef)
	if err != nil {
		return nil
	}
	result := make(map[string]fileChurn)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		add, errA := strconv.Atoi(parts[0])
		del, errD := strconv.Atoi(parts[1])
		if errA != nil || errD != nil {
			continue // binary file ("-")
		}
		path := parts[2]
		// Renames render as "old => new"; numstat may use the arrow form. Prefer
		// the post-arrow path so it matches the name-status entry.
		if idx := strings.Index(path, " => "); idx >= 0 {
			path = path[idx+len(" => "):]
			path = strings.TrimSuffix(path, "}")
		}
		result[path] = fileChurn{add: add, del: del}
	}
	return result
}

// fullFileContextLines is the -U value used for full-file diffs: large enough to
// envelope any real file (well under INT_MAX) so the whole file renders as a
// single hunk with diff coloring. git clamps it to the actual file size.
const fullFileContextLines = 1 << 30

// FileDiff returns the parsed diff for a single file.
// contextLines controls the number of unchanged lines around each hunk (-U flag).
// A value of 0 uses git's default (3); a negative value shows the full file.
// For untracked files (where git diff returns nothing), a synthetic diff is
// generated showing the entire file as added.
func (g *GitClient) FileDiff(baseRef, path string, contextLines int) (*types.DiffResult, error) {
	args := []string{"diff"}
	switch {
	case contextLines < 0:
		args = append(args, fmt.Sprintf("-U%d", fullFileContextLines))
	case contextLines > 0:
		args = append(args, fmt.Sprintf("-U%d", contextLines))
	}
	args = append(args, baseRef, "--", path)
	out, err := g.run(args...)
	if err != nil {
		// diff returns exit 1 when there are differences, which is expected
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// Use the output even though exit code was 1
		} else {
			return nil, fmt.Errorf("git diff %s: %w", path, err)
		}
	}

	hunks := parseDiff(out)

	// If git diff returned nothing (e.g. untracked file), build a synthetic
	// all-added diff from the working-tree content.
	if len(hunks) == 0 {
		content, readErr := g.FileContent("", path)
		if readErr == nil && content != "" {
			hunks = buildSyntheticDiff(content)
		}
	}

	return &types.DiffResult{
		Path:  path,
		Hunks: hunks,
	}, nil
}

// FileContent returns file content at a given ref, or from the working tree if ref is empty.
func (g *GitClient) FileContent(ref, path string) (string, error) {
	if ref == "" || ref == "WORKING" {
		absPath := filepath.Join(g.repoRoot, path)
		out, err := exec.Command("cat", absPath).Output()
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", path, err)
		}
		return string(out), nil
	}
	out, err := g.run("show", ref+":"+path)
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %w", ref, path, err)
	}
	return out, nil
}

// CurrentRef returns the current HEAD commit hash.
func (g *GitClient) CurrentRef() (string, error) {
	out, err := g.run("rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// LogEntry represents a single commit in the log.
type LogEntry struct {
	Hash    string
	Subject string
}

// RecentCommits returns the last n commits as short hash + subject.
func (g *GitClient) RecentCommits(n int) ([]LogEntry, error) {
	out, err := g.run("log", "--oneline", fmt.Sprintf("-n%d", n), "--format=%h %s")
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var entries []LogEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		entry := LogEntry{Hash: parts[0]}
		if len(parts) > 1 {
			entry.Subject = parts[1]
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ResolveRef resolves a ref string (e.g. "abc123~1") to a full commit hash.
func (g *GitClient) ResolveRef(ref string) (string, error) {
	out, err := g.run("rev-parse", ref)
	if err != nil {
		return "", fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	return strings.TrimSpace(out), nil
}

// HashObject writes a file's content into the git object store and returns its blob SHA.
// This works with uncommitted files — it hashes the working tree content.
func (g *GitClient) HashObject(path string) (string, error) {
	absPath := filepath.Join(g.repoRoot, path)
	out, err := g.run("hash-object", "-w", absPath)
	if err != nil {
		return "", fmt.Errorf("git hash-object %s: %w", path, err)
	}
	return strings.TrimSpace(out), nil
}

// HashObjectDry computes a file's blob SHA without writing to the object store.
// Used for comparison-only operations (e.g. checking if a file changed since a snapshot).
func (g *GitClient) HashObjectDry(path string) (string, error) {
	absPath := filepath.Join(g.repoRoot, path)
	out, err := g.run("hash-object", absPath)
	if err != nil {
		return "", fmt.Errorf("git hash-object %s: %w", path, err)
	}
	return strings.TrimSpace(out), nil
}

// HashObjectsDry computes blob SHAs for multiple paths in a single git invocation
// via `git hash-object --stdin-paths`. Returns a map from input path to SHA.
// Paths that git refuses to hash (missing, unreadable) are omitted from the result
// without failing the batch: a single missing file on a 2s refresh tick should not
// blank out auto-unmark for every other file.
func (g *GitClient) HashObjectsDry(paths []string) (map[string]string, error) {
	result := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return result, nil
	}

	var stdin strings.Builder
	for _, p := range paths {
		stdin.WriteString(filepath.Join(g.repoRoot, p))
		stdin.WriteByte('\n')
	}

	cmd := exec.Command("git", "hash-object", "--stdin-paths")
	cmd.Dir = g.repoRoot
	cmd.Stdin = strings.NewReader(stdin.String())
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "GIT_DIR=") && !strings.HasPrefix(e, "GIT_WORK_TREE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered
	out, err := cmd.Output()
	if err != nil {
		// Fall back to per-path hashing so one unreadable file can't kill the batch.
		for _, p := range paths {
			if sha, ferr := g.HashObjectDry(p); ferr == nil {
				result[p] = sha
			}
		}
		return result, nil
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i, sha := range lines {
		if i >= len(paths) {
			break
		}
		sha = strings.TrimSpace(sha)
		if sha != "" {
			result[paths[i]] = sha
		}
	}
	return result, nil
}

// CatFile retrieves content from the git object store by SHA.
func (g *GitClient) CatFile(sha string) (string, error) {
	out, err := g.run("cat-file", "-p", sha)
	if err != nil {
		return "", fmt.Errorf("git cat-file %s: %w", sha, err)
	}
	return out, nil
}

func (g *GitClient) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoRoot
	// Clear worktree env vars so git uses cmd.Dir as the repo root.
	// Without this, git may use GIT_DIR/GIT_WORK_TREE from the parent
	// process (e.g. when running inside a git worktree).
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "GIT_DIR=") && !strings.HasPrefix(e, "GIT_WORK_TREE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered
	out, err := cmd.Output()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// buildSyntheticDiff creates a single hunk showing the entire content as added lines.
// Used for untracked files that have no base version to diff against.
func buildSyntheticDiff(content string) []types.DiffHunk {
	return buildSyntheticHunks(content, types.DiffLineAdded)
}

// buildSyntheticDeleteDiff creates a single hunk showing the entire content as removed lines.
// Used for files that existed in a snapshot but have been deleted.
func buildSyntheticDeleteDiff(content string) []types.DiffHunk {
	return buildSyntheticHunks(content, types.DiffLineRemoved)
}

// buildSyntheticHunks creates a single hunk showing entire content as either all-added or all-removed.
func buildSyntheticHunks(content string, kind types.DiffLineKind) []types.DiffHunk {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	hunk := types.DiffHunk{}
	if kind == types.DiffLineAdded {
		hunk.NewStart = 1
		hunk.NewCount = len(lines)
	} else {
		hunk.OldStart = 1
		hunk.OldCount = len(lines)
	}

	for i, line := range lines {
		dl := types.DiffLine{Kind: kind, Content: line}
		if kind == types.DiffLineAdded {
			dl.NewLineNum = i + 1
		} else {
			dl.OldLineNum = i + 1
		}
		hunk.Lines = append(hunk.Lines, dl)
	}
	return []types.DiffHunk{hunk}
}

func parseFileStatus(s string) types.FileChangeStatus {
	switch {
	case s == "A":
		return types.FileAdded
	case s == "D":
		return types.FileDeleted
	case s == "M":
		return types.FileModified
	case strings.HasPrefix(s, "R"):
		return types.FileRenamed
	default:
		return types.FileModified
	}
}

// parseDiff parses unified diff output into structured hunks.
func parseDiff(raw string) []types.DiffHunk {
	var hunks []types.DiffHunk
	lines := strings.Split(raw, "\n")

	var current *types.DiffHunk
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if current != nil {
				hunks = append(hunks, *current)
			}
			current = parseHunkHeader(line)
			continue
		}
		if current == nil {
			continue
		}

		dl := types.DiffLine{Content: line}
		switch {
		case strings.HasPrefix(line, "+"):
			dl.Kind = types.DiffLineAdded
			dl.NewLineNum = current.NewStart + countLines(current.Lines, types.DiffLineAdded, types.DiffLineContext)
			dl.Content = line[1:]
		case strings.HasPrefix(line, "-"):
			dl.Kind = types.DiffLineRemoved
			dl.OldLineNum = current.OldStart + countLines(current.Lines, types.DiffLineRemoved, types.DiffLineContext)
			dl.Content = line[1:]
		default:
			if len(line) > 0 && line[0] == ' ' {
				dl.Content = line[1:]
			}
			dl.Kind = types.DiffLineContext
			dl.OldLineNum = current.OldStart + countLines(current.Lines, types.DiffLineRemoved, types.DiffLineContext)
			dl.NewLineNum = current.NewStart + countLines(current.Lines, types.DiffLineAdded, types.DiffLineContext)
		}
		current.Lines = append(current.Lines, dl)
	}
	if current != nil {
		hunks = append(hunks, *current)
	}

	return hunks
}

func parseHunkHeader(line string) *types.DiffHunk {
	// Format: @@ -old_start,old_count +new_start,new_count @@ optional header
	h := &types.DiffHunk{}

	// Find the ranges between @@ markers
	parts := strings.SplitN(line, "@@", 3)
	if len(parts) < 2 {
		return h
	}
	if len(parts) == 3 {
		h.Header = strings.TrimSpace(parts[2])
	}

	ranges := strings.TrimSpace(parts[1])
	rangeParts := strings.Fields(ranges)

	for _, rp := range rangeParts {
		if strings.HasPrefix(rp, "-") {
			nums := strings.SplitN(rp[1:], ",", 2)
			h.OldStart, _ = strconv.Atoi(nums[0])
			if len(nums) > 1 {
				h.OldCount, _ = strconv.Atoi(nums[1])
			} else {
				h.OldCount = 1
			}
		} else if strings.HasPrefix(rp, "+") {
			nums := strings.SplitN(rp[1:], ",", 2)
			h.NewStart, _ = strconv.Atoi(nums[0])
			if len(nums) > 1 {
				h.NewCount, _ = strconv.Atoi(nums[1])
			} else {
				h.NewCount = 1
			}
		}
	}

	return h
}

func countLines(lines []types.DiffLine, kinds ...types.DiffLineKind) int {
	n := 0
	for _, l := range lines {
		for _, k := range kinds {
			if l.Kind == k {
				n++
				break
			}
		}
	}
	return n
}
