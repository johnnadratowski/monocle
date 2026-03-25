package tui

import (
	"testing"

	"github.com/anthropics/monocle/internal/types"
)

func TestAutoFocusMode_EntersOnPlanContentItem(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: true},
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan", IsPlan: true},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	if m.sidebarHidden {
		t.Fatal("sidebar should start visible")
	}
	if m.diffView.wrap {
		t.Fatal("wrap should start disabled")
	}

	result, _ := m.Update(contentItemMsg{id: "plan-1"})
	app := result.(appModel)

	if !app.focusModeActive {
		t.Error("expected focusModeActive to be true")
	}
	if !app.sidebarHidden {
		t.Error("expected sidebar to be hidden in focus mode")
	}
	if !app.diffView.wrap {
		t.Error("expected wrap to be enabled in focus mode")
	}
	if app.focusModeSavedSidebar {
		t.Error("expected saved sidebar state to be false (was visible)")
	}
	if app.focusModeSavedWrap {
		t.Error("expected saved wrap state to be false (was disabled)")
	}
}

func TestAutoFocusMode_DoesNotEnterOnNonPlanContentItem(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: true},
		contentItems: []types.ContentItem{
			{ID: "doc-1", Title: "Doc", IsPlan: false},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	result, _ := m.Update(contentItemMsg{id: "doc-1"})
	app := result.(appModel)

	if app.focusModeActive {
		t.Error("expected focusModeActive to be false for non-plan content")
	}
	if app.sidebarHidden {
		t.Error("expected sidebar to remain visible for non-plan content")
	}
}

func TestAutoFocusMode_ExitsOnSubmit(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: true, ClearAfterSubmit: "never"},
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan", IsPlan: true},
		},
		session: &types.ReviewSession{ID: "test"},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	// Enter focus mode
	result, _ := m.Update(contentItemMsg{id: "plan-1"})
	m = result.(appModel)

	if !m.focusModeActive {
		t.Fatal("expected focusModeActive to be true after entering")
	}

	// Submit review
	result, _ = m.Update(submitSuccessMsg{})
	app := result.(appModel)

	if app.focusModeActive {
		t.Error("expected focusModeActive to be false after submit")
	}
	if app.sidebarHidden {
		t.Error("expected sidebar to be restored to visible")
	}
	if app.diffView.wrap {
		t.Error("expected wrap to be restored to disabled")
	}
}

func TestAutoFocusMode_DoesNotReapplyOnSecondContentItem(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: true},
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan", IsPlan: true},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	// Enter focus mode
	result, _ := m.Update(contentItemMsg{id: "plan-1"})
	m = result.(appModel)

	// User manually toggles sidebar back
	m.sidebarHidden = false

	// Second content item arrives
	result, _ = m.Update(contentItemMsg{id: "plan-1"})
	app := result.(appModel)

	if app.sidebarHidden {
		t.Error("expected sidebar to remain visible after manual toggle + second content item")
	}
	if !app.focusModeActive {
		t.Error("expected focusModeActive to remain true")
	}
}

func TestAutoFocusMode_Disabled_NoEffect(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: false},
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan", IsPlan: true},
		},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40

	result, _ := m.Update(contentItemMsg{id: "plan-1"})
	app := result.(appModel)

	if app.focusModeActive {
		t.Error("expected focusModeActive to be false when config is disabled")
	}
	if app.sidebarHidden {
		t.Error("expected sidebar to remain visible when config is disabled")
	}
}

func TestAutoFocusMode_RestoresCustomState(t *testing.T) {
	engine := &stubEngine{
		cfg: &types.Config{AutoFocusMode: true, Wrap: true, ClearAfterSubmit: "never"},
		contentItems: []types.ContentItem{
			{ID: "plan-1", Title: "Plan", IsPlan: true},
		},
		session: &types.ReviewSession{ID: "test"},
	}
	m := NewApp(engine)
	m.width = 120
	m.height = 40
	m.sidebarHidden = true // user had it hidden already

	// wrap is already true from config
	if !m.diffView.wrap {
		t.Fatal("expected wrap to be true from config")
	}

	result, _ := m.Update(contentItemMsg{id: "plan-1"})
	m = result.(appModel)

	// Saved state should reflect the pre-existing state
	if !m.focusModeSavedSidebar {
		t.Error("expected saved sidebar to be true (was already hidden)")
	}
	if !m.focusModeSavedWrap {
		t.Error("expected saved wrap to be true (was already enabled)")
	}

	// Submit review
	result, _ = m.Update(submitSuccessMsg{})
	app := result.(appModel)

	// Should restore to pre-focus-mode state
	if !app.sidebarHidden {
		t.Error("expected sidebar to remain hidden (original state)")
	}
	if !app.diffView.wrap {
		t.Error("expected wrap to remain enabled (original state)")
	}
}
