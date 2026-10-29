package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/josephschmitt/monocle/internal/types"
)

// The top bar names the tour: a PR's tour by its number and name, any other
// by its title. An agent titles a PR's tour "PR #308 · CMP-8 Signup Fixes".
func TestTourBarTitle(t *testing.T) {
	for _, c := range []struct{ title, want string }{
		{"PR #308 · CMP-8 Signup Fixes", "Tour for PR #308: CMP-8 Signup Fixes"},
		{"PR #12: Fix the retry", "Tour for PR #12: Fix the retry"},
		{"PR #12 - Fix the retry", "Tour for PR #12: Fix the retry"},
		{"pr #12 — Fix the retry", "Tour for PR #12: Fix the retry"},
		{"PR #12 Fix the retry", "Tour for PR #12: Fix the retry"},
		{"  PR #7  ", "Tour for PR #7"},
		{"PR #308fix", "Tour: PR #308fix"},
		{"Signup walkthrough", "Tour: Signup walkthrough"},
		{"About PR #308 · x", "Tour: About PR #308 · x"},
		{"", "Tour"},
	} {
		if got := tourBarTitle(c.title); got != c.want {
			t.Errorf("tourBarTitle(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

// The tour's name takes the centre of the top bar while the tour is on, and
// gives it back to the review's name when W turns the tour off.
func TestTheTopBarNamesTheTourWhileItIsOn(t *testing.T) {
	tour := viewsTour()
	tour.Title = "PR #308 · CMP-8 Signup Fixes"
	m, _ := tourAppWith(t, tour, inertViewers(&types.Config{}))
	m.reviewName = "The review"
	m = pressKey(t, m, ".")
	if !m.tour.on {
		t.Fatal("the tour should be on after .")
	}
	if bar := ansi.Strip(m.renderTitleBar()); !strings.Contains(bar, "Tour for PR #308: CMP-8 Signup Fixes") || strings.Contains(bar, "The review") {
		t.Errorf("with the tour on the top bar is %q", bar)
	}
	m = pressKey(t, m, "W")
	if bar := ansi.Strip(m.renderTitleBar()); strings.Contains(bar, "Tour for") || !strings.Contains(bar, "The review") {
		t.Errorf("with the tour off the top bar is %q", bar)
	}
}
