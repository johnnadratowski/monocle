package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/josephschmitt/monocle/internal/adapters"
	"github.com/josephschmitt/monocle/internal/client"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed tools.json
var toolsJSON []byte

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var (
	toolsOnce   sync.Once
	toolDescMap map[string]string
)

// toolDescriptions returns a map of tool name → description loaded from tools.json.
func toolDescriptions() map[string]string {
	toolsOnce.Do(func() {
		var defs []toolDef
		if err := json.Unmarshal(toolsJSON, &defs); err != nil {
			panic(fmt.Sprintf("parse embedded tools.json: %v", err))
		}
		toolDescMap = make(map[string]string, len(defs))
		for _, d := range defs {
			toolDescMap[d.Name] = d.Description
		}
	})
	return toolDescMap
}

func registerTools(s *sdkmcp.Server) {
	desc := toolDescriptions()

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "review_status",
		Description: desc["review_status"],
	}, handleReviewStatus)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "get_feedback",
		Description: desc["get_feedback"],
	}, handleGetFeedback)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "send_artifact",
		Description: desc["send_artifact"],
	}, handleSendArtifact)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "add_files",
		Description: desc["add_files"],
	}, handleAddFiles)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "remove_files",
		Description: desc["remove_files"],
	}, handleRemoveFiles)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "set_file_groups",
		Description: desc["set_file_groups"],
	}, handleSetFileGroups)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "add_annotations",
		Description: desc["add_annotations"],
	}, handleAddAnnotations)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "set_review_name",
		Description: desc["set_review_name"],
	}, handleSetReviewName)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "set_base_ref",
		Description: desc["set_base_ref"],
	}, handleSetBaseRef)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "set_repo",
		Description: desc["set_repo"],
	}, handleSetRepo)

	sdkmcp.AddTool(s, &sdkmcp.Tool{
		Name:        "send_diff",
		Description: desc["send_diff"],
	}, handleSendDiff)
}

type setReviewNameParams struct {
	Name  string `json:"name"`
	Force bool   `json:"force,omitempty"`
}

func handleSetReviewName(ctx context.Context, req *sdkmcp.CallToolRequest, params setReviewNameParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.SetReviewNameMsg{Type: protocol.TypeSetReviewName, Name: params.Name, Force: params.Force},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}
	r := resp.(*protocol.SetReviewNameResponse)
	if !r.Success {
		return errResult("%s", r.Message), nil, nil
	}
	return textResult(r.Message), nil, nil
}

// -- Tool parameter types --

type reviewStatusParams struct{}

type getFeedbackParams struct {
	Wait bool `json:"wait,omitempty"`
}

type sendArtifactParams struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type addFilesParams struct {
	Paths []string `json:"paths"`
}

type removeFilesParams struct {
	Paths []string `json:"paths"`
}

type fileGroupEntryParam struct {
	Path            string `json:"path"`
	Workstream      string `json:"workstream,omitempty"`
	WorkstreamOrder int    `json:"workstream_order,omitempty"`
	Category        string `json:"category,omitempty"`
	Group           string `json:"group,omitempty"`
	GroupOrder      int    `json:"group_order,omitempty"`
	SortIndex       int    `json:"sort_index,omitempty"`
	Criticality     int    `json:"criticality,omitempty"`
}

type setFileGroupsParams struct {
	Entries []fileGroupEntryParam `json:"entries"`
	Replace bool                  `json:"replace,omitempty"`
}

type docRefParam struct {
	Kind      string `json:"kind"` // "file" (repo path) or "artifact" (id)
	Doc       string `json:"doc"`
	Label     string `json:"label,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	StartCol  int    `json:"start_col,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	EndCol    int    `json:"end_col,omitempty"`
}

type annotationEntryParam struct {
	File      string        `json:"file"`
	LineStart int           `json:"line_start"`
	LineEnd   int           `json:"line_end"`
	Summary   string        `json:"summary"`
	Refs      []docRefParam `json:"refs,omitempty"`
}

type addAnnotationsParams struct {
	Entries []annotationEntryParam `json:"entries"`
	Replace bool                   `json:"replace,omitempty"`
}

type setBaseRefParams struct {
	// Ref is the commit to diff against (its own changes are excluded). Required
	// unless Reset is true.
	Ref string `json:"ref,omitempty"`
	// Reset reverts to working-tree review (re-enables auto-advance to HEAD).
	Reset bool `json:"reset,omitempty"`
}

type diffPairParam struct {
	// Label names this pane in the sidebar. It need not exist on disk.
	Label string `json:"label,omitempty"`
	// Before is the left side; empty renders the pair as an all-new file.
	Before string `json:"before,omitempty"`
	After  string `json:"after"`
	// Lang is an optional syntax-highlighting hint, e.g. "go", "py".
	Lang string `json:"lang,omitempty"`
}

type sendDiffParams struct {
	Name  string          `json:"name"`
	Pairs []diffPairParam `json:"pairs"`
}

type setRepoParams struct {
	// Path is any path inside the repository/worktree to review. The repo root
	// is derived from it. Empty means the current working directory.
	Path string `json:"path,omitempty"`
}

// -- Tool handlers --

func handleReviewStatus(ctx context.Context, req *sdkmcp.CallToolRequest, _ reviewStatusParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
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
	return textResult(bindingLine(status.RepoRoot, status.ReviewName) + text + groupingNudge(c)), nil, nil
}

// bindingLine renders a one-line header naming the repo (and review, if named)
// the answering engine is bound to, so an agent can confirm which review row it
// is talking to. Returns "" when the engine has no session yet (nothing to name).
func bindingLine(repoRoot, reviewName string) string {
	if repoRoot == "" {
		return ""
	}
	if reviewName != "" {
		return fmt.Sprintf("[repo: %s · review: %s]\n", repoRoot, reviewName)
	}
	return fmt.Sprintf("[repo: %s]\n", repoRoot)
}

// handleSendDiff renders one or more before/after comparisons for the reviewer.
// Both sides come from the call, so the comparison need not correspond to any
// file, commit, or working-tree state: nothing on disk is read and no git
// command runs. Each pair becomes its own artifact, so a multi-pair call gives
// the reviewer several switchable diffs.
func handleSendDiff(ctx context.Context, req *sdkmcp.CallToolRequest, params sendDiffParams) (*sdkmcp.CallToolResult, any, error) {
	if strings.TrimSpace(params.Name) == "" {
		return errResult("name is required"), nil, nil
	}
	if len(params.Pairs) == 0 {
		return errResult("at least one pair is required"), nil, nil
	}

	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	ids := make([]string, 0, len(params.Pairs))
	for i, p := range params.Pairs {
		// A stable id derived from the name and pane means re-sending the same
		// comparison updates it in place instead of stacking duplicates.
		id := diffPairID(params.Name, p.Label, i)
		title := params.Name
		if p.Label != "" {
			// With several panes the label alone is ambiguous in the sidebar, so
			// keep the set name alongside it.
			if len(params.Pairs) > 1 {
				title = params.Name + " — " + p.Label
			} else {
				title = p.Label
			}
		}

		resp, err := c.Request(&protocol.SubmitDiffMsg{
			Type:        protocol.TypeSubmitDiff,
			ID:          id,
			Title:       title,
			Before:      p.Before,
			After:       p.After,
			ContentType: p.Lang,
		}, client.DefaultTimeout)
		if err != nil {
			return errResult("request: %v", err), nil, nil
		}
		out := resp.(*protocol.SubmitDiffResponse)
		if !out.Success {
			return errResult("%s", out.Message), nil, nil
		}
		ids = append(ids, out.ID)
	}

	return textResult(fmt.Sprintf(
		"Sent %d diff(s) for review under %q. The reviewer sees each as a side-by-side comparison; ids: %s",
		len(ids), params.Name, strings.Join(ids, ", "),
	)), nil, nil
}

// diffPairID builds a stable, readable artifact id for a diff pane so repeat
// sends replace rather than accumulate.
func diffPairID(name, label string, index int) string {
	base := slugify(name)
	switch {
	case label != "":
		return "diff-" + base + "-" + slugify(label)
	case index > 0:
		return fmt.Sprintf("diff-%s-%d", base, index+1)
	default:
		return "diff-" + base
	}
}

// slugify reduces a display string to a compact id-safe token.
func slugify(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// handleSetRepo re-points this MCP server at the engine for the repo containing
// the given path. Claude Code launches the MCP server with the *lead's* roots,
// so every teammate binds to the lead's per-repo socket and writes the lead's
// review row; calling set_repo after entering a worktree fixes that with no
// Claude Code change. It autospawns a headless engine for the repo if the
// reviewer hasn't opened Monocle on it yet — the same autospawn the TUI and
// review CLI use, so the human attaches to this very engine when they open
// `monocle -C <repo>`.
func handleSetRepo(ctx context.Context, req *sdkmcp.CallToolRequest, params setRepoParams) (*sdkmcp.CallToolResult, any, error) {
	repoRoot, socket, err := resolveRepoBinding(params.Path)
	if err != nil {
		return errResult("set_repo: %v", err), nil, nil
	}

	if _, _, err := adapters.EnsureServe(adapters.AutoSpawnOptions{RepoRoot: repoRoot, Socket: socket}); err != nil {
		return errResult("set_repo: start engine for %s: %v", repoRoot, err), nil, nil
	}

	bindSocket(socket)

	// Confirm by reading status from the now-bound engine so the response names
	// the repo (and review) the agent now owns.
	c, err := client.ConnectDefault()
	if err != nil {
		return errResult("set_repo: bound to %s but engine unreachable: %v", repoRoot, err), nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.GetReviewStatusMsg{Type: protocol.TypeGetReviewStatus},
		client.DefaultTimeout,
	)
	if err != nil {
		return textResult(fmt.Sprintf("Monocle is now bound to %s.", repoRoot)), nil, nil
	}
	st := resp.(*protocol.GetReviewStatusResponse)
	name := st.ReviewName
	if name == "" {
		name = "(unnamed)"
	}
	return textResult(fmt.Sprintf(
		"Monocle is now bound to %s (review: %s). All review tools now target this repo.",
		repoRoot, name,
	)), nil, nil
}

// groupingNudge returns a reminder to call set_file_groups while changed files
// still lack agent grouping (no group label and no category override). It returns
// "" once every file is grouped, or when there are no changed files, so the
// reminder appears only while there is grouping work to do.
func groupingNudge(c *client.Client) string {
	resp, err := c.Request(
		&protocol.GetChangedFilesMsg{Type: protocol.TypeGetChangedFiles},
		client.DefaultTimeout,
	)
	if err != nil {
		return ""
	}
	r, ok := resp.(*protocol.GetChangedFilesResponse)
	if !ok {
		return ""
	}
	return groupingNudgeText(r.Files)
}

// groupingNudgeText builds the grouping reminder for a set of changed files, or
// "" when there are no files or every file already has agent grouping (a group
// label or category override).
func groupingNudgeText(files []types.ChangedFile) string {
	if len(files) == 0 {
		return ""
	}
	ungrouped := 0
	for _, f := range files {
		if f.GroupLabel == "" && f.Category == "" {
			ungrouped++
		}
	}
	if ungrouped == 0 {
		return ""
	}
	return fmt.Sprintf(
		"\n\n%d of %d changed file(s) are not yet grouped for the reviewer. Call set_file_groups to organize them by stack layer or feature area, ordered entry point -> dependency (e.g. UI -> backend -> database), so the review reads as a top-down story.",
		ungrouped, len(files),
	)
}

func handleGetFeedback(ctx context.Context, req *sdkmcp.CallToolRequest, params getFeedbackParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	// A blocking wait uses the ctx-aware request so that if the agent aborts the
	// tool call (client-side timeout/cancellation), we close the engine
	// connection — which fires the engine's disconnect watcher and releases the
	// server-side wait WITHOUT consuming the verdict. A plain Request(_, 0) would
	// ignore ctx, leaving the engine to deliver a later submission into a
	// response nobody reads and mark it delivered — silently losing it.
	// AckRequired opts into two-phase delivery: the engine holds the verdict as
	// recoverable until we confirm below, so a crash between its write and our
	// read redelivers rather than losing it.
	msg := &protocol.PollFeedbackMsg{
		Type:        protocol.TypePollFeedback,
		Wait:        params.Wait,
		AckRequired: true,
	}
	var resp any
	var reqErr error
	if params.Wait {
		resp, reqErr = c.RequestWithContext(ctx, msg)
	} else {
		resp, reqErr = c.Request(msg, client.DefaultTimeout)
	}
	if reqErr != nil {
		return errResult("request: %v", reqErr), nil, nil
	}

	feedback := resp.(*protocol.PollFeedbackResponse)
	if !feedback.HasFeedback {
		return textResult("No feedback pending."), nil, nil
	}
	// The verdict is in hand and about to be returned to the agent — commit the
	// delivery. Done last so any failure above leaves it recoverable.
	result := textResult(feedback.Feedback)
	client.AckFeedbackDefault(feedback.DeliveryID)
	return result, nil, nil
}

func handleSendArtifact(ctx context.Context, req *sdkmcp.CallToolRequest, params sendArtifactParams) (*sdkmcp.CallToolResult, any, error) {
	// Media files (images, video, audio) are sent by reference: the engine
	// copies the file into managed storage rather than embedding text content.
	var mediaPath string
	content := params.Content
	if params.FilePath != "" && content == "" && types.IsMediaFile(params.FilePath) {
		abs, err := filepath.Abs(params.FilePath)
		if err != nil {
			return errResult("resolve media path: %v", err), nil, nil
		}
		if _, err := os.Stat(abs); err != nil {
			return errResult("media file: %v", err), nil, nil
		}
		mediaPath = abs
		if params.ID == "" {
			params.ID = filepath.Base(params.FilePath)
		}
	} else if content == "" && params.FilePath != "" {
		data, err := os.ReadFile(params.FilePath)
		if err != nil {
			return errResult("read file: %v", err), nil, nil
		}
		content = string(data)
		if params.ID == "" {
			params.ID = filepath.Base(params.FilePath)
		}
	}
	if content == "" && mediaPath == "" {
		return errResult("either content or file_path is required"), nil, nil
	}

	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.SubmitContentMsg{
			Type:        protocol.TypeSubmitContent,
			ID:          params.ID,
			Title:       params.Title,
			Content:     content,
			ContentType: params.ContentType,
			IsPlan:      true,
			MediaPath:   mediaPath,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	submit := resp.(*protocol.SubmitContentResponse)
	// Include the server-minted id when the caller passed an empty ID —
	// without this the agent has no way to address the artifact later
	// (mark reviewed, dismiss, fetch versions).
	body := submit.Message
	if submit.ID != "" {
		body = fmt.Sprintf("%s\nid: %s", submit.Message, submit.ID)
	}
	// Sending an artifact is the natural "here's the review" moment, so remind
	// the agent to group the changed files if it hasn't (self-silences when there
	// are no changed files yet, e.g. an up-front plan).
	return textResult(body + groupingNudge(c)), nil, nil
}

func handleAddFiles(ctx context.Context, req *sdkmcp.CallToolRequest, params addFilesParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
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
	return textResult(add.Message + groupingNudge(c)), nil, nil
}

func handleRemoveFiles(ctx context.Context, req *sdkmcp.CallToolRequest, params removeFilesParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	resp, err := c.Request(
		&protocol.RemoveAdditionalFilesMsg{
			Type:  protocol.TypeRemoveAdditionalFiles,
			Paths: params.Paths,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	rem := resp.(*protocol.RemoveAdditionalFilesResponse)
	if !rem.Success {
		return errResult("%s", rem.Message), nil, nil
	}
	return textResult(rem.Message), nil, nil
}

func handleSetFileGroups(ctx context.Context, req *sdkmcp.CallToolRequest, params setFileGroupsParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	entries := make([]protocol.FileGroupEntry, 0, len(params.Entries))
	for _, e := range params.Entries {
		entries = append(entries, protocol.FileGroupEntry{
			Path:            e.Path,
			Workstream:      e.Workstream,
			WorkstreamOrder: e.WorkstreamOrder,
			Category:        e.Category,
			Group:           e.Group,
			GroupOrder:      e.GroupOrder,
			SortIndex:       e.SortIndex,
			Criticality:     e.Criticality,
		})
	}

	resp, err := c.Request(
		&protocol.SetFileGroupsMsg{
			Type:    protocol.TypeSetFileGroups,
			Entries: entries,
			Replace: params.Replace,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	r := resp.(*protocol.SetFileGroupsResponse)
	if !r.Success {
		return errResult("%s", r.Message), nil, nil
	}
	return textResult(r.Message), nil, nil
}

func handleAddAnnotations(ctx context.Context, req *sdkmcp.CallToolRequest, params addAnnotationsParams) (*sdkmcp.CallToolResult, any, error) {
	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	entries := make([]protocol.AnnotationEntry, 0, len(params.Entries))
	for _, e := range params.Entries {
		refs := make([]types.DocRef, 0, len(e.Refs))
		for _, r := range e.Refs {
			kind := types.DocRefKind(r.Kind)
			if kind != types.DocRefArtifact {
				kind = types.DocRefFile
			}
			refs = append(refs, types.DocRef{
				Kind:      kind,
				Doc:       r.Doc,
				Label:     r.Label,
				StartLine: r.StartLine,
				StartCol:  r.StartCol,
				EndLine:   r.EndLine,
				EndCol:    r.EndCol,
			})
		}
		entries = append(entries, protocol.AnnotationEntry{
			File:      e.File,
			LineStart: e.LineStart,
			LineEnd:   e.LineEnd,
			Summary:   e.Summary,
			Refs:      refs,
		})
	}

	resp, err := c.Request(
		&protocol.AddAnnotationsMsg{
			Type:    protocol.TypeAddAnnotations,
			Entries: entries,
			Replace: params.Replace,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}
	r := resp.(*protocol.AddAnnotationsResponse)
	if !r.Success {
		return errResult("%s", r.Message), nil, nil
	}
	return textResult(r.Summary()), nil, nil
}

func handleSetBaseRef(ctx context.Context, req *sdkmcp.CallToolRequest, params setBaseRefParams) (*sdkmcp.CallToolResult, any, error) {
	if params.Reset == (params.Ref != "") {
		return errResult("provide exactly one of: ref (commit to diff against), or reset=true"), nil, nil
	}

	c, guard := boundClient()
	if guard != nil {
		return guard, nil, nil
	}
	defer c.Close()

	if params.Reset {
		if _, err := c.Request(
			&protocol.SetAutoAdvanceRefMsg{Type: protocol.TypeSetAutoAdvanceRef, Enabled: true},
			client.DefaultTimeout,
		); err != nil {
			return errResult("request: %v", err), nil, nil
		}
		return textResult("Base ref reset to working tree (HEAD)."), nil, nil
	}

	resp, err := c.Request(
		&protocol.SetBaseRefMsg{
			Type:      protocol.TypeSetBaseRef,
			Ref:       params.Ref,
			Exclusive: true,
		},
		client.DefaultTimeout,
	)
	if err != nil {
		return errResult("request: %v", err), nil, nil
	}

	out := resp.(*protocol.SetBaseRefResponse)
	if out.Error != "" {
		return errResult("set base ref: %s", out.Error), nil, nil
	}
	return textResult(fmt.Sprintf("Reviewing changes since %s (your commits included). Resets to HEAD after the reviewer submits.", params.Ref)), nil, nil
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
