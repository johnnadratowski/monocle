package protocol

// Inbound message types (from CLI subcommands to engine via socket)
const (
	TypeGetReviewStatus    = "get_review_status"
	TypePollFeedback       = "poll_feedback"
	TypeSubmitContent      = "submit_content"
	TypeSubscribe          = "subscribe"
	TypeConnect            = "connect"
	TypeIdentify           = "identify"
	TypeAddAdditionalFiles = "add_additional_files"
)

// Outbound message types (from engine to CLI subcommands)
const (
	TypeGetReviewStatusResponse    = "get_review_status_response"
	TypePollFeedbackResponse       = "poll_feedback_response"
	TypeSubmitContentResponse      = "submit_content_response"
	TypeSubscribeResponse          = "subscribe_response"
	TypeConnectResponse            = "connect_response"
	TypeEventNotification          = "event_notification"
	TypeAddAdditionalFilesResponse = "add_additional_files_response"
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
type PollFeedbackMsg struct {
	Type string `json:"type"`
	Wait bool   `json:"wait"`
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
type SubmitContentResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// SubscribeMsg requests a persistent event subscription on this connection.
type SubscribeMsg struct {
	Type   string   `json:"type"`
	Events []string `json:"events"`
}

// SubscribeResponse acknowledges a subscription request.
type SubscribeResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
}

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

// ConnectResponse acknowledges a connect request.
type ConnectResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
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
type AddAdditionalFilesResponse struct {
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count"`
}
