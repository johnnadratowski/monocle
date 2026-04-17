package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/josephschmitt/monocle/internal/protocol"
)

func TestEmitClaudePermissionDecision_Approve(t *testing.T) {
	var buf bytes.Buffer
	if err := emitClaudePermissionDecision(&buf, &protocol.PollFeedbackResponse{
		Action: "approve",
	}); err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	hso := out["hookSpecificOutput"].(map[string]any)
	if hso["hookEventName"] != "PermissionRequest" {
		t.Errorf("wrong event name: %v", hso["hookEventName"])
	}
	decision := hso["decision"].(map[string]any)
	if decision["behavior"] != "allow" {
		t.Errorf("expected behavior=allow for approve action, got %v", decision["behavior"])
	}
	if _, present := decision["message"]; present {
		t.Error("allow decision must not include a message field")
	}
}

func TestEmitClaudePermissionDecision_RequestChanges(t *testing.T) {
	var buf bytes.Buffer
	if err := emitClaudePermissionDecision(&buf, &protocol.PollFeedbackResponse{
		Action:   "request_changes",
		Feedback: "Add error handling for the timeout case.",
	}); err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(buf.Bytes(), &out)
	decision := out["hookSpecificOutput"].(map[string]any)["decision"].(map[string]any)
	if decision["behavior"] != "deny" {
		t.Errorf("expected behavior=deny, got %v", decision["behavior"])
	}
	if decision["message"] != "Add error handling for the timeout case." {
		t.Errorf("feedback should be passed through as message, got %v", decision["message"])
	}
}

func TestEmitClaudePermissionDecision_RequestChangesEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := emitClaudePermissionDecision(&buf, &protocol.PollFeedbackResponse{
		Action: "request_changes",
	}); err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(buf.Bytes(), &out)
	decision := out["hookSpecificOutput"].(map[string]any)["decision"].(map[string]any)
	if decision["message"] == "" {
		t.Error("a deny decision with empty feedback should still carry a non-empty message")
	}
}

func TestEmitClaudePreToolUseContext(t *testing.T) {
	var buf bytes.Buffer
	if err := emitClaudePreToolUseContext(&buf, "hello context"); err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	json.Unmarshal(buf.Bytes(), &out)
	hso := out["hookSpecificOutput"].(map[string]any)
	if hso["hookEventName"] != "PreToolUse" {
		t.Errorf("wrong event name: %v", hso["hookEventName"])
	}
	if hso["additionalContext"] != "hello context" {
		t.Errorf("context missing or wrong: %v", hso["additionalContext"])
	}
	// PreToolUse additions must NOT carry a permissionDecision (that's the
	// PermissionRequest sibling's job).
	if _, present := hso["permissionDecision"]; present {
		t.Error("enter-plan hook must not emit permissionDecision")
	}
}

func TestDecodeHookInput_ParsesClaudePayload(t *testing.T) {
	payload := `{
      "session_id": "sess-123",
      "cwd": "/tmp/project",
      "permission_mode": "plan",
      "hook_event_name": "PermissionRequest",
      "tool_name": "ExitPlanMode",
      "tool_input": {"plan": "# Plan\n- step 1", "planFilePath": "/home/me/.claude/plans/p.md"}
    }`
	in, err := decodeHookInput(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if in.SessionID != "sess-123" {
		t.Errorf("session id: %q", in.SessionID)
	}
	if in.ToolInput.Plan == "" || in.ToolInput.PlanFilePath != "/home/me/.claude/plans/p.md" {
		t.Errorf("tool_input not parsed: %+v", in.ToolInput)
	}
}

func TestDecodeHookInput_EmptyStdin(t *testing.T) {
	if _, err := decodeHookInput(strings.NewReader("")); err == nil {
		t.Error("expected error for empty stdin")
	}
}

func TestFirstHeading(t *testing.T) {
	cases := map[string]string{
		"# Title\nbody":    "Title",
		"intro\n## Second": "Second",
		"no heading here":  "",
	}
	for in, want := range cases {
		if got := firstHeading(in); got != want {
			t.Errorf("firstHeading(%q) = %q, want %q", in, got, want)
		}
	}
}
