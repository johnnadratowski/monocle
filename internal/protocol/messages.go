package protocol

import (
	"fmt"
	"strings"

	"github.com/josephschmitt/monocle/internal/types"
)

// Inbound message types (from CLI subcommands to engine via socket)
const (
	TypeGetReviewStatus       = "get_review_status"
	TypePollFeedback          = "poll_feedback"
	TypeSubmitContent         = "submit_content"
	TypeSubscribe             = "subscribe"
	TypeConnect               = "connect"
	TypeIdentify              = "identify"
	TypeAddAdditionalFiles    = "add_additional_files"
	TypeRemoveAdditionalFiles = "remove_additional_files"
	TypeSetFileGroups         = "set_file_groups"
	TypeAddAnnotations        = "add_annotations"
	TypeSetReviewName         = "set_review_name"
	TypeMarkActivity          = "mark_activity"
	TypeAwaitReview           = "await_review"
)

// Outbound message types (from engine to CLI subcommands)
const (
	TypeGetReviewStatusResponse       = "get_review_status_response"
	TypePollFeedbackResponse          = "poll_feedback_response"
	TypeSubmitContentResponse         = "submit_content_response"
	TypeSubscribeResponse             = "subscribe_response"
	TypeConnectResponse               = "connect_response"
	TypeEventNotification             = "event_notification"
	TypeAddAdditionalFilesResponse    = "add_additional_files_response"
	TypeRemoveAdditionalFilesResponse = "remove_additional_files_response"
	TypeSetFileGroupsResponse         = "set_file_groups_response"
	TypeAddAnnotationsResponse        = "add_annotations_response"
	TypeSetReviewNameResponse         = "set_review_name_response"
	TypeMarkActivityResponse          = "mark_activity_response"
	TypeAwaitReviewResponse           = "await_review_response"
)

// GetReviewStatusMsg requests the current review state from the engine.
type GetReviewStatusMsg struct {
	Type string `json:"type"`
}

// GetReviewStatusResponse returns the current review state.
type GetReviewStatusResponse struct {
	Type         string `json:"type"`
	Status       string `json:"status"` // "no_feedback" | "pending" | "pause_requested"
	CommentCount int    `json:"comment_count,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// PollFeedbackMsg requests pending feedback, optionally blocking until available.
//
// MaxWaitMs bounds a blocking wait (Wait=true): after that many milliseconds
// with no reviewer submission the engine stops blocking and returns the
// no-feedback result, so a hook never hangs indefinitely when the engine
// outlives an active reviewer. Zero means unbounded (the historical
// behaviour). The bound is ignored while a pause has been explicitly
// requested — a reviewer who pressed P may take as long as they need.
type PollFeedbackMsg struct {
	Type      string `json:"type"`
	Wait      bool   `json:"wait"`
	MaxWaitMs int    `json:"max_wait_ms,omitempty"`
}

// PollFeedbackResponse returns feedback if available.
type PollFeedbackResponse struct {
	Type         string `json:"type"`
	HasFeedback  bool   `json:"has_feedback"`
	Feedback     string `json:"feedback,omitempty"`
	CommentCount int    `json:"comment_count,omitempty"`
	Action       string `json:"action,omitempty"` // "approve" | "request_changes"
}

// SubmitContentMsg sends reviewable content (plans, docs) from the agent.
type SubmitContentMsg struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	IsPlan      bool   `json:"is_plan,omitempty"`
}

// SubmitContentResponse acknowledges content submission.
// ID echoes the stored content's id — the daemon mints a UUID when the
// caller sends an empty ID, so this field lets the caller address the
// just-submitted item (mark reviewed, dismiss, fetch versions).
type SubmitContentResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	ID      string `json:"id,omitempty"`
}

// SubscribeMsg requests a persistent event subscription on this connection.
//
// Passive=true marks the connection as a viewer (e.g. the TUI) that should
// NOT be counted as an attached agent. The server forwards events but skips
// the subscriberCount bookkeeping that Submit() uses to pick push vs queue
// delivery, and suppresses the EventConnectionChanged broadcast that
// otherwise tells the UI "an agent is connected". The zero value (false)
// preserves the existing push-subscriber semantics for backwards
// compatibility with any agent that sends SubscribeMsg directly.
type SubscribeMsg struct {
	Type    string   `json:"type"`
	Events  []string `json:"events"`
	Passive bool     `json:"passive,omitempty"`
}

// SubscribeResponse acknowledges a subscription request.
// ProtocolVersion lets clients detect a stale daemon that predates wire
// features they rely on (e.g. Passive subscribers). A zero value means
// the daemon is older than ProtocolVersion=1 and must be respawned for
// the client to trust newer behaviour.
type SubscribeResponse struct {
	Type            string `json:"type"`
	Success         bool   `json:"success"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
}

// CurrentProtocolVersion is bumped when a wire-protocol change requires
// new client behaviour. Bump this whenever a new client semantically
// depends on a daemon feature that an older daemon would silently drop.
//
//	1 — initial versioned protocol (Passive subscribers, ContentAdded,
//	    presence-flag responses).
const CurrentProtocolVersion = 1

// EventNotification pushes an engine event to a subscribed connection.
type EventNotification struct {
	Type    string         `json:"type"`
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

// ConnectMsg requests a persistent connection with optional event forwarding
// but without becoming a push subscriber. The connection supports request/response
// for tool calls and receives event notifications, but does not increment
// subscriberCount (so Submit() always queues feedback for pull delivery).
type ConnectMsg struct {
	Type   string   `json:"type"`
	Events []string `json:"events,omitempty"`
}

// ConnectResponse acknowledges a connect request. ProtocolVersion mirrors
// SubscribeResponse so the agent's event connection can detect a stale
// daemon (one predating wire features it relies on) the same way the TUI's
// subscribe path does.
type ConnectResponse struct {
	Type            string `json:"type"`
	Success         bool   `json:"success"`
	ProtocolVersion int    `json:"protocol_version,omitempty"`
}

// IdentifyMsg carries the agent's self-reported name (sent after MCP handshake).
type IdentifyMsg struct {
	Type  string `json:"type"`
	Agent string `json:"agent"`
}

// AddAdditionalFilesMsg sends file/directory paths to add for review.
type AddAdditionalFilesMsg struct {
	Type  string   `json:"type"`
	Paths []string `json:"paths"`
}

// AddAdditionalFilesResponse acknowledges additional files submission.
// Added carries the newly-attached files (not the cumulative list) so
// callers can distinguish a fresh add from a no-op de-dup.
//
// AddedPresent disambiguates "new daemon returned an empty Added list"
// (genuinely added zero files) from "old daemon doesn't populate Added at
// all". Without this flag, a JSON `omitempty` on an empty slice is
// indistinguishable on the wire from the field being absent, so the
// client cannot tell whether to trust Added or fall back to a cumulative
// fetch. New daemons always set AddedPresent=true; old daemons leave it
// false.
type AddAdditionalFilesResponse struct {
	Type         string                 `json:"type"`
	Success      bool                   `json:"success"`
	Message      string                 `json:"message,omitempty"`
	Count        int                    `json:"count"`
	Added        []types.AdditionalFile `json:"added,omitempty"`
	AddedPresent bool                   `json:"added_present,omitempty"`
}

// RemoveAdditionalFilesMsg removes previously-added files by path. Paths are
// resolved to absolute by the engine before matching, mirroring add.
type RemoveAdditionalFilesMsg struct {
	Type  string   `json:"type"`
	Paths []string `json:"paths"`
}

// RemoveAdditionalFilesResponse acknowledges removal of additional files.
// Count is the number of files that actually matched and were removed.
type RemoveAdditionalFilesResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count"`
}

// FileGroupEntry is one file's agent-supplied grouping metadata.
type FileGroupEntry struct {
	Path            string `json:"path"`
	Workstream      string `json:"workstream,omitempty"`
	WorkstreamOrder int    `json:"workstream_order,omitempty"`
	Category        string `json:"category,omitempty"`
	Group           string `json:"group,omitempty"`
	GroupOrder      int    `json:"group_order,omitempty"`
	SortIndex       int    `json:"sort_index,omitempty"`
	Criticality     int    `json:"criticality,omitempty"`
}

// SetFileGroupsMsg assigns grouping metadata to changed files. When Replace is
// true the supplied entries fully replace any prior metadata for the session;
// otherwise they are merged in (upsert per path).
type SetFileGroupsMsg struct {
	Type    string           `json:"type"`
	Entries []FileGroupEntry `json:"entries"`
	Replace bool             `json:"replace,omitempty"`
}

// SetFileGroupsResponse acknowledges a grouping update. Count is the number of
// entries applied.
type SetFileGroupsResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count"`
}

// AnnotationEntry is one agent annotation: a rationale attached to a code range
// in a file, with structured doc links.
type AnnotationEntry struct {
	File      string         `json:"file"`
	LineStart int            `json:"line_start"`
	LineEnd   int            `json:"line_end"`
	Summary   string         `json:"summary"`
	Refs      []types.DocRef `json:"refs,omitempty"`
}

// AddAnnotationsMsg attaches agent annotations to code ranges. Replace=true
// clears all annotations for the session first; otherwise it replaces per file.
type AddAnnotationsMsg struct {
	Type    string            `json:"type"`
	Entries []AnnotationEntry `json:"entries"`
	Replace bool              `json:"replace,omitempty"`
}

// SetReviewNameMsg sets a human-friendly name for the current review, shown in
// the TUI's top bar. An empty name clears it.
type SetReviewNameMsg struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type SetReviewNameResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// AnnotationReject explains why one annotation entry failed validation and was
// not stored, so the agent can correct it and resend.
type AnnotationReject struct {
	File      string `json:"file"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Reason    string `json:"reason"`
}

// AddAnnotationsResponse acknowledges an annotation update. Count is the number
// of accepted (stored) entries. Rejected lists entries dropped by validation
// (bad ranges, files outside the review); Warnings flags non-fatal problems
// such as doc refs that don't resolve — the annotation is still stored, but the
// link will read "not found" to the reviewer.
type AddAnnotationsResponse struct {
	Type     string             `json:"type"`
	Success  bool               `json:"success"`
	Message  string             `json:"message,omitempty"`
	Count    int                `json:"count"`
	Rejected []AnnotationReject `json:"rejected,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
}

// Summary renders a human/agent-readable description of the result, including
// any rejected entries and ref-resolution warnings. Shared by the MCP tool and
// the `monocle review annotate` CLI so both report identically.
func (r *AddAnnotationsResponse) Summary() string {
	var b strings.Builder
	b.WriteString(r.Message)
	if len(r.Rejected) > 0 {
		fmt.Fprintf(&b, "\n\nRejected %d entr%s (not stored — fix and resend):",
			len(r.Rejected), plural(len(r.Rejected), "y", "ies"))
		for _, rej := range r.Rejected {
			fmt.Fprintf(&b, "\n  - %s:%d-%d — %s", rej.File, rej.LineStart, rej.LineEnd, rej.Reason)
		}
	}
	if len(r.Warnings) > 0 {
		b.WriteString("\n\nWarnings (annotation stored, but the reviewer will see these refs as unresolved):")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "\n  - %s", w)
		}
	}
	return b.String()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// MarkActivityMsg notifies the engine that a write-tool just fired in the
// current session, marking the session as having unreviewed changes. The
// Stop-hook's AwaitReview call consults this flag to decide whether to
// block the turn or let it end normally.
type MarkActivityMsg struct {
	Type string `json:"type"`
}

// MarkActivityResponse acknowledges an activity mark.
type MarkActivityResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

// AwaitReviewMsg is issued by the Stop hook at turn-end. If the session
// has unreviewed activity (a write-tool fired during the turn), the engine
// blocks until the reviewer submits feedback. Otherwise it returns
// immediately with HasActivity=false so the agent's turn can end cleanly.
type AwaitReviewMsg struct {
	Type string `json:"type"`
	Wait bool   `json:"wait"` // true = block on reviewer; false = snapshot query
	// MaxWaitMs bounds a blocking wait (Wait=true): after that many
	// milliseconds with no reviewer submission the engine stops blocking and
	// returns HasActivity=true without a verdict, so the Stop hook ends the
	// turn normally instead of hanging when the engine outlives an active
	// reviewer. Zero means unbounded (the historical behaviour). The bound is
	// ignored while a pause has been explicitly requested.
	MaxWaitMs int `json:"max_wait_ms,omitempty"`
}

// AwaitReviewResponse reports the outcome of an AwaitReview call.
// When HasActivity is false the turn may end normally; when true with
// Action="approve" the turn ends after the reviewer saw the diff; when
// true with Action="request_changes" the hook converts the feedback into
// a Stop-hook block decision that sends Claude back to work.
type AwaitReviewResponse struct {
	Type        string `json:"type"`
	HasActivity bool   `json:"has_activity"`
	Action      string `json:"action,omitempty"`
	Feedback    string `json:"feedback,omitempty"`
}
