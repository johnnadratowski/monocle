package imports

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// goResolver maps Go package imports to changed files. A changed .go file in
// package dir D that imports module path M/sub creates an edge to every changed
// .go file under repoRoot/sub. The module path comes from go.mod.
type goResolver struct{}

func (goResolver) edges(repoRoot string, files []string, inSet func(string) bool) []Edge {
	modPath := modulePath(repoRoot)
	if modPath == "" {
		return nil
	}

	// Index changed files by their directory (repo-relative, slash form).
	filesByDir := map[string][]string{}
	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f))
		filesByDir[dir] = append(filesByDir[dir], filepath.ToSlash(f))
	}

	var edges []Edge
	fset := token.NewFileSet()
	for _, f := range files {
		abs := filepath.Join(repoRoot, f)
		src, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		af, err := parser.ParseFile(fset, abs, src, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range af.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(path, modPath) {
				continue // external/stdlib import
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(path, modPath), "/")
			for _, target := range filesByDir[rel] {
				if target != filepath.ToSlash(f) {
					edges = append(edges, Edge{From: filepath.ToSlash(f), To: target})
				}
			}
		}
	}
	return edges
}

// modulePath reads the module path from go.mod at repoRoot, or "" if absent.
func modulePath(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
