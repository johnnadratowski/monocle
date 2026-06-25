package imports

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// regexResolver handles languages without a dedicated parser (currently
// Solidity) by extracting relative import paths from import statements. It
// resolves only relative specifiers — enough for intra-changeset edges, which
// are almost always relative.
type regexResolver struct{}

// importStringPattern matches the quoted path in common import forms:
//
//	import "./Foo.sol";
//	import {X} from "../Bar.sol";
//	import * as Y from "./baz";
var importStringPattern = regexp.MustCompile(`(?m)\bimport\b[^;'"]*['"]([^'"]+)['"]`)

var solExts = []string{".sol", ""}

func (regexResolver) edges(repoRoot string, files []string, inSet func(string) bool) []Edge {
	absToRel := make(map[string]string, len(files))
	for _, f := range files {
		absToRel[filepath.ToSlash(filepath.Join(repoRoot, f))] = filepath.ToSlash(f)
	}

	var edges []Edge
	for _, f := range files {
		abs := filepath.Join(repoRoot, f)
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		for _, m := range importStringPattern.FindAllStringSubmatch(string(data), -1) {
			spec := m[1]
			if !strings.HasPrefix(spec, ".") {
				continue // only relative imports
			}
			target := resolveRelative(filepath.Dir(abs), spec, solExts)
			if toRel, ok := absToRel[filepath.ToSlash(target)]; ok && toRel != filepath.ToSlash(f) {
				edges = append(edges, Edge{From: filepath.ToSlash(f), To: toRel})
			}
		}
	}
	return edges
}

// resolveRelative joins a relative specifier and tries the given extensions.
func resolveRelative(baseDir, spec string, exts []string) string {
	p := filepath.Join(baseDir, spec)
	for _, ext := range exts {
		cand := p + ext
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	return ""
}
