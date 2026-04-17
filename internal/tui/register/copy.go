package register

// Strings shown in the wizard. Kept centralized so copy changes don't require
// hunting through step files.
const (
	titleRegister   = "Register"
	titleUnregister = "Unregister"

	helpHint = "enter: next  •  space: toggle  •  shift+tab: back  •  q/esc: cancel"

	// Agents step.
	agentsIntroRegister   = "Pick the agents to register Monocle with."
	agentsIntroUnregister = "Pick the agents to remove Monocle from."
	scopeLabel            = "Scope:"
	scopeUser             = "User"
	scopeProject          = "Project"
	scopeHelp             = "User-level config in $HOME, or per-project config in this repo."
	agentsEmptyUnregister = "Nothing is registered at this scope."

	// Claude step.
	claudeTitleRegister   = "Install Claude Code Hooks (Optional)"
	claudeTitleUnregister = "Remove Claude Code Hooks (Optional)"

	// Shown in faint text directly under the title so the question is the
	// first thing the user reads before the toggles.
	claudeSubtitleRegister   = "Would you like to install any hooks to force a Monocle review at the end of Claude's work?"
	claudeSubtitleUnregister = "Would you like to remove the hooks Monocle previously installed, or leave them in place?"

	// Per-toggle descriptions rendered directly under the checkbox row.
	planToggleDesc = "Route ExitPlanMode through Monocle, so any plan Claude produces needs your sign-off before it runs."
	gateToggleDesc = "Forcibly pause Claude at the end of any turn that edited files, and wait for you to submit a review before the next turn starts."

	// claudeExplain lives below the hook toggles and contains the compat
	// note about the channels flag. No hardcoded line breaks — the wizard
	// word-wraps at render time based on the available width, so the copy
	// flows naturally in both narrow and wide terminals.
	claudeExplain = `Heads up: hooks don't fire when Claude is launched with --dangerously-load-development-channels (MCP channels mode). If you use that flag, these toggles are no-ops at runtime.`

	claudeExplainUnregister = `Heads up: keeping either hook group in place is only useful if you've customized the commands yourself, or want to keep the review gate active while the MCP server entry is torn down. By default both are removed.`

	planToggleRegister   = "Install plan-mode hooks  (ExitPlanMode → Monocle)"
	gateToggleRegister   = "Install turn-end review gate  (PostToolUse + Stop)"
	planToggleUnregister = "Keep plan-mode hooks in settings.json"
	gateToggleUnregister = "Keep turn-end review gate in settings.json"

	// Confirm step.
	confirmTitleRegister   = "Ready to register"
	confirmTitleUnregister = "Ready to unregister"
	confirmHelpRegister    = "These files will be written or updated:"
	confirmHelpUnregister  = "These files will be removed or cleaned up:"

	// Execute step.
	executeTitleRegister   = "Registering…"
	executeTitleUnregister = "Unregistering…"
	executeDone            = "Done. Press enter to close."
)

// hookRow is the audit detail shown under each Claude toggle so power users
// can see exactly what lands in settings.json.
type hookRow struct {
	label string
	note  string
}

var planHookRows = []hookRow{
	{label: "PermissionRequest: ExitPlanMode", note: "hooks exit-plan"},
	{label: "PreToolUse: ExitPlanMode", note: "hooks enter-plan"},
}

var reviewGateRows = []hookRow{
	{label: "PostToolUse: Edit|Write|NotebookEdit|MultiEdit", note: "hooks mark-activity"},
	{label: "Stop", note: "hooks on-stop"},
}
