package tui

import "testing"

func TestThemeByName(t *testing.T) {
	// Every named theme resolves and carries a syntax style.
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		if th.SyntaxStyle == "" {
			t.Errorf("theme %q has empty SyntaxStyle", name)
		}
	}
	// Molokai uses the monokai chroma style.
	if got := ThemeByName("molokai").SyntaxStyle; got != "monokai" {
		t.Errorf("molokai SyntaxStyle = %q, want monokai", got)
	}
	// Unknown/empty names fall back to the dark default (monokai syntax).
	if got := ThemeByName("nope").SyntaxStyle; got != "monokai" {
		t.Errorf("unknown theme SyntaxStyle = %q, want monokai (dark fallback)", got)
	}
	if got := ThemeByName("").SyntaxStyle; got != "monokai" {
		t.Errorf("empty theme SyntaxStyle = %q, want monokai (dark fallback)", got)
	}
}

func TestValidThemeName(t *testing.T) {
	for _, name := range []string{"dark", "light", "molokai", "dracula", "nord"} {
		if !validThemeName(name) {
			t.Errorf("validThemeName(%q) = false, want true", name)
		}
	}
	if validThemeName("bogus") {
		t.Error("validThemeName(\"bogus\") = true, want false")
	}
}

func TestNextThemeName(t *testing.T) {
	names := ThemeNames()
	// Cycling through all names returns to the start.
	cur := names[0]
	for range names {
		cur = NextThemeName(cur)
	}
	if cur != names[0] {
		t.Errorf("cycling all themes returned %q, want %q", cur, names[0])
	}
	// Unknown current name starts the cycle at the first theme.
	if got := NextThemeName("bogus"); got != names[0] {
		t.Errorf("NextThemeName(bogus) = %q, want %q", got, names[0])
	}
}
