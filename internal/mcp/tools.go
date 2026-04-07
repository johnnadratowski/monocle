package mcp

import (
	"context"
	"fmt"

	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/protocol"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerTools(s *sdkmcp.Server) {
	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "review_status",
		Description: "Check the current Monocle review status. Returns whether feedback is pending, a pause was requested, or no feedback is available.",
	}, handleReviewStatus)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "get_feedback",
		Description: "Retrieve review feedback from Monocle. With wait=true, blocks until the reviewer submits feedback.",
	}, handleGetFeedback)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "send_artifact",
		Description: "Send content to Monocle for the reviewer to see. Use this to share plans, code, or other artifacts for review.",
	}, handleSendArtifact)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "add_files",
		Description: "Add file paths to the current Monocle review session.",
	}, handleAddFiles)
}

// -- Tool parameter types --

type reviewStatusParams struct{}

type getFeedbackParams struct {
	Wait bool `json:"wait,omitempty"`
}

type sendArtifactParams struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty" jsonschema:"optional"`
}

type addFilesParams struct {
	Paths []string `json:"paths"`
}

// -- Tool handlers --

func handleReviewStatus(ctx context.Context, req *sdkmcp.CallToolRequest, _ reviewStatusParams) (*sdkmcp.CallToolResult, any, error) {
	c, err := client.ConnectDefault()
	if err != nil {
		return errResult("connect: %v", err), nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	status := resp.(*protocol.GetReviewStatusResponse)
	text := status.Status
	if status.Summary != "" {
		text = status.Summary
	}
	return textResult(text), nil, nil
}

func handleGetFeedback(ctx context.Context, req *sdkmcp.CallToolRequest, params getFeedbackParams) (*sdkmcp.CallToolResult, any, error) {
	c, err := client.ConnectDefault()
	if err != nil {
		return errResult("connect: %v", err), nil, nil
	}
	defer c.Close()

	timeout := client.DefaultTimeout
	if params.Wait {
		timeout = 0 // no deadline — block until feedback
	}

	resp, err := c.Request(
		&protocol.PollFeedbackMsg{Type: protocol.TypePollFeedback, Wait: params.Wait},
		timeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	feedback := resp.(*protocol.PollFeedbackResponse)
	if !feedback.HasFeedback {
		return textResult("No feedback pending."), nil, nil
	}
	return textResult(feedback.Feedback), nil, nil
}

func handleSendArtifact(ctx context.Context, req *sdkmcp.CallToolRequest, params sendArtifactParams) (*sdkmcp.CallToolResult, any, error) {
	c, err := client.ConnectDefault()
	if err != nil {
		return errResult("connect: %v", err), nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.SubmitContentMsg{
			Type:        protocol.TypeSubmitContent,
			ID:          params.ID,
			Title:       params.Title,
			Content:     params.Content,
			ContentType: params.ContentType,
			IsPlan:      true,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	submit := resp.(*protocol.SubmitContentResponse)
	return textResult(submit.Message), nil, nil
}

func handleAddFiles(ctx context.Context, req *sdkmcp.CallToolRequest, params addFilesParams) (*sdkmcp.CallToolResult, any, error) {
	c, err := client.ConnectDefault()
	if err != nil {
		return errResult("connect: %v", err), nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.AddAdditionalFilesMsg{
			Type:  protocol.TypeAddAdditionalFiles,
			Paths: params.Paths,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	add := resp.(*protocol.AddAdditionalFilesResponse)
	return textResult(add.Message), nil, nil
}

// -- Helpers --

func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		},
	}
}

func errResult(format string, args ...any) *sdkmcp.CallToolResult {
	r := &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
		IsError: true,
	}
	return r
}
