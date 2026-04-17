package register

import (
	"strings"
	"testing"
)

func TestHeader_BreadcrumbSkipsClaudeWhenNotSelected(t *testing.T) {
	m := NewModel(defaultOptsRegister())
	m = windowSize(m, 200, 40)

	// Initial state: nothing selected. Breadcrumb should NOT contain "Claude".
	view := m.View()
	s := collapseBorders(stripANSI(view.Content))
	if strings.Contains(s, "Claude ›") || strings.Contains(s, "› Claude") {
		t.Errorf("breadcrumb should not contain Claude before it's selected; got:\n%s", s)
	}

	// Select Claude, breadcrumb should now contain it.
	m = pressKey(m, " ")
	view = m.View()
	s = collapseBorders(stripANSI(view.Content))
	if !strings.Contains(s, "Claude") {
		t.Errorf("breadcrumb should contain Claude after selecting it; got:\n%s", s)
	}

	// Deselect Claude, breadcrumb should drop Claude again.
	m = pressKey(m, " ")
	view = m.View()
	s = collapseBorders(stripANSI(view.Content))
	if strings.Contains(s, "Claude ›") || strings.Contains(s, "› Claude") {
		t.Errorf("breadcrumb should drop Claude after deselecting; got:\n%s", s)
	}
}
