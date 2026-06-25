// Package imports computes a reading order for a set of changed files from their
// intra-changeset import graph: a file imported by others sorts before its
// dependents (dependencies first, dependents last). It is deterministic and
// sender-independent — derived from the changed set Monocle already has.
//
// Resolution is per-language behind the resolver interface: esbuild for TS/JS
// (accurate extraction of static/dynamic/re-export imports), go/parser for Go,
// and a lightweight regex for Solidity. Only edges where both endpoints are in
// the changed set are kept.
package imports

import (
	"path/filepath"
	"sort"
	"strings"
)

// Edge means file From imports file To. Both are repo-relative paths within the
// changed set.
type Edge struct {
	From string
	To   string
}

// resolver extracts intra-set import edges for the files it handles.
type resolver interface {
	// edges returns import edges among files. repoRoot is absolute; files and the
	// returned edge endpoints are repo-relative. inSet reports whether a
	// repo-relative path is part of the changed set.
	edges(repoRoot string, files []string, inSet func(string) bool) []Edge
}

// extResolvers maps a lowercase file extension to the resolver for it.
var extResolvers = map[string]resolver{
	".ts":  esbuildResolver{},
	".tsx": esbuildResolver{},
	".js":  esbuildResolver{},
	".jsx": esbuildResolver{},
	".mjs": esbuildResolver{},
	".cjs": esbuildResolver{},
	".go":  goResolver{},
	".sol": regexResolver{},
}

// Order returns a map from each repo-relative file path to an import-order rank:
// the depth of the file in the intra-changeset dependency graph. A file that
// imports nothing in the set has rank 0; a file importing a rank-0 file has rank
// 1, and so on — so lower ranks (foundational files) sort first. Files with the
// same rank are left for the caller to break (e.g. by churn). Cyclic edges are
// broken arbitrarily but deterministically.
//
// repoRoot must be absolute. files are repo-relative. Files whose language has no
// resolver simply get rank 0.
func Order(repoRoot string, files []string) map[string]int {
	set := make(map[string]bool, len(files))
	for _, f := range files {
		set[filepath.ToSlash(f)] = true
	}
	inSet := func(p string) bool { return set[filepath.ToSlash(p)] }

	// Group files by resolver and collect edges.
	byResolver := map[resolver][]string{}
	for _, f := range files {
		if r, ok := extResolvers[strings.ToLower(filepath.Ext(f))]; ok {
			byResolver[r] = append(byResolver[r], f)
		}
	}
	var edges []Edge
	for r, fs := range byResolver {
		edges = append(edges, r.edges(repoRoot, fs, inSet)...)
	}

	return rankByDepth(files, edges)
}

// rankByDepth assigns each file the longest dependency-chain depth below it.
func rankByDepth(files []string, edges []Edge) map[string]int {
	// deps[f] = set of files f imports (in-set).
	deps := make(map[string][]string, len(files))
	for _, f := range files {
		deps[filepath.ToSlash(f)] = nil
	}
	seen := map[Edge]bool{}
	for _, e := range edges {
		from, to := filepath.ToSlash(e.From), filepath.ToSlash(e.To)
		if from == to || seen[Edge{from, to}] {
			continue
		}
		if _, ok := deps[from]; !ok {
			continue
		}
		if _, ok := deps[to]; !ok {
			continue
		}
		seen[Edge{from, to}] = true
		deps[from] = append(deps[from], to)
	}

	rank := make(map[string]int, len(files))
	const (
		visiting = -1
	)
	state := map[string]int{} // 0 unset, visiting, 1 done
	var depth func(f string) int
	depth = func(f string) int {
		if state[f] == visiting {
			return 0 // cycle: break by treating this back-edge as no contribution
		}
		if state[f] == 1 {
			return rank[f]
		}
		state[f] = visiting
		max := 0
		// Deterministic traversal order.
		ds := append([]string(nil), deps[f]...)
		sort.Strings(ds)
		for _, d := range ds {
			if c := depth(d) + 1; c > max {
				max = c
			}
		}
		state[f] = 1
		rank[f] = max
		return max
	}
	for _, f := range files {
		depth(filepath.ToSlash(f))
	}
	return rank
}
