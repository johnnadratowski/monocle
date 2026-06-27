package core

import (
	"fmt"

	"github.com/josephschmitt/monocle/internal/types"
)

// gitStub implements GitAPI for unit tests without requiring a real git repo.
type gitStub struct {
	repoRoot   string
	currentRef string
	files      []types.ChangedFile
	diffResult *types.DiffResult
	commits    []LogEntry

	resolveRefs map[string]string // ref -> resolved sha for ResolveRef (overrides currentRef)

	// Per-path overrides for testing snapshot diffing and auto-unmark
	fileContents    map[string]string // path -> content for FileContent("", path)
	hashObjects     map[string]string // path -> sha for HashObject
	hashObjectDrys  map[string]string // path -> sha for HashObjectDry
	catFileContents map[string]string // sha -> content for CatFile
}

func (g *gitStub) RepoRoot() string { return g.repoRoot }

func (g *gitStub) CurrentRef() (string, error) {
	return g.currentRef, nil
}

func (g *gitStub) Diff(_ string) ([]types.ChangedFile, error) {
	return g.files, nil
}

func (g *gitStub) FileDiff(_, _ string, _ int) (*types.DiffResult, error) {
	if g.diffResult != nil {
		return g.diffResult, nil
	}
	return &types.DiffResult{}, nil
}

func (g *gitStub) FileContent(_, path string) (string, error) {
	if g.fileContents != nil {
		if c, ok := g.fileContents[path]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("file not found: %s", path)
}

func (g *gitStub) RecentCommits(_ int) ([]LogEntry, error) {
	return g.commits, nil
}

func (g *gitStub) ResolveRef(ref string) (string, error) {
	if g.resolveRefs != nil {
		if sha, ok := g.resolveRefs[ref]; ok {
			return sha, nil
		}
	}
	return g.currentRef, nil
}

func (g *gitStub) HashObject(path string) (string, error) {
	if g.hashObjects != nil {
		if sha, ok := g.hashObjects[path]; ok {
			return sha, nil
		}
	}
	return "deadbeef1234567890abcdef1234567890abcdef", nil
}

func (g *gitStub) HashObjectDry(path string) (string, error) {
	if g.hashObjectDrys != nil {
		if sha, ok := g.hashObjectDrys[path]; ok {
			return sha, nil
		}
	}
	return "deadbeef1234567890abcdef1234567890abcdef", nil
}

func (g *gitStub) HashObjectsDry(paths []string) (map[string]string, error) {
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		sha, _ := g.HashObjectDry(p)
		out[p] = sha
	}
	return out, nil
}

func (g *gitStub) CatFile(sha string) (string, error) {
	if g.catFileContents != nil {
		if c, ok := g.catFileContents[sha]; ok {
			return c, nil
		}
	}
	return "", fmt.Errorf("blob not found: %s", sha)
}
