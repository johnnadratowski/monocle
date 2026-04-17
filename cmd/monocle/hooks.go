package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/protocol"
)

// HooksCmd groups subcommands invoked by an agent harness in response to
// lifecycle events (plan mode entry/exit, etc). Each subcommand reads the
// agent's hook payload on stdin and emits the agent's expected decision
// JSON on stdout. The caller is always an automated hook runner, not a
// human; all error paths exit 0 with empty stdout so the agent degrades
// to its default behavior rather than hard-blocking.
type HooksCmd struct {
	ExitPlan  ExitPlanHookCmd  `cmd:"" name:"exit-plan" help:"Handle an agent's plan-mode exit: send the plan to the Monocle reviewer and approve or deny based on feedback."`
	EnterPlan EnterPlanHookCmd `cmd:"" name:"enter-plan" help:"Inject review context into the agent right before it begins planning."`
}

// ExitPlanHookCmd handles the agent's plan-mode exit event. For Claude Code
// this is the PermissionRequest hook matched on the ExitPlanMode tool. The
// hook blocks until the reviewer submits in the Monocle TUI, then emits an
// allow/deny decision in the format the invoking agent expects.
type ExitPlanHookCmd struct {
	WorkDirFlag
	Agent  string `help:"Agent whose hook is invoking this command." required:"" enum:"claude"`
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
}

// EnterPlanHookCmd handles the agent's pre-plan event. For Claude Code this
// is the PreToolUse hook matched on the ExitPlanMode tool — it fires right
// before Claude begins drafting its plan. We inject a short context string
// pointing out that the eventual ExitPlanMode will be gated by a Monocle
// reviewer, plus any pending reviewer feedback the agent should address.
type EnterPlanHookCmd struct {
	WorkDirFlag
	Agent  string `help:"Agent whose hook is invoking this command." required:"" enum:"claude"`
	Socket string `help:"Override socket path" env:"MONOCLE_SOCKET" default:""`
}

// hookInput is the common subset of Claude Code's hook payload we care about.
// Other agents will need their own decoder when they're added to the --agent
// enum.
type hookInput struct {
	SessionID      string             `json:"session_id"`
	CWD            string             `json:"cwd"`
	PermissionMode string             `json:"permission_mode"`
	HookEventName  string             `json:"hook_event_name"`
	ToolName       string             `json:"tool_name"`
	ToolInput      hookToolInput      `json:"tool_input"`
}

type hookToolInput struct {
	Plan         string `json:"plan"`
	PlanFilename string `json:"plan_filename"`
}

func decodeHookInput(r io.Reader) (hookInput, error) {
	var in hookInput
	data, err := io.ReadAll(r)
	if err != nil {
		return in, err
	}
	if len(data) == 0 {
		return in, errors.New("empty stdin")
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return in, fmt.Errorf("parse hook input: %w", err)
	}
	return in, nil
}

// firstHeading returns the first markdown H1/H2 from the plan body, stripped
// of leading "#" characters. Falls back to empty string.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
	}
	return ""
}

func (cmd *ExitPlanHookCmd) Run() error {
	in, err := decodeHookInput(os.Stdin)
	if err != nil {
		// No usable payload — let the agent fall back to its default prompt.
		return nil
	}

	switch cmd.Agent {
	case "claude":
		return cmd.runClaude(in)
	default:
		// Unsupported agent — exit cleanly so the harness falls back.
		return nil
	}
}

func (cmd *ExitPlanHookCmd) runClaude(in hookInput) error {
	plan := in.ToolInput.Plan
	if plan == "" {
		return nil
	}

	workdir := cmd.WorkDir
	if workdir == "" && in.CWD != "" {
		workdir = in.CWD
	}

	socketPath, err := resolveSocketForWorkDir(cmd.Socket, workdir)
	if err != nil {
		return nil
	}

	planID := in.ToolInput.PlanFilename
	if planID == "" {
		planID = fmt.Sprintf("exit-plan-%s.md", in.SessionID)
	}
	title := firstHeading(plan)
	if title == "" {
		title = planID
	}

	submit, err := client.Connect(socketPath)
	if err != nil {
		// Engine not running — let Claude show its normal permission prompt.
		return nil
	}
	if _, err := submit.Request(
		&protocol.SubmitContentMsg{
			Type:        protocol.TypeSubmitContent,
			ID:          planID,
			Title:       title,
			Content:     plan,
			ContentType: "md",
			IsPlan:      true,
		},
		client.DefaultTimeout,
	); err != nil {
		submit.Close()
		return nil
	}
	submit.Close()

	// Second connection for the blocking poll — the socket server rejects
	// overlapping blocking calls on the same connection.
	wait, err := client.Connect(socketPath)
	if err != nil {
		return nil
	}
	defer wait.Close()

	resp, err := wait.Request(
		&protocol.PollFeedbackMsg{Type: protocol.TypePollFeedback, Wait: true},
		0,
	)
	if err != nil {
		return nil
	}
	feedback, ok := resp.(*protocol.PollFeedbackResponse)
	if !ok {
		return nil
	}

	return emitClaudePermissionDecision(os.Stdout, feedback)
}

// emitClaudePermissionDecision writes the Claude Code PermissionRequest hook
// response that reflects the reviewer's action.
func emitClaudePermissionDecision(w io.Writer, feedback *protocol.PollFeedbackResponse) error {
	decision := map[string]any{"behavior": "allow"}
	if feedback.Action == "request_changes" {
		msg := feedback.Feedback
		if msg == "" {
			msg = "Reviewer requested changes."
		}
		decision = map[string]any{"behavior": "deny", "message": msg}
	}
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName": "PermissionRequest",
			"decision":      decision,
		},
	}
	return json.NewEncoder(w).Encode(payload)
}

func (cmd *EnterPlanHookCmd) Run() error {
	in, err := decodeHookInput(os.Stdin)
	if err != nil {
		return nil
	}

	switch cmd.Agent {
	case "claude":
		return cmd.runClaude(in)
	default:
		return nil
	}
}

func (cmd *EnterPlanHookCmd) runClaude(in hookInput) error {
	workdir := cmd.WorkDir
	if workdir == "" && in.CWD != "" {
		workdir = in.CWD
	}

	socketPath, err := resolveSocketForWorkDir(cmd.Socket, workdir)
	if err != nil {
		return nil
	}

	// Start with the base context. If the engine is reachable, layer on any
	// pending reviewer state. Timeout here is strict because the PreToolUse
	// hook runs with a 5-second timeout.
	context := "Monocle is running for this session. When you submit a plan via ExitPlanMode, it will be sent to a human reviewer who can approve or request changes — the approval flow is automatic, you do not need to run any review commands yourself."

	c, err := client.Connect(socketPath)
	if err == nil {
		resp, err := c.Request(
			&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus},
			2*time.Second,
		)
		c.Close()
		if err == nil {
			if status, ok := resp.(*protocol.GetReviewStatusResponse); ok {
				if status.Status == "pending" || status.CommentCount > 0 {
					context += fmt.Sprintf(" There are %d unaddressed reviewer comment(s) — read them before finalizing the plan.", status.CommentCount)
				}
			}
		}
	}

	return emitClaudePreToolUseContext(os.Stdout, context)
}

func emitClaudePreToolUseContext(w io.Writer, context string) error {
	payload := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "PreToolUse",
			"additionalContext": context,
		},
	}
	return json.NewEncoder(w).Encode(payload)
}
