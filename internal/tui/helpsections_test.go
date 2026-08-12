package tui

import (
	"strings"
	"testing"
)

func TestKeyRankOrdering(t *testing.T) {
	t.Run("classes run lowercase, upper, digit, punctuation, modified, command", func(t *testing.T) {
		want := []string{"c", "C", "1", "%", "ctrl+g", ":submit"}
		var last int
		for i, k := range want {
			class := keyClass(k)
			if i > 0 && class <= last {
				t.Errorf("%q (class %d) should sort after the previous key (class %d)", k, class, last)
			}
			last = class
		}
	})

	t.Run("a modified key sorts beside its shift variant", func(t *testing.T) {
		rows := sortHelpRows([]helpRow{
			{"ctrl+t", "terminal"},
			{"ctrl+g", "editor"},
			{"ctrl+shift+t", "terminal, takeover"},
			{"ctrl+shift+g", "editor, takeover"},
		})
		var got []string
		for _, r := range rows {
			got = append(got, r.key)
		}
		want := []string{"ctrl+g", "ctrl+shift+g", "ctrl+t", "ctrl+shift+t"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("lowercase sorts before its uppercase twin", func(t *testing.T) {
		rows := sortHelpRows([]helpRow{{"S", "submit"}, {"s", "suggest"}})
		if rows[0].key != "s" {
			t.Errorf("expected s before S, got %v", rows)
		}
	})

	t.Run("primaryKey takes the first key of a pair", func(t *testing.T) {
		for label, want := range map[string]string{
			"j/k":                        "j",
			"S / :submit":                "S",
			"ctrl+d/ctrl+u":              "ctrl+d",
			":ref <rev>":                 ":ref",
			"B / :base-artifact-version": "B",
		} {
			if got := primaryKey(label); got != want {
				t.Errorf("primaryKey(%q) = %q, want %q", label, got, want)
			}
		}
	})
}

// helpSectionsFor renders the modal and returns section title -> row key labels,
// which is what the taxonomy tests assert against.
func helpSectionsFor(t *testing.T, reviewTracking bool) map[string][]string {
	t.Helper()
	km := DefaultKeyMap()
	m := helpModel{active: true, keys: &km, reviewTracking: reviewTracking, width: 110, height: 400, theme: DefaultTheme()}
	out := stripANSISeq(m.View())

	sections := map[string][]string{}
	titles := map[string]bool{}
	for _, s := range helpSectionOrder {
		titles[s] = true
	}
	current := ""
	for _, line := range strings.Split(out, "\n") {
		body := strings.TrimSpace(strings.Trim(line, "│╭╮╰╯─"))
		if titles[body] {
			current = body
			continue
		}
		if current == "" || body == "" {
			continue
		}
		// Row lines are indented four spaces inside the border; continuation
		// lines of a wrapped description are indented further.
		if !strings.HasPrefix(strings.Trim(line, "│"), "    ") || strings.HasPrefix(strings.Trim(line, "│"), "      ") {
			continue
		}
		sections[current] = append(sections[current], strings.Fields(body)[0])
	}
	return sections
}

func TestHelpSectionsAreOrderedAndComplete(t *testing.T) {
	sections := helpSectionsFor(t, true)

	t.Run("every section has rows", func(t *testing.T) {
		for _, title := range helpSectionOrder {
			if len(sections[title]) == 0 {
				t.Errorf("section %q rendered no rows", title)
			}
		}
	})

	t.Run("rows within a section are in key order", func(t *testing.T) {
		for title, keys := range sections {
			if helpUnsorted[title] {
				continue
			}
			rows := make([]helpRow, len(keys))
			for i, k := range keys {
				rows[i] = helpRow{key: k}
			}
			if !helpRowsInOrder(rows) {
				var want []string
				for _, r := range sortHelpRows(rows) {
					want = append(want, r.key)
				}
				t.Errorf("section %q is out of order:\n got %v\nwant %v", title, keys, want)
			}
		}
	})

	t.Run("no binding appears in two sections", func(t *testing.T) {
		// A key may legitimately repeat when it means different things in
		// different panes (x, /, d). What must not repeat is the same key doing
		// the same job in two places, so this checks section membership of the
		// keys that are globally unique.
		seen := map[string]string{}
		dual := map[string]bool{"x": true, "/": true, "d": true, "ctrl+g": true}
		for _, title := range helpSectionOrder {
			for _, k := range sections[title] {
				if dual[k] {
					continue
				}
				if prev, ok := seen[k]; ok && prev != title {
					t.Errorf("key %q appears in both %q and %q", k, prev, title)
				}
				seen[k] = title
			}
		}
	})

	t.Run("the annotation doc key is filed under Open", func(t *testing.T) {
		// The regression that prompted the taxonomy: `o` was under Navigation,
		// where nobody looking for "open the docs" would scan.
		found := false
		for _, k := range sections[secOpen] {
			if k == PrimaryLabel(DefaultKeyMap().OpenDocRef) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q missing from %q; sections were %v", "o", secOpen, sections[secOpen])
		}
	})
}
