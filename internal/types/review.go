package types

import "time"

type FileChangeStatus string

const (
	FileAdded    FileChangeStatus = "added"
	FileModified FileChangeStatus = "modified"
	FileDeleted  FileChangeStatus = "deleted"
	FileRenamed  FileChangeStatus = "renamed"
	FileNone     FileChangeStatus = "none" // no git status (directory mode)
)

type CommentType string

const (
	CommentIssue      CommentType = "issue"
	CommentSuggestion CommentType = "suggestion"
	// CommentQuestion is a comment that wants an answer rather than an edit.
	// Marking one as a note buried it among observations the agent could skip;
	// marking it as an issue demanded a change that may not be needed.
	CommentQuestion CommentType = "question"
	CommentNote     CommentType = "note"
	CommentPraise   CommentType = "praise"
)

// WantsResponse reports whether a comment type obliges the agent to come back:
// an issue or suggestion asks for an edit, a question asks for an answer. Notes
// and praise ask for nothing.
func (t CommentType) WantsResponse() bool {
	return t == CommentIssue || t == CommentSuggestion || t == CommentQuestion
}

type TargetType string

const (
	TargetFile           TargetType = "file"
	TargetContent        TargetType = "content"
	TargetAdditionalFile TargetType = "additional_file"
)

type SubmitAction string

const (
	ActionRequestChanges SubmitAction = "request_changes"
	ActionApprove        SubmitAction = "approve"
	// ActionQuestions asks the agent to answer before going further, without
	// asking for changes. A reviewer who only wants something explained had to
	// choose between approving (which closes the review and reads as "ship it")
	// and requesting changes (which tells the agent to edit code it may not need
	// to touch). This is the third answer: the review stays open exactly as it
	// does for request_changes, and only the message to the agent differs.
	ActionQuestions SubmitAction = "questions"
)

// ClosesReview reports whether an action ends the review. Only approval does;
// everything else leaves the review open for another round, so lifecycle code
// should ask this rather than testing for one specific action and silently
// treating any new one as an approval.
func (a SubmitAction) ClosesReview() bool { return a == ActionApprove }

type ReviewSession struct {
	ID              string
	Agent           string
	RepoRoot        string
	BaseRef         string
	ReviewName      string // optional agent-supplied name for the review (shown in the top bar)
	AutoAdvanceRef  bool   // when true, BaseRef advances to HEAD on refresh; persisted so resume keeps a deliberately-set base
	SelectedRef     string // the commit the reviewer/agent selected as the base (display); empty in auto-advance mode
	ChangedFiles    []ChangedFile
	ContentItems    []ContentItem
	AdditionalFiles []AdditionalFile
	Comments        []ReviewComment
	Annotations     []Annotation
	FileStatuses    map[string]bool // path -> reviewed
	IgnorePatterns  []string
	ReviewRound     int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ChangedFile struct {
	Path     string
	Status   FileChangeStatus
	Reviewed bool

	// Churn, computed from git diff --numstat. Both are 0 for untracked or
	// binary files where git reports no line counts.
	Additions int
	Deletions int

	// Agent-supplied grouping metadata (see SetFileGroups). Empty/zero when the
	// agent has not categorized the file; the TUI falls back to a path heuristic.
	Workstream      string // optional top-level grouping above GroupLabel, e.g. a feature/workstream name
	WorkstreamOrder int    // display order of the workstream (lower first)
	Category        string // overrides the heuristic category when set
	GroupLabel      string // free-form group, e.g. "UI", "Backend", "Database"
	GroupOrder      int    // display order of the group (lower first)
	SortIndex       int    // order within the group (lower first)
	Criticality     int    // optional agent-assigned importance (higher = more critical)

	// ImportOrder is the intra-changeset import rank computed natively by Monocle:
	// a file imported by others sorts before its dependents (dependencies first).
	// 0 when unknown. Used as the within-group sort when the agent gives no
	// explicit SortIndex.
	ImportOrder int
}

type AdditionalFile struct {
	Path     string // absolute filesystem path
	Name     string // display name (basename or relative path)
	Reviewed bool

	// Agent-supplied grouping metadata (see SetFileGroups), matched by Name or
	// Path. Lets agent-attached files participate in the grouped sidebar view.
	Workstream      string
	WorkstreamOrder int
	Category        string
	GroupLabel      string
	GroupOrder      int
	SortIndex       int
}

type ContentItem struct {
	ID           string
	Title        string
	Content      string
	ContentType  string
	IsPlan       bool
	Reviewed     bool
	VersionCount int // number of versions stored (derived from content_versions table)
	Comments     []ReviewComment
	CreatedAt    time.Time
	UpdatedAt    time.Time

	// Media artifacts (images, video, audio). When MediaPath is non-empty the
	// item is a media artifact rather than text/markdown: MediaPath is the
	// absolute path to Monocle's stored copy, MediaType is the category
	// ("image"/"video"/"audio"), and MimeType is the full MIME type.
	MediaPath string
	MediaType string
	MimeType  string
}

// IsMedia reports whether this content item is a media artifact.
func (c ContentItem) IsMedia() bool { return c.MediaPath != "" }

type ContentVersion struct {
	ContentItemID string
	Version       int
	Title         string
	Content       string
	CreatedAt     time.Time
}

type ReviewComment struct {
	ID          string
	TargetType  TargetType
	TargetRef   string // file path or content item ID
	LineStart   int
	LineEnd     int
	Type        CommentType
	Body        string
	CodeSnippet string
	Resolved    bool
	ReviewRound int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Annotation is an agent-authored explanation attached to a specific range of
// changed code, linking out to the docs that motivate it. Annotations are a
// distinct channel from reviewer ReviewComments — they are authored by the agent
// to aid the reviewer and are NEVER included in the feedback sent back to the
// agent.
type Annotation struct {
	ID          string
	TargetRef   string // file path the annotated code lives in
	LineStart   int    // first annotated code line (new-file line number)
	LineEnd     int    // last annotated code line
	Summary     string // one-line rationale shown inline
	Refs        []DocRef
	ReviewRound int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DocRefKind distinguishes a repo file from a sent artifact as a doc target.
type DocRefKind string

const (
	DocRefFile     DocRefKind = "file"     // Doc is a repo-relative file path
	DocRefArtifact DocRefKind = "artifact" // Doc is a content-item (artifact) ID
)

// DocRef links an annotation to a span of a document. The span is a
// line/column range (1-based lines, 0-based columns); zero values mean
// unspecified (start/end of line or document).
type DocRef struct {
	Kind      DocRefKind `json:"kind"`
	Doc       string     `json:"doc"`   // repo-relative file path or artifact id
	Label     string     `json:"label"` // link display text
	StartLine int        `json:"start_line"`
	StartCol  int        `json:"start_col"`
	EndLine   int        `json:"end_line"`
	EndCol    int        `json:"end_col"`
}

type ReviewSubmission struct {
	ID              string
	SessionID       string
	Action          SubmitAction
	FormattedReview string
	CommentCount    int
	ReviewRound     int
	SubmittedAt     time.Time
	DeliveredAt     *time.Time // nil = not yet delivered (queued)
}

type DiffHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Header   string
	Lines    []DiffLine
}

type DiffLineKind string

const (
	DiffLineContext DiffLineKind = "context"
	DiffLineAdded   DiffLineKind = "added"
	DiffLineRemoved DiffLineKind = "removed"
)

type DiffLine struct {
	Kind       DiffLineKind
	OldLineNum int
	NewLineNum int
	Content    string
}

type DiffResult struct {
	Path  string
	Hunks []DiffHunk
}

type ReviewSummary struct {
	Session                *ReviewSession
	FileComments           map[string][]ReviewComment // path -> comments
	ContentComments        map[string][]ReviewComment // item id -> comments
	AdditionalFileComments map[string][]ReviewComment // additional file path -> comments
	IssueCt                int
	SuggestionCt           int
	QuestionCt             int
	NoteCt                 int
	PraiseCt               int
}

type ReviewSnapshot struct {
	ID           int
	SessionID    string
	SubmissionID string
	ReviewRound  int
	HeadRef      string
	BaseRef      string
	Files        []SnapshotFile
	FilesByPath  map[string]*SnapshotFile // built at load time for O(1) lookup
	CreatedAt    time.Time
}

type SnapshotFile struct {
	Path     string
	Status   FileChangeStatus
	Reviewed bool
	BlobSHA  string // git blob SHA (git mode)
	Content  string // raw content fallback (non-git mode)
}

type SessionSummary struct {
	ID           string
	Agent        string
	RepoRoot     string
	FileCount    int
	CommentCount int
	ReviewRound  int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
