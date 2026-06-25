package imports

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRankByDepth(t *testing.T) {
	// a imports b; c imports a. Expect b=0, a=1, c=2.
	files := []string{"a.ts", "b.ts", "c.ts"}
	edges := []Edge{{"a.ts", "b.ts"}, {"c.ts", "a.ts"}}
	rank := rankByDepth(files, edges)
	if rank["b.ts"] != 0 || rank["a.ts"] != 1 || rank["c.ts"] != 2 {
		t.Errorf("ranks = %v, want b=0 a=1 c=2", rank)
	}
}

func TestRankByDepthCycle(t *testing.T) {
	// a <-> b cycle must not hang and must produce finite ranks.
	files := []string{"a.ts", "b.ts"}
	edges := []Edge{{"a.ts", "b.ts"}, {"b.ts", "a.ts"}}
	rank := rankByDepth(files, edges)
	if len(rank) != 2 {
		t.Fatalf("expected 2 ranks, got %v", rank)
	}
}

func TestOrderTypeScript(t *testing.T) {
	dir := t.TempDir()
	// b imports nothing; a imports b; c imports a (and an external lib).
	write(t, dir, "b.ts", `export const b = 1;`)
	write(t, dir, "a.ts", `import { b } from "./b"; export const a = b + 1;`)
	write(t, dir, "c.ts", `import { a } from "./a"; import x from "lodash"; export const c = a;`)

	rank := Order(dir, []string{"a.ts", "b.ts", "c.ts"})
	if rank["b.ts"] != 0 {
		t.Errorf("b.ts rank = %d, want 0 (imports nothing)", rank["b.ts"])
	}
	if !(rank["a.ts"] > rank["b.ts"]) {
		t.Errorf("a.ts (%d) should rank after b.ts (%d)", rank["a.ts"], rank["b.ts"])
	}
	if !(rank["c.ts"] > rank["a.ts"]) {
		t.Errorf("c.ts (%d) should rank after a.ts (%d)", rank["c.ts"], rank["a.ts"])
	}
}

func TestOrderTypeScriptReExport(t *testing.T) {
	dir := t.TempDir()
	// Re-export is an edge regex would miss; esbuild catches it.
	write(t, dir, "base.ts", `export const base = 1;`)
	write(t, dir, "index.ts", `export * from "./base";`)
	rank := Order(dir, []string{"base.ts", "index.ts"})
	if !(rank["index.ts"] > rank["base.ts"]) {
		t.Errorf("index.ts (%d) should rank after base.ts (%d) via re-export", rank["index.ts"], rank["base.ts"])
	}
}

func TestOrderGo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/proj\n\ngo 1.25\n")
	write(t, dir, "core/core.go", "package core\n\nfunc C() {}\n")
	write(t, dir, "api/api.go", "package api\n\nimport \"example.com/proj/core\"\n\nfunc A() { core.C() }\n")

	rank := Order(dir, []string{"api/api.go", "core/core.go"})
	if !(rank["api/api.go"] > rank["core/core.go"]) {
		t.Errorf("api (%d) should rank after core (%d) — api imports core", rank["api/api.go"], rank["core/core.go"])
	}
}

func TestOrderSolidity(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "Token.sol", "// SPDX\npragma solidity ^0.8.0;\ncontract Token {}\n")
	write(t, dir, "Sale.sol", "// SPDX\npragma solidity ^0.8.0;\nimport \"./Token.sol\";\ncontract Sale {}\n")

	rank := Order(dir, []string{"Sale.sol", "Token.sol"})
	if !(rank["Sale.sol"] > rank["Token.sol"]) {
		t.Errorf("Sale (%d) should rank after Token (%d) — Sale imports Token", rank["Sale.sol"], rank["Token.sol"])
	}
}

func TestOrderTSConfigAlias(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "tsconfig.json", `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`)
	write(t, dir, "src/util.ts", `export const u = 1;`)
	write(t, dir, "src/app.ts", `import { u } from "@/util"; export const app = u;`)
	rank := Order(dir, []string{"src/app.ts", "src/util.ts"})
	if !(rank["src/app.ts"] > rank["src/util.ts"]) {
		t.Errorf("app (%d) should rank after util (%d) via @/ alias", rank["src/app.ts"], rank["src/util.ts"])
	}
}
