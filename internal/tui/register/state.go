// Package register implements the interactive register/unregister TUI wizard.
//
// One wizard, two modes: register walks the user through picking agents,
// integration modes, and Claude hook groups; unregister walks the user
// through picking which agents to remove and which hook groups (if any) to
// leave behind. The steps and layout are shared; only copy and adapter
// dispatch differ.
package register

import (
	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/tui"
)

// Mode selects which verb the wizard is performing.
type Mode int

const (
	ModeRegister Mode = iota
	ModeUnregister
)

// Step is a wizard phase.
type Step int

const (
	StepAgents Step = iota
	StepClaude
	StepConfirm
	StepExecute
	StepDone
)

// IntegrationChoice is the mode picker value on the agents step.
// "auto" defers to the per-adapter default.
type IntegrationChoice string

const (
	IntegrationAuto   IntegrationChoice = "auto"
	IntegrationMCP    IntegrationChoice = "mcp"
	IntegrationSkills IntegrationChoice = "skills"
)

// Options configures the wizard before it starts. Fields that were set from
// CLI flags lock the corresponding question and display a "(via --flag)"
// annotation so users understand why it's not editable.
type Options struct {
	Mode Mode

	// Theme + keys pulled from the main TUI so the wizard feels native.
	Theme tui.Theme
	Keys  tui.KeyMap

	// Adapters to offer. Register: all adapters. Unregister: only those with
	// existing config at the chosen scope.
	Adapters []adapters.AgentAdapter

	// Global pre-fills scope; GlobalLocked=true means --global was explicit.
	Global       bool
	GlobalLocked bool

	// IntegrationMode pre-fills the integration toggle (register-only).
	// "auto" means "use adapter default" (not locked).
	IntegrationMode       IntegrationChoice
	IntegrationModeLocked bool

	// Register-only hook opt-outs. Set from --no-plan-hook / --no-review-gate.
	SkipPlanHook       bool
	SkipPlanHookLocked bool

	SkipReviewGate       bool
	SkipReviewGateLocked bool

	// Unregister-only hook keep toggles.
	KeepPlanHook       bool
	KeepPlanHookLocked bool

	KeepReviewGate       bool
	KeepReviewGateLocked bool
}


// WizardState is the full state used by every step. Steps read/write fields
// here via the Model; there's no per-step sub-model — all state lives in one
// place so back-navigation is trivial and the persistent "summary" column can
// derive from it without cross-step coupling.
type WizardState struct {
	mode   Mode
	opts   Options
	theme  tui.Theme
	keys   tui.KeyMap
	width  int
	height int

	adapters []adapters.AgentAdapter

	step    Step
	history []Step

	// Agents step
	scope       bool // true = global (user), false = project
	selected    map[string]bool
	integration map[string]IntegrationChoice // per agent
	agentCursor int                          // -1 = scope row, 0..N-1 = agent rows

	// Claude step toggles. In register mode, these are the Skip* values; in
	// unregister mode, they are the Keep* values. The UI copy flips but the
	// underlying two booleans are symmetric.
	planToggle    bool // register: skip?, unregister: keep?
	gateToggle    bool
	claudeCursor  int // 0 = plan, 1 = gate

	// Execute step
	runIndex int
	results  []AgentResult

	cancelled bool
}

// --- messages ---

// advanceMsg requests forward navigation. Handled by the root model.
type advanceMsg struct{}

// backMsg requests backward navigation.
type backMsg struct{}

// cancelMsg aborts the wizard.
type cancelMsg struct{}

// runNextMsg kicks off the next agent in StepExecute.
type runNextMsg struct{}

// agentFinishedMsg reports the outcome of one adapter invocation.
type agentFinishedMsg struct {
	index  int
	result AgentResult
}

// executeDoneMsg means every adapter has finished.
type executeDoneMsg struct{}

// NewWizardState builds the initial state from Options.
func NewWizardState(opts Options) WizardState {
	s := WizardState{
		mode:         opts.Mode,
		opts:         opts,
		theme:        opts.Theme,
		keys:         opts.Keys,
		adapters:     opts.Adapters,
		step:         StepAgents,
		scope:        opts.Global,
		selected:    make(map[string]bool),
		integration: make(map[string]IntegrationChoice),
		agentCursor: 0, // scope is a header toggle, not a selectable row
	}
	for _, a := range opts.Adapters {
		s.integration[a.Name()] = opts.IntegrationMode
		if s.integration[a.Name()] == "" {
			s.integration[a.Name()] = IntegrationAuto
		}
	}
	// Register seeds the Claude toggles from the Skip* flags (true = skip).
	// Unregister seeds from the Keep* flags (true = keep).
	if opts.Mode == ModeRegister {
		s.planToggle = opts.SkipPlanHook
		s.gateToggle = opts.SkipReviewGate
	} else {
		s.planToggle = opts.KeepPlanHook
		s.gateToggle = opts.KeepReviewGate
	}
	return s
}

// resolveIntegrationMode converts an agent's stored IntegrationChoice to the
// adapter-facing IntegrationMode, respecting per-adapter defaults under "auto".
func resolveIntegrationMode(agent string, choice IntegrationChoice) adapters.IntegrationMode {
	switch choice {
	case IntegrationMCP:
		return adapters.ModeMCPTools
	case IntegrationSkills:
		return adapters.ModeSkills
	default:
		if agent == "claude" {
			return adapters.ModeMCPTools
		}
		return adapters.ModeSkills
	}
}

// claudeSelected reports whether the Claude adapter is currently selected.
func (s WizardState) claudeSelected() bool {
	for _, a := range s.adapters {
		if a.Name() == "claude" && s.selected[a.Name()] {
			return true
		}
	}
	return false
}

// anySelected reports whether at least one agent is selected.
func (s WizardState) anySelected() bool {
	for _, a := range s.adapters {
		if s.selected[a.Name()] {
			return true
		}
	}
	return false
}

// selectedAdapters returns the selected adapter list in display order.
func (s WizardState) selectedAdapters() []adapters.AgentAdapter {
	var out []adapters.AgentAdapter
	for _, a := range s.adapters {
		if s.selected[a.Name()] {
			out = append(out, a)
		}
	}
	return out
}

// nextStep returns the step that should follow `step`, honoring the Claude-only
// skip. Used by Advance/Back navigation so both directions stay consistent.
func (s WizardState) nextStep(step Step) Step {
	switch step {
	case StepAgents:
		if s.claudeSelected() {
			return StepClaude
		}
		return StepConfirm
	case StepClaude:
		return StepConfirm
	case StepConfirm:
		return StepExecute
	case StepExecute:
		return StepDone
	}
	return StepDone
}
