package imports

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

// esbuildResolver extracts TS/JS import edges. esbuild parses each changed file
// and reports every import specifier (static, dynamic, re-export, require) via
// OnResolve; we resolve those specifiers ourselves (relative paths + tsconfig
// path aliases) and keep edges that land on another changed file. Everything is
// marked external so esbuild never recurses into node_modules or the wider tree.
type esbuildResolver struct{}

var tsExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".json"}

func (esbuildResolver) edges(repoRoot string, files []string, inSet func(string) bool) []Edge {
	if len(files) == 0 {
		return nil
	}
	// Canonicalize so paths match esbuild's (e.g. macOS /var -> /private/var).
	if r, err := filepath.EvalSymlinks(repoRoot); err == nil {
		repoRoot = r
	}
	aliases := loadTSConfigPaths(repoRoot)

	absToRel := make(map[string]string, len(files))
	entries := make([]string, 0, len(files))
	for _, f := range files {
		abs := filepath.Join(repoRoot, f)
		absToRel[filepath.ToSlash(abs)] = filepath.ToSlash(f)
		entries = append(entries, abs)
	}

	var (
		mu    sync.Mutex
		edges []Edge
	)
	plugin := api.Plugin{
		Name: "monocle-import-edges",
		Setup: func(b api.PluginBuild) {
			b.OnResolve(api.OnResolveOptions{Filter: `.*`}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				// Let esbuild resolve and load the entry points themselves so it
				// parses them — only intercept the imports they contain.
				if args.Kind == api.ResolveEntryPoint {
					return api.OnResolveResult{}, nil
				}
				// args.Importer is the absolute path of the file doing the import;
				// args.Path is the raw specifier. esbuild parses entry points
				// concurrently, so guard the shared edge slice.
				fromRel, ok := absToRel[filepath.ToSlash(args.Importer)]
				if ok {
					if target := resolveTSSpecifier(repoRoot, args.Importer, args.Path, aliases); target != "" {
						if toRel, ok := absToRel[filepath.ToSlash(target)]; ok && toRel != fromRel {
							mu.Lock()
							edges = append(edges, Edge{From: fromRel, To: toRel})
							mu.Unlock()
						}
					}
				}
				// Never follow the import — we only want each entry's direct edges.
				return api.OnResolveResult{Path: args.Path, External: true}, nil
			})
		},
	}

	api.Build(api.BuildOptions{
		EntryPoints: entries,
		Bundle:      true,
		Write:       false,
		Outdir:      repoRoot, // required for multiple entry points; nothing is written
		LogLevel:    api.LogLevelSilent,
		Plugins:     []api.Plugin{plugin},
	})
	return edges
}

// resolveTSSpecifier resolves an import specifier to an absolute file path,
// handling relative imports and tsconfig path aliases. Returns "" when it can't
// be resolved to an existing file (bare module imports, unknown aliases).
func resolveTSSpecifier(repoRoot, importerAbs, spec string, aliases map[string][]string) string {
	if strings.HasPrefix(spec, ".") {
		base := filepath.Dir(importerAbs)
		return resolveWithExts(filepath.Join(base, spec))
	}
	if strings.HasPrefix(spec, "/") {
		return resolveWithExts(spec)
	}
	// tsconfig path aliases, e.g. "@/foo" -> ["src/foo"].
	for pattern, targets := range aliases {
		if matched, suffix := matchAlias(pattern, spec); matched {
			for _, t := range targets {
				candidate := filepath.Join(repoRoot, strings.Replace(t, "*", suffix, 1))
				if r := resolveWithExts(candidate); r != "" {
					return r
				}
			}
		}
	}
	return ""
}

// matchAlias matches a tsconfig path pattern ("@app/*") against a specifier and
// returns the captured suffix for the "*" wildcard.
func matchAlias(pattern, spec string) (bool, string) {
	if star := strings.IndexByte(pattern, '*'); star >= 0 {
		prefix := pattern[:star]
		if strings.HasPrefix(spec, prefix) {
			return true, spec[len(prefix):]
		}
		return false, ""
	}
	return pattern == spec, ""
}

// resolveWithExts tries the path as-is, with each known extension, and as a
// directory index file. Returns the first existing file (absolute) or "".
func resolveWithExts(p string) string {
	if isFile(p) {
		return p
	}
	for _, ext := range tsExts {
		if isFile(p + ext) {
			return p + ext
		}
	}
	for _, ext := range tsExts {
		if idx := filepath.Join(p, "index"+ext); isFile(idx) {
			return idx
		}
	}
	return ""
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// loadTSConfigPaths reads compilerOptions.paths/baseUrl from tsconfig.json at the
// repo root, returning alias-pattern -> target-pattern(s) with targets made
// relative to repoRoot. Best-effort: returns nil when absent or unparseable.
func loadTSConfigPaths(repoRoot string) map[string][]string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "tsconfig.json"))
	if err != nil {
		return nil
	}
	var cfg struct {
		CompilerOptions struct {
			BaseURL string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	base := cfg.CompilerOptions.BaseURL
	if base == "" {
		base = "."
	}
	out := make(map[string][]string, len(cfg.CompilerOptions.Paths))
	for pattern, targets := range cfg.CompilerOptions.Paths {
		rebased := make([]string, len(targets))
		for i, t := range targets {
			rebased[i] = filepath.ToSlash(filepath.Join(base, t))
		}
		out[pattern] = rebased
	}
	return out
}
