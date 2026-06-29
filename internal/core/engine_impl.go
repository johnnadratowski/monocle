package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/josephschmitt/monocle/internal/db"
	"github.com/josephschmitt/monocle/internal/imports"
	"github.com/josephschmitt/monocle/internal/protocol"
	"github.com/josephschmitt/monocle/internal/types"
)

// Engine implements EngineAPI and coordinates all Monocle subsystems.
type Engine struct {
	mu sync.RWMutex

	// cfg is held behind atomic.Pointer so concurrent readers (e.g.
	// GetFileDiff reading ContextLines, Submit reading
	// MarkReviewedOnSubmit) don't race with SaveConfig swapping a new
	// value in. Always go through cfgPtr / setCfgPtr — never embed
	// types.Config directly.
	cfg       atomic.Pointer[types.Config]
	database  *db.DB
	git       GitAPI
	server    *SocketServer
	feedback  *FeedbackQueue
	formatter *ReviewFormatter
	sessions  *SessionManager

	current *types.ReviewSession

	// hasUnreviewedActivity is set by handleMarkActivity when Claude fires a
	// write-tool (PostToolUse hook). Cleared when the reviewer's feedback
	// queue is next drained. Used by handleAwaitReview to decide whether a
	// turn needs a review gate or can end normally.
	hasUnreviewedActivity bool

	// autoAdvanceRef: when true, baseRef advances to HEAD on each refresh
	autoAdvanceRef bool
	lastKnownHead  string
	selectedRef    string // the commit the user picked (BaseRef stores its parent for diffing)
	// agentBaseRef: true when the base was set by an agent (set-base-ref) for a
	// single review. It is reverted to auto-advance once the reviewer submits, so
	// the base returns to HEAD after the review concludes.
	agentBaseRef bool

	// reviewBase: when non-nil, the snapshot is the single source of truth for
	// review tracking — file list merging, auto-unmark, and diff computation all
	// use this snapshot as the base. Nil means normal git-based diffing.
	reviewBase *types.ReviewSnapshot

	// autoUnmarked memoizes which files have already been auto-unmarked against
	// a given snapshot SHA (path -> snapshot blob SHA last unmarked against).
	// Without this, a changed file would be force-unmarked on every periodic
	// refresh, clobbering an explicit manual `r` re-mark. Guarded by e.mu.
	// Reset when snapshots are deleted or a session is (re)loaded.
	autoUnmarked map[string]string

	// event subscribers: EventKind -> subscriber ID -> callback
	subscribers map[EventKind]map[int]EventCallback
	nextSubID   int
}

// NewEngine constructs an Engine with all subsystems wired together.
// When nonGitMode is true, a DirClient is used instead of GitClient,
// allowing Monocle to browse non-git directories.
func NewEngine(cfg *types.Config, database *db.DB, repoRoot string, nonGitMode bool) (*Engine, error) {
	var git GitAPI
	if nonGitMode {
		git = NewDirClient(repoRoot, cfg.IgnorePatterns)
	} else {
		git = NewGitClient(repoRoot)
	}
	server := NewSocketServer()
	feedback := NewFeedbackQueue()

	e := &Engine{
		database:       database,
		git:            git,
		server:         server,
		feedback:       feedback,
		sessions:       NewSessionManager(database, git),
		autoAdvanceRef: !nonGitMode,
		subscribers:    make(map[EventKind]map[int]EventCallback),
	}
	e.cfg.Store(cfg)

	e.formatter = NewReviewFormatter(func(path string, start, end int) string {
		content, err := git.FileContent("", path)
		if err != nil {
			return ""
		}
		return extractLines(content, start, end)
	}, cfg.ReviewFormat)

	e.formatter.SetContentItemProvider(func(id string) string {
		e.mu.RLock()
		session := e.current
		e.mu.RUnlock()
		if session == nil {
			return ""
		}
		item, err := database.GetContentItem(session.ID, id)
		if err != nil || item == nil {
			return ""
		}
		return item.Content
	})

	server.SetEngine(e)

	return e, nil
}

// extractLines returns the requested line range (1-based, inclusive) from content.
func extractLines(content string, start, end int) string {
	if start <= 0 {
		return ""
	}
	var lines []byte
	lineNum := 1
	lineStart := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			if lineNum >= start && lineNum <= end {
				line := content[lineStart:i]
				lines = append(lines, []byte(line)...)
				lines = append(lines, '\n')
			}
			if lineNum > end {
				break
			}
			lineNum++
			lineStart = i + 1
		}
	}
	return string(lines)
}

// -- Session lifecycle --

func (e *Engine) StartSession(opts SessionOptions) (*types.ReviewSession, error) {
	session, err := e.sessions.CreateSession(opts)
	if err != nil {
		return nil, err
	}

	if _, _, err := e.sessions.RefreshChangedFiles(session); err != nil {
		return nil, fmt.Errorf("refresh changed files: %w", err)
	}

	e.mu.Lock()
	e.current = session
	// Persist the engine's current auto-advance/selected-ref state onto the new
	// session row so it survives a later resume.
	e.current.AutoAdvanceRef = e.autoAdvanceRef
	e.current.SelectedRef = e.selectedRef
	_ = e.database.UpdateSession(e.current)
	e.hasUnreviewedActivity = false
	e.autoUnmarked = nil
	e.mu.Unlock()

	return session, nil
}

func (e *Engine) ResumeSession(sessionID string) (*types.ReviewSession, error) {
	session, err := e.sessions.ResumeSession(sessionID)
	if err != nil {
		return nil, err
	}

	files, _, err := e.sessions.RefreshChangedFiles(session)
	if err != nil {
		return nil, fmt.Errorf("refresh changed files: %w", err)
	}

	e.mu.Lock()
	e.current = session
	// Apply grouping metadata and import order up front so the first render is
	// already grouped. RefreshChangedFiles re-derives the file list from git
	// (which carries no grouping), so without this the sidebar paints ungrouped
	// and the grouping only layers in on the next refresh tick.
	e.applyFileMetadata(session.ID, files)
	applyImportOrder(session.RepoRoot, files)
	e.current.ChangedFiles = files
	// Restore the persisted base-ref state. Without this, auto-advance defaults
	// back to true and the next refresh advances BaseRef to HEAD — emptying the
	// diff (and the file list) when the reviewed work has since been committed.
	e.autoAdvanceRef = session.AutoAdvanceRef
	e.selectedRef = session.SelectedRef
	e.lastKnownHead = "" // re-detect HEAD; only advances when auto-advance is on
	e.hasUnreviewedActivity = false
	e.autoUnmarked = nil
	// reviewBase stays nil — Working Tree is the default view.
	// Snapshots in DB are used for auto-unmark during refresh.
	e.mu.Unlock()

	return session, nil
}

func (e *Engine) GetSession() *types.ReviewSession {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.current
}

func (e *Engine) ListSessions(opts ListSessionsOptions) ([]types.SessionSummary, error) {
	return e.sessions.ListSessions(opts)
}

// -- Browsing --

func (e *Engine) RefreshChangedFiles() ([]types.ChangedFile, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	// Auto-advance baseRef to HEAD when commits happen
	if e.autoAdvanceRef {
		head, err := e.git.CurrentRef()
		if err == nil && head != e.lastKnownHead {
			e.lastKnownHead = head
			if head != session.BaseRef {
				e.mu.Lock()
				if e.current != nil {
					e.current.BaseRef = head
					e.current.UpdatedAt = time.Now()
					_ = e.database.UpdateSession(e.current)
				}
				e.mu.Unlock()
			}
		}
	}

	// priorReviewed is captured from the DB before RefreshChangedFiles' replace
	// deletes rows. It still contains the reviewed flag for files that the replace
	// prunes (e.g. reverted snapshot-only files), which filesRelativeToSnapshot
	// re-adds — without it, those re-inserted rows would lose their reviewed state.
	files, priorReviewed, err := e.sessions.RefreshChangedFiles(session)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	if e.current != nil && e.current.ID == session.ID {
		if e.IsReviewTrackingEnabled() {
			// Auto-unmark runs against the latest snapshot in DB, independent
			// of the current view (reviewBase). This way reviewed state stays
			// accurate even when the user is viewing Working Tree diffs.
			latestSnap, snapErr := e.latestSnapshotFromDB(session.ID)
			if snapErr != nil {
				e.mu.Unlock()
				return nil, snapErr
			}
			if latestSnap != nil {
				e.autoUnmarkChangedFiles(session, files, latestSnap)
			}

			// When a review base is selected, recompute the file list relative
			// to the snapshot: add reverted files, remove unchanged files.
			if e.reviewBase != nil {
				files = e.filesRelativeToSnapshot(session, files, e.reviewBase, priorReviewed)
			}
		}
		e.applyFileMetadata(session.ID, files)
		applyImportOrder(session.RepoRoot, files)
		e.current.ChangedFiles = files
	}
	e.mu.Unlock()

	return files, nil
}

// applyImportOrder computes the intra-changeset import reading order over the
// changed files and stores each file's rank on it (dependencies first). It reads
// the working-tree file contents; failures degrade to rank 0 (no ordering).
func applyImportOrder(repoRoot string, files []types.ChangedFile) {
	if repoRoot == "" || len(files) == 0 {
		return
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	ranks := imports.Order(repoRoot, paths)
	for i := range files {
		files[i].ImportOrder = ranks[files[i].Path]
	}
}

// applyFileMetadata merges agent-supplied grouping metadata (stored separately
// from changed_files so it survives refreshes) into the given files in place.
// Must be called with e.mu held.
func (e *Engine) applyFileMetadata(sessionID string, files []types.ChangedFile) {
	meta, err := e.database.GetFileMetadata(sessionID)
	if err != nil || len(meta) == 0 {
		return
	}
	for i := range files {
		if m, ok := meta[files[i].Path]; ok {
			files[i].Workstream = m.Workstream
			files[i].WorkstreamOrder = m.WorkstreamOrder
			files[i].Category = m.Category
			files[i].GroupLabel = m.GroupLabel
			files[i].GroupOrder = m.GroupOrder
			files[i].SortIndex = m.SortIndex
			files[i].Criticality = m.Criticality
		}
	}
}

// handleSetFileGroups persists agent-supplied grouping metadata, refreshes the
// changed-file list so the new grouping is reflected, and notifies the TUI.
func (e *Engine) handleSetFileGroups(msg *protocol.SetFileGroupsMsg) *protocol.SetFileGroupsResponse {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return &protocol.SetFileGroupsResponse{
			Type:    protocol.TypeSetFileGroupsResponse,
			Success: false,
			Message: "no active session",
		}
	}

	metas := make([]types.ChangedFile, 0, len(msg.Entries))
	for _, en := range msg.Entries {
		metas = append(metas, types.ChangedFile{
			Path:            en.Path,
			Workstream:      en.Workstream,
			WorkstreamOrder: en.WorkstreamOrder,
			Category:        en.Category,
			GroupLabel:      en.Group,
			GroupOrder:      en.GroupOrder,
			SortIndex:       en.SortIndex,
			Criticality:     en.Criticality,
		})
	}
	if err := e.database.SetFileMetadata(session.ID, metas, msg.Replace); err != nil {
		return &protocol.SetFileGroupsResponse{
			Type:    protocol.TypeSetFileGroupsResponse,
			Success: false,
			Message: err.Error(),
		}
	}

	// Merge the new metadata into the in-memory view and notify the TUI.
	e.mu.Lock()
	if e.current != nil && e.current.ID == session.ID {
		e.applyFileMetadata(session.ID, e.current.ChangedFiles)
	}
	e.mu.Unlock()
	e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})

	return &protocol.SetFileGroupsResponse{
		Type:    protocol.TypeSetFileGroupsResponse,
		Success: true,
		Message: fmt.Sprintf("Grouped %d file(s)", len(metas)),
		Count:   len(metas),
	}
}

func (e *Engine) GetChangedFiles() []types.ChangedFile {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.current == nil {
		return nil
	}
	return e.current.ChangedFiles
}

func (e *Engine) GetContentItems() []types.ContentItem {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.current == nil {
		return nil
	}
	return e.current.ContentItems
}

func (e *Engine) GetFileDiff(path string) (*types.DiffResult, error) {
	return e.getFileDiff(path, false)
}

// GetFileDiffFull returns the diff for a file with full-file context (the whole
// file shown with diff coloring) rather than only the lines around each change.
func (e *Engine) GetFileDiffFull(path string) (*types.DiffResult, error) {
	return e.getFileDiff(path, true)
}

func (e *Engine) getFileDiff(path string, full bool) (*types.DiffResult, error) {
	e.mu.RLock()
	session := e.current
	snapshot := e.reviewBase
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	// Review base mode: diff stored content against current working tree
	if snapshot != nil {
		return e.snapshotFileDiff(snapshot, path, full)
	}

	ctxLines := 0
	if cfg := e.cfg.Load(); cfg != nil {
		ctxLines = cfg.ContextLines
	}
	if full {
		ctxLines = -1
	}
	return e.git.FileDiff(session.BaseRef, path, ctxLines)
}

func (e *Engine) GetFileContent(path string) (string, error) {
	return e.git.FileContent("", path)
}

func (e *Engine) GetContentItem(id string) (*types.ContentItem, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	return e.database.GetContentItem(session.ID, id)
}

// GetContentDiff computes a diff between the previous and current version of a content item.
// Returns nil if no previous version exists.
func (e *Engine) GetContentDiff(id string) (*types.DiffResult, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	versions, err := e.database.GetContentVersions(session.ID, id)
	if err != nil {
		return nil, err
	}
	if len(versions) < 2 {
		return nil, nil
	}
	prev := versions[len(versions)-2]
	curr := versions[len(versions)-1]
	hunks, err := TextDiff(prev.Content, curr.Content)
	if err != nil {
		return nil, fmt.Errorf("compute content diff: %w", err)
	}
	return &types.DiffResult{Path: id, Hunks: hunks}, nil
}

// GetContentVersions returns all versions of a content item.
func (e *Engine) GetContentVersions(id string) ([]types.ContentVersion, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	return e.database.GetContentVersions(session.ID, id)
}

// GetContentDiffBetweenVersions computes a diff between two specific versions of a content item.
func (e *Engine) GetContentDiffBetweenVersions(id string, fromVersion, toVersion int) (*types.DiffResult, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	from, err := e.database.GetContentVersion(session.ID, id, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("get version %d: %w", fromVersion, err)
	}
	to, err := e.database.GetContentVersion(session.ID, id, toVersion)
	if err != nil {
		return nil, fmt.Errorf("get version %d: %w", toVersion, err)
	}
	hunks, err := TextDiff(from.Content, to.Content)
	if err != nil {
		return nil, fmt.Errorf("compute version diff: %w", err)
	}
	return &types.DiffResult{Path: id, Hunks: hunks}, nil
}

// -- Additional files --

func (e *Engine) GetAdditionalFiles() []types.AdditionalFile {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.current == nil {
		return nil
	}
	// Merge agent grouping metadata into a copy so additional files can
	// participate in the grouped view without mutating the stored slice.
	afs := make([]types.AdditionalFile, len(e.current.AdditionalFiles))
	copy(afs, e.current.AdditionalFiles)
	e.applyAdditionalFileMetadata(e.current.ID, afs)
	return afs
}

// GetAnnotations returns the agent-authored annotations for the active session.
func (e *Engine) GetAnnotations() []types.Annotation {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.current == nil {
		return nil
	}
	return e.current.Annotations
}

// handleSetReviewName records the agent-supplied review name on the current
// session and notifies the TUI so the top bar updates.
func (e *Engine) handleSetReviewName(msg *protocol.SetReviewNameMsg) *protocol.SetReviewNameResponse {
	name := strings.TrimSpace(msg.Name)
	e.mu.Lock()
	if e.current == nil {
		e.mu.Unlock()
		return &protocol.SetReviewNameResponse{
			Type:    protocol.TypeSetReviewNameResponse,
			Success: false,
			Message: "no active session",
		}
	}
	e.current.ReviewName = name
	_ = e.database.UpdateSession(e.current) // persist so the name survives a resume
	e.mu.Unlock()

	e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})
	return &protocol.SetReviewNameResponse{
		Type:    protocol.TypeSetReviewNameResponse,
		Success: true,
		Message: fmt.Sprintf("Review name set to %q", name),
	}
}

// handleAddAnnotations persists agent annotations, refreshes the in-memory view,
// and notifies the TUI. Annotations are a separate channel from reviewer
// comments and are never returned to the agent as feedback.
func (e *Engine) handleAddAnnotations(msg *protocol.AddAnnotationsMsg) *protocol.AddAnnotationsResponse {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return &protocol.AddAnnotationsResponse{
			Type:    protocol.TypeAddAnnotationsResponse,
			Success: false,
			Message: "no active session",
		}
	}

	// Annotations render on the diff, so only the review's changed files can carry
	// them; build the allow-set once.
	changed := make(map[string]bool)
	for _, f := range e.GetChangedFiles() {
		changed[f.Path] = true
	}

	var (
		anns     = make([]types.Annotation, 0, len(msg.Entries))
		rejected []protocol.AnnotationReject
		warnings []string
	)
	for _, en := range msg.Entries {
		start, end := en.LineStart, en.LineEnd
		if end == 0 {
			end = start // single-line shorthand
		}
		reject := func(reason string) {
			rejected = append(rejected, protocol.AnnotationReject{
				File: en.File, LineStart: en.LineStart, LineEnd: en.LineEnd, Reason: reason,
			})
		}
		switch {
		case start < 1:
			reject("line_start must be >= 1")
			continue
		case end < start:
			reject(fmt.Sprintf("line_end (%d) must be >= line_start (%d)", end, start))
			continue
		case strings.TrimSpace(en.Summary) == "":
			reject("summary is required")
			continue
		case !changed[en.File]:
			reject("file is not one of the review's changed files")
			continue
		}
		// Unresolvable refs are a warning, not a rejection: the annotation is still
		// useful, the link just reads "not found" to the reviewer.
		for _, r := range en.Refs {
			if reason := e.docRefResolveError(r); reason != "" {
				warnings = append(warnings, fmt.Sprintf("%s:%d — %s", en.File, start, reason))
			}
		}
		anns = append(anns, types.Annotation{
			TargetRef:   en.File,
			LineStart:   start,
			LineEnd:     end,
			Summary:     en.Summary,
			Refs:        en.Refs,
			ReviewRound: session.ReviewRound,
		})
	}

	// Apply when there's something to store, or when a replace was requested (so a
	// bare replace can clear the file/session). Skip the write — and the clear —
	// when nothing is accepted and replace is off, to avoid no-op churn.
	if len(anns) > 0 || msg.Replace {
		if err := e.database.SetAnnotations(session.ID, anns, msg.Replace); err != nil {
			return &protocol.AddAnnotationsResponse{
				Type:    protocol.TypeAddAnnotationsResponse,
				Success: false,
				Message: err.Error(),
			}
		}

		// Reload from the DB so the in-memory copy carries minted IDs, then notify.
		if reloaded, err := e.database.GetAnnotations(session.ID); err == nil {
			e.mu.Lock()
			if e.current != nil && e.current.ID == session.ID {
				e.current.Annotations = reloaded
			}
			e.mu.Unlock()
		}
		e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})
	}

	return &protocol.AddAnnotationsResponse{
		Type:     protocol.TypeAddAnnotationsResponse,
		Success:  true,
		Message:  fmt.Sprintf("Added %d annotation(s)", len(anns)),
		Count:    len(anns),
		Rejected: rejected,
		Warnings: warnings,
	}
}

// docRefResolveError returns a human-readable reason when a doc ref can't be
// resolved against the current session, or "" when it resolves. Drives the
// non-fatal warnings on add_annotations so the agent learns about typo'd links.
func (e *Engine) docRefResolveError(r types.DocRef) string {
	if strings.TrimSpace(r.Doc) == "" {
		return "ref is missing a doc path"
	}
	switch r.Kind {
	case types.DocRefArtifact:
		if item, err := e.GetContentItem(r.Doc); err != nil || item == nil {
			return fmt.Sprintf("artifact ref %q could not be resolved", r.Doc)
		}
	default: // file
		if _, err := e.GetFileContent(r.Doc); err != nil {
			return fmt.Sprintf("file ref %q could not be resolved", r.Doc)
		}
	}
	return ""
}

// applyAdditionalFileMetadata merges grouping metadata onto additional files,
// matched by display Name first then absolute Path (the agent may reference
// either). Safe to call under e.mu held (it only reads the DB and mutates afs).
func (e *Engine) applyAdditionalFileMetadata(sessionID string, afs []types.AdditionalFile) {
	meta, err := e.database.GetFileMetadata(sessionID)
	if err != nil || len(meta) == 0 {
		return
	}
	for i := range afs {
		m, ok := meta[afs[i].Name]
		if !ok {
			m, ok = meta[afs[i].Path]
		}
		if ok {
			afs[i].Workstream = m.Workstream
			afs[i].WorkstreamOrder = m.WorkstreamOrder
			afs[i].Category = m.Category
			afs[i].GroupLabel = m.GroupLabel
			afs[i].GroupOrder = m.GroupOrder
			afs[i].SortIndex = m.SortIndex
		}
	}
}

func (e *Engine) AddAdditionalPaths(paths []string) ([]types.AdditionalFile, error) {
	e.mu.Lock()
	if e.current == nil {
		e.mu.Unlock()
		return nil, fmt.Errorf("no active session")
	}
	session := e.current

	// Build set of existing paths for dedup
	existing := make(map[string]bool, len(session.AdditionalFiles))
	for _, af := range session.AdditionalFiles {
		existing[af.Path] = true
	}

	var added []types.AdditionalFile
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			continue
		}

		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		if info.IsDir() {
			_ = filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				name := d.Name()
				if d.IsDir() {
					// Skip hidden and noisy directories
					if strings.HasPrefix(name, ".") || name == "node_modules" {
						return filepath.SkipDir
					}
					return nil
				}
				// Skip hidden files and .DS_Store
				if strings.HasPrefix(name, ".") {
					return nil
				}
				if !d.Type().IsRegular() {
					return nil
				}
				if existing[path] {
					return nil
				}
				relName, _ := filepath.Rel(absPath, path)
				if relName == "" {
					relName = filepath.Base(path)
				}
				af := types.AdditionalFile{
					Path: path,
					Name: relName,
				}
				session.AdditionalFiles = append(session.AdditionalFiles, af)
				existing[path] = true
				added = append(added, af)
				return nil
			})
		} else {
			if existing[absPath] {
				continue
			}
			af := types.AdditionalFile{
				Path: absPath,
				Name: filepath.Base(absPath),
			}
			session.AdditionalFiles = append(session.AdditionalFiles, af)
			existing[absPath] = true
			added = append(added, af)
		}
	}
	// Persist to DB
	for i := range added {
		if err := e.database.UpsertAdditionalFile(session.ID, &added[i]); err != nil {
			e.mu.Unlock()
			return nil, fmt.Errorf("persist additional file %s: %w", added[i].Path, err)
		}
	}
	e.mu.Unlock()

	for _, af := range added {
		e.emit(EventAdditionalFileAdded, EventPayload{
			Kind: EventAdditionalFileAdded,
			Path: af.Path,
		})
	}

	return added, nil
}

func (e *Engine) GetAdditionalFileContent(absPath string) (string, error) {
	e.mu.RLock()
	found := false
	if e.current != nil {
		for _, af := range e.current.AdditionalFiles {
			if af.Path == absPath {
				found = true
				break
			}
		}
	}
	e.mu.RUnlock()

	if !found {
		return "", fmt.Errorf("path not in additional files: %s", absPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("read additional file: %w", err)
	}
	return string(data), nil
}

func (e *Engine) handleAddAdditionalFiles(msg *protocol.AddAdditionalFilesMsg) *protocol.AddAdditionalFilesResponse {
	added, err := e.AddAdditionalPaths(msg.Paths)
	if err != nil {
		return &protocol.AddAdditionalFilesResponse{
			Type:    protocol.TypeAddAdditionalFilesResponse,
			Success: false,
			Message: err.Error(),
		}
	}

	return &protocol.AddAdditionalFilesResponse{
		Type:         protocol.TypeAddAdditionalFilesResponse,
		Success:      true,
		Message:      fmt.Sprintf("Added %d file(s) for review", len(added)),
		Count:        len(added),
		Added:        added,
		AddedPresent: true,
	}
}

func (e *Engine) handleRemoveAdditionalFiles(msg *protocol.RemoveAdditionalFilesMsg) *protocol.RemoveAdditionalFilesResponse {
	before := len(e.GetAdditionalFiles())
	var firstErr error
	for _, p := range msg.Paths {
		if err := e.RemoveAdditionalFile(p); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return &protocol.RemoveAdditionalFilesResponse{
			Type:    protocol.TypeRemoveAdditionalFilesResponse,
			Success: false,
			Message: firstErr.Error(),
		}
	}
	removed := before - len(e.GetAdditionalFiles())
	return &protocol.RemoveAdditionalFilesResponse{
		Type:    protocol.TypeRemoveAdditionalFilesResponse,
		Success: true,
		Message: fmt.Sprintf("Removed %d file(s) from review", removed),
		Count:   removed,
	}
}

// RemoveAdditionalFile removes a single additional (added) file from the review
// by its path, deleting it and any comments left on it. It is the per-file
// counterpart to add_files; the reviewer triggers it with the dismiss key and
// the remove_files agent tool removes them in batches. The path is resolved to
// absolute (like add) so callers may pass relative paths.
func (e *Engine) RemoveAdditionalFile(path string) error {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	e.mu.Lock()

	if e.current == nil {
		e.mu.Unlock()
		return fmt.Errorf("no active session")
	}

	sessionID := e.current.ID
	if err := e.database.DeleteAdditionalFile(sessionID, path); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("delete additional file: %w", err)
	}
	if err := e.database.DeleteCommentsByTarget(sessionID, types.TargetAdditionalFile, path); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("delete additional file comments: %w", err)
	}

	files := e.current.AdditionalFiles[:0]
	for _, f := range e.current.AdditionalFiles {
		if f.Path != path {
			files = append(files, f)
		}
	}
	e.current.AdditionalFiles = files

	comments := e.current.Comments[:0]
	for _, c := range e.current.Comments {
		if c.TargetType == types.TargetAdditionalFile && c.TargetRef == path {
			continue
		}
		comments = append(comments, c)
	}
	e.current.Comments = comments

	e.mu.Unlock()

	// emit must be called without the lock held (see emit doc comment).
	e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})
	return nil
}

// -- Commenting --

func (e *Engine) AddComment(target CommentTarget, commentType types.CommentType, body string) (*types.ReviewComment, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	now := time.Now()
	comment := &types.ReviewComment{
		ID:          uuid.New().String(),
		TargetType:  target.TargetType,
		TargetRef:   target.TargetRef,
		LineStart:   target.LineStart,
		LineEnd:     target.LineEnd,
		Type:        commentType,
		Body:        body,
		ReviewRound: session.ReviewRound,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := e.database.CreateComment(session.ID, comment); err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}

	e.mu.Lock()
	session.Comments = append(session.Comments, *comment)
	e.mu.Unlock()

	return comment, nil
}

func (e *Engine) EditComment(commentID string, commentType types.CommentType, body string) (*types.ReviewComment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return nil, fmt.Errorf("no active session")
	}

	var found *types.ReviewComment
	for i := range e.current.Comments {
		if e.current.Comments[i].ID == commentID {
			found = &e.current.Comments[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("comment %s not found", commentID)
	}

	found.Type = commentType
	found.Body = body
	found.UpdatedAt = time.Now()

	if err := e.database.UpdateComment(found); err != nil {
		return nil, fmt.Errorf("update comment: %w", err)
	}

	result := *found
	return &result, nil
}

func (e *Engine) DeleteComment(commentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.DeleteComment(commentID); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}

	comments := e.current.Comments[:0]
	for _, c := range e.current.Comments {
		if c.ID != commentID {
			comments = append(comments, c)
		}
	}
	e.current.Comments = comments

	return nil
}

func (e *Engine) ResolveComment(commentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	var found *types.ReviewComment
	for i := range e.current.Comments {
		if e.current.Comments[i].ID == commentID {
			found = &e.current.Comments[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("comment %s not found", commentID)
	}

	found.Resolved = !found.Resolved
	found.UpdatedAt = time.Now()

	if err := e.database.ResolveComment(commentID, found.Resolved); err != nil {
		return fmt.Errorf("resolve comment: %w", err)
	}

	return nil
}

func (e *Engine) ClearComments() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.ClearComments(e.current.ID); err != nil {
		return fmt.Errorf("clear comments: %w", err)
	}

	e.current.Comments = nil

	return nil
}

// ClearReview resets the current review to a blank slate: clears all comments,
// content items/plans, and reviewed states. Does not advance the round or
// create a submission record.
func (e *Engine) ClearReview() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	sessionID := e.current.ID

	if err := e.database.ClearComments(sessionID); err != nil {
		return fmt.Errorf("clear comments: %w", err)
	}
	e.current.Comments = nil

	if err := e.database.DeleteContentItems(sessionID); err != nil {
		return fmt.Errorf("clear content items: %w", err)
	}
	e.current.ContentItems = nil

	// Added files are part of the review view, so a full clear removes them too
	// (their comments are already gone via ClearComments above).
	if err := e.database.DeleteAdditionalFiles(sessionID); err != nil {
		return fmt.Errorf("clear additional files: %w", err)
	}
	e.current.AdditionalFiles = nil

	if err := e.database.DeleteAnnotations(sessionID); err != nil {
		return fmt.Errorf("clear annotations: %w", err)
	}
	e.current.Annotations = nil

	if err := e.database.ResetAllReviewed(sessionID); err != nil {
		return fmt.Errorf("reset reviewed: %w", err)
	}
	for i := range e.current.ChangedFiles {
		e.current.ChangedFiles[i].Reviewed = false
	}
	for k := range e.current.FileStatuses {
		e.current.FileStatuses[k] = false
	}

	// Wipe snapshot history — :clear is a full reset
	e.deleteSnapshots()

	return nil
}

// -- Review status --

func (e *Engine) MarkReviewed(path string) error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	// Check additional files first
	for i := range e.current.AdditionalFiles {
		if e.current.AdditionalFiles[i].Path == path {
			e.current.AdditionalFiles[i].Reviewed = true
			return e.database.MarkAdditionalFileReviewed(e.current.ID, path, true)
		}
	}

	if err := e.database.MarkFileReviewed(e.current.ID, path, true); err != nil {
		return fmt.Errorf("mark reviewed: %w", err)
	}

	e.current.FileStatuses[path] = true
	for i := range e.current.ChangedFiles {
		if e.current.ChangedFiles[i].Path == path {
			e.current.ChangedFiles[i].Reviewed = true
			break
		}
	}

	return nil
}

func (e *Engine) UnmarkReviewed(path string) error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	// Check additional files first
	for i := range e.current.AdditionalFiles {
		if e.current.AdditionalFiles[i].Path == path {
			e.current.AdditionalFiles[i].Reviewed = false
			return e.database.MarkAdditionalFileReviewed(e.current.ID, path, false)
		}
	}

	if err := e.database.MarkFileReviewed(e.current.ID, path, false); err != nil {
		return fmt.Errorf("unmark reviewed: %w", err)
	}

	e.current.FileStatuses[path] = false
	for i := range e.current.ChangedFiles {
		if e.current.ChangedFiles[i].Path == path {
			e.current.ChangedFiles[i].Reviewed = false
			break
		}
	}

	return nil
}

// DismissArtifact permanently removes an artifact from the current session,
// along with its version history and any comments targeting it.
func (e *Engine) DismissArtifact(id string) error {
	e.mu.Lock()

	if e.current == nil {
		e.mu.Unlock()
		return fmt.Errorf("no active session")
	}

	sessionID := e.current.ID
	if err := e.database.DeleteContentItem(sessionID, id); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("delete content item: %w", err)
	}
	if err := e.database.DeleteCommentsByTarget(sessionID, types.TargetContent, id); err != nil {
		e.mu.Unlock()
		return fmt.Errorf("delete artifact comments: %w", err)
	}

	items := e.current.ContentItems[:0]
	for _, item := range e.current.ContentItems {
		if item.ID != id {
			items = append(items, item)
		}
	}
	e.current.ContentItems = items

	comments := e.current.Comments[:0]
	for _, c := range e.current.Comments {
		if c.TargetType == types.TargetContent && c.TargetRef == id {
			continue
		}
		comments = append(comments, c)
	}
	e.current.Comments = comments

	e.mu.Unlock()

	// emit must be called without the lock held (see emit doc comment).
	e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})
	return nil
}

func (e *Engine) MarkContentReviewed(id string) error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.MarkContentItemReviewed(e.current.ID, id, true); err != nil {
		return fmt.Errorf("mark content reviewed: %w", err)
	}

	for i := range e.current.ContentItems {
		if e.current.ContentItems[i].ID == id {
			e.current.ContentItems[i].Reviewed = true
			break
		}
	}

	return nil
}

func (e *Engine) UnmarkContentReviewed(id string) error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.MarkContentItemReviewed(e.current.ID, id, false); err != nil {
		return fmt.Errorf("unmark content reviewed: %w", err)
	}

	for i := range e.current.ContentItems {
		if e.current.ContentItems[i].ID == id {
			e.current.ContentItems[i].Reviewed = false
			break
		}
	}

	return nil
}

func (e *Engine) ResetAllReviewed() error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.ResetAllReviewed(e.current.ID); err != nil {
		return fmt.Errorf("reset all reviewed: %w", err)
	}

	for i := range e.current.ChangedFiles {
		e.current.ChangedFiles[i].Reviewed = false
	}
	for i := range e.current.AdditionalFiles {
		e.current.AdditionalFiles[i].Reviewed = false
	}
	for i := range e.current.ContentItems {
		e.current.ContentItems[i].Reviewed = false
	}
	for k := range e.current.FileStatuses {
		e.current.FileStatuses[k] = false
	}

	return nil
}

func (e *Engine) MarkAllReviewed() error {
	if !e.IsReviewTrackingEnabled() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	if err := e.database.MarkAllReviewed(e.current.ID); err != nil {
		return fmt.Errorf("mark all reviewed: %w", err)
	}

	for i := range e.current.ChangedFiles {
		e.current.ChangedFiles[i].Reviewed = true
	}
	for i := range e.current.AdditionalFiles {
		e.current.AdditionalFiles[i].Reviewed = true
	}
	for i := range e.current.ContentItems {
		e.current.ContentItems[i].Reviewed = true
	}
	for k := range e.current.FileStatuses {
		e.current.FileStatuses[k] = true
	}

	return nil
}

// -- Submission --

func (e *Engine) GetReviewSummary() (*types.ReviewSummary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.current == nil {
		return nil, fmt.Errorf("no active session")
	}

	summary := &types.ReviewSummary{
		Session:                e.current,
		FileComments:           make(map[string][]types.ReviewComment),
		ContentComments:        make(map[string][]types.ReviewComment),
		AdditionalFileComments: make(map[string][]types.ReviewComment),
	}

	for _, c := range e.current.Comments {
		if c.Resolved {
			continue
		}
		switch c.TargetType {
		case types.TargetFile:
			summary.FileComments[c.TargetRef] = append(summary.FileComments[c.TargetRef], c)
		case types.TargetContent:
			summary.ContentComments[c.TargetRef] = append(summary.ContentComments[c.TargetRef], c)
		case types.TargetAdditionalFile:
			summary.AdditionalFileComments[c.TargetRef] = append(summary.AdditionalFileComments[c.TargetRef], c)
		}
		switch c.Type {
		case types.CommentIssue:
			summary.IssueCt++
		case types.CommentSuggestion:
			summary.SuggestionCt++
		case types.CommentNote:
			summary.NoteCt++
		case types.CommentPraise:
			summary.PraiseCt++
		}
	}

	return summary, nil
}

func (e *Engine) Submit(action types.SubmitAction, body string) error {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()

	if session == nil {
		return fmt.Errorf("no active session")
	}

	// Validate: request_changes must include at least one unresolved comment or body text
	if action == types.ActionRequestChanges {
		hasBody := strings.TrimSpace(body) != ""
		hasUnresolved := false
		for _, c := range session.Comments {
			if !c.Resolved {
				hasUnresolved = true
				break
			}
		}
		if !hasBody && !hasUnresolved {
			return fmt.Errorf("request_changes requires at least one comment or review body")
		}
	}

	formatted := e.formatter.Format(session, session.Comments, action, body)

	// The daemon always queues feedback for pull delivery: the agent gets an
	// event notification as a hint and then retrieves the review via
	// get_feedback. Round advancement, marking the submission delivered, and
	// clearing comments happen at pull time in completeQueuedDelivery.
	e.feedback.Submit(formatted, false)

	// Save submission record
	now := time.Now()
	sub := &types.ReviewSubmission{
		ID:              uuid.New().String(),
		SessionID:       session.ID,
		Action:          types.SubmitAction(formatted.Action),
		FormattedReview: formatted.Formatted,
		CommentCount:    formatted.CommentCount,
		ReviewRound:     session.ReviewRound,
		SubmittedAt:     now,
	}
	_ = e.database.CreateSubmission(session.ID, sub)

	// Snapshot lifecycle: request_changes creates a snapshot, approve wipes all.
	// markReviewedOnSubmit and ResetAllReviewed acquire e.mu internally,
	// so snapshot operations that access e.current/e.reviewBase must
	// run under a separate lock scope.
	if e.IsReviewTrackingEnabled() {
		if action == types.ActionRequestChanges {
			e.markReviewedOnSubmit(session)
			e.mu.Lock()
			_ = e.createSnapshot(session, sub.ID)
			e.mu.Unlock()
			// Don't reset reviewed state — auto-unmark will handle it on the
			// next refresh, unmarking only files that actually changed.
		} else {
			// Approve: review is complete, wipe everything
			e.mu.Lock()
			e.deleteSnapshots()
			e.mu.Unlock()
			_ = e.ResetAllReviewed()
		}
	}

	// An agent-provided base ref is scoped to a single review: now that the
	// reviewer has submitted, revert to auto-advance so the base returns to HEAD.
	e.resetAgentBaseRefAfterReview()

	e.emit(EventFeedbackStatusChanged, EventPayload{
		Kind:   EventFeedbackStatusChanged,
		Status: e.feedback.GetStatus(),
	})

	e.emit(EventFeedbackSubmitted, EventPayload{
		Kind:    EventFeedbackSubmitted,
		Message: buildFeedbackSummary(formatted.Action, session.Comments),
		Status:  formatted.Action,
	})

	return nil
}

// buildFeedbackSummary creates a human-readable one-liner for channel notifications.
func buildFeedbackSummary(action string, comments []types.ReviewComment) string {
	issues, suggestions, notes, _ := countByType(comments)

	// Build counts portion (skip praise — not actionable)
	var parts []string
	if issues > 0 {
		if issues == 1 {
			parts = append(parts, "1 issue")
		} else {
			parts = append(parts, fmt.Sprintf("%d issues", issues))
		}
	}
	if suggestions > 0 {
		if suggestions == 1 {
			parts = append(parts, "1 suggestion")
		} else {
			parts = append(parts, fmt.Sprintf("%d suggestions", suggestions))
		}
	}
	if notes > 0 {
		if notes == 1 {
			parts = append(parts, "1 note")
		} else {
			parts = append(parts, fmt.Sprintf("%d notes", notes))
		}
	}
	counts := strings.Join(parts, ", ")

	if action == string(types.ActionRequestChanges) {
		if counts != "" {
			return fmt.Sprintf("Your reviewer requested changes (%s). Call get_feedback to retrieve the full review and address their comments.", counts)
		}
		return "Your reviewer requested changes. Call get_feedback to retrieve the full review and address their comments."
	}

	// Approved
	if counts != "" {
		return fmt.Sprintf("Your reviewer approved your changes with %s. Call get_feedback to read the review.", counts)
	}
	return "Your reviewer approved your changes. Call get_feedback to read the review."
}

func (e *Engine) FormatReview(action types.SubmitAction, body string) (string, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()

	if session == nil {
		return "", fmt.Errorf("no active session")
	}

	formatted := e.formatter.Format(session, session.Comments, action, body)
	return formatted.Formatted, nil
}

func (e *Engine) GetSubmissions() ([]types.ReviewSubmission, error) {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	return e.database.GetSubmissions(session.ID)
}

// -- Base ref management --

// SetBaseRef manually sets the diff baseline and disables auto-advance.
// The base is set to the parent of the given ref so the diff includes that
// commit's changes (the user selects a commit to review, not to exclude).
func (e *Engine) SetBaseRef(ref string) error {
	// Resolve to the parent so the selected commit's changes are included
	resolved, err := e.git.ResolveRef(ref + "~1")
	if err != nil {
		// Fall back to the ref itself if it has no parent (root commit)
		resolved, err = e.git.ResolveRef(ref)
		if err != nil {
			return fmt.Errorf("resolve ref %q: %w", ref, err)
		}
	}

	// Remember the user's actual selection for display purposes
	selected, err := e.git.ResolveRef(ref)
	if err != nil {
		selected = resolved
	}

	return e.applyBaseRef(resolved, selected, false)
}

// SetBaseRefExact sets the diff baseline to exactly the given ref and disables
// auto-advance. Unlike SetBaseRef it does NOT shift to the ref's parent, so the
// ref's own changes are excluded and the diff shows ref..worktree. Use it when
// the caller specifies the commit to diff *against* (e.g. an agent reviewing the
// commits it made since branching) rather than the earliest commit to include.
//
// The base is marked agent-set so it reverts to auto-advance (HEAD) once the
// reviewer submits — the agent provides a ref for one review, then state returns
// to normal working-tree review.
func (e *Engine) SetBaseRefExact(ref string) error {
	resolved, err := e.git.ResolveRef(ref)
	if err != nil {
		return fmt.Errorf("resolve ref %q: %w", ref, err)
	}
	return e.applyBaseRef(resolved, resolved, true)
}

// applyBaseRef stores the diff baseline and the user-facing selection, disables
// auto-advance, and clears any active snapshot view. diffBase is the commit the
// diff is computed against; selected is the ref shown to the user. agentSet marks
// the base as agent-provided so it is reverted after the next review.
func (e *Engine) applyBaseRef(diffBase, selected string, agentSet bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.current == nil {
		return fmt.Errorf("no active session")
	}

	e.current.BaseRef = diffBase
	e.current.UpdatedAt = time.Now()
	e.autoAdvanceRef = false
	e.agentBaseRef = agentSet
	e.selectedRef = selected
	e.current.AutoAdvanceRef = false
	e.current.SelectedRef = selected
	_ = e.database.UpdateSession(e.current)

	// Clear review base view but keep snapshots in DB — only approve wipes history
	e.reviewBase = nil

	return nil
}

// SetAutoAdvanceRef enables or disables auto-advancing baseRef to HEAD on each refresh.
func (e *Engine) SetAutoAdvanceRef(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.autoAdvanceRef = enabled
	if enabled {
		e.lastKnownHead = "" // Force HEAD re-detection on next refresh
		e.selectedRef = ""
		e.agentBaseRef = false
		// Clear review base view but keep snapshots in DB — only approve wipes history
		e.reviewBase = nil
	}
	// Persist so the choice survives a resume.
	if e.current != nil {
		e.current.AutoAdvanceRef = e.autoAdvanceRef
		e.current.SelectedRef = e.selectedRef
		_ = e.database.UpdateSession(e.current)
	}
}

// resetAgentBaseRefAfterReview reverts an agent-provided base ref back to
// auto-advance (HEAD) once a review concludes. It is a no-op when the base was
// set by the reviewer or auto-advance is already active. Callers must not hold
// e.mu (SetAutoAdvanceRef and emit acquire it).
func (e *Engine) resetAgentBaseRefAfterReview() {
	e.mu.RLock()
	agentSet := e.agentBaseRef
	e.mu.RUnlock()
	if !agentSet {
		return
	}
	e.SetAutoAdvanceRef(true)
	e.emit(EventFileChanged, EventPayload{Kind: EventFileChanged})
}

// SelectedBaseRef returns the commit the user selected in the ref picker.
// This differs from session.BaseRef which stores the parent for diffing.
// Returns empty string when in auto-advance mode.
func (e *Engine) SelectedBaseRef() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.selectedRef
}

// IsAutoAdvanceRef returns whether auto-advance is enabled.
func (e *Engine) IsAutoAdvanceRef() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.autoAdvanceRef
}

// RecentCommits returns recent commits for the ref picker.
func (e *Engine) RecentCommits(n int) ([]LogEntry, error) {
	return e.git.RecentCommits(n)
}

// -- Review snapshots --

// GetSnapshots returns all review snapshots for the current session.
func (e *Engine) GetSnapshots() ([]types.ReviewSnapshot, error) {
	if !e.IsReviewTrackingEnabled() {
		return nil, nil
	}
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}
	return e.database.GetSnapshots(session.ID)
}

// SetSnapshotBase activates snapshot-based diffing against the given snapshot.
func (e *Engine) SetSnapshotBase(snapshotID int) error {
	snap, err := e.database.GetSnapshot(snapshotID)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reviewBase = snap
	return nil
}

// ClearSnapshotBase deactivates review-base diffing and reverts to git diffs.
func (e *Engine) ClearSnapshotBase() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reviewBase = nil
}

// GetActiveSnapshot returns the current review base snapshot, or nil if using git diffs.
func (e *Engine) GetActiveSnapshot() *types.ReviewSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.reviewBase
}

// HasSnapshots returns true if any snapshots exist for the current session.
func (e *Engine) HasSnapshots() (bool, error) {
	if !e.IsReviewTrackingEnabled() {
		return false, nil
	}
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return false, fmt.Errorf("no active session")
	}
	return e.database.HasSnapshots(session.ID)
}

// createSnapshot captures the current state of all changed files into a snapshot.
func (e *Engine) createSnapshot(session *types.ReviewSession, submissionID string) error {
	if e.git == nil {
		return nil
	}

	headRef, err := e.git.CurrentRef()
	if err != nil {
		headRef = "unknown"
	}

	var snapshotFiles []types.SnapshotFile
	for _, f := range session.ChangedFiles {
		sf := types.SnapshotFile{
			Path:     f.Path,
			Status:   f.Status,
			Reviewed: f.Reviewed,
		}

		// Hash the working tree content into git's object store
		if f.Status != types.FileDeleted {
			sha, err := e.git.HashObject(f.Path)
			if err != nil {
				// Fallback: try reading content directly (non-git mode)
				content, readErr := e.git.FileContent("", f.Path)
				if readErr == nil {
					sf.Content = content
				}
			} else {
				sf.BlobSHA = sha
			}
		}

		snapshotFiles = append(snapshotFiles, sf)
	}

	_, err = e.database.CreateSnapshot(
		session.ID, submissionID, session.ReviewRound,
		headRef, session.BaseRef, snapshotFiles,
	)
	if err != nil {
		return err
	}

	// Snapshot saved to DB. The user can activate it via the ref picker.
	// reviewBase is NOT auto-set — Working Tree remains the default view.
	return nil
}

// latestSnapshotFromDB loads the most recent snapshot from the database.
// Returns (nil, nil) when no snapshots exist; (nil, err) on DB failure.
func (e *Engine) latestSnapshotFromDB(sessionID string) (*types.ReviewSnapshot, error) {
	snapshots, err := e.database.GetSnapshots(sessionID)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		return nil, nil
	}
	snap, err := e.database.GetSnapshot(snapshots[0].ID)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %d: %w", snapshots[0].ID, err)
	}
	return snap, nil
}

// deleteSnapshots removes all snapshots for the current session and clears the review base.
func (e *Engine) deleteSnapshots() {
	if e.current != nil {
		_ = e.database.DeleteSnapshots(e.current.ID)
	}
	e.reviewBase = nil
	e.autoUnmarked = nil
}

// markReviewedOnSubmit marks files as reviewed based on the config setting.
func (e *Engine) markReviewedOnSubmit(session *types.ReviewSession) {
	var mode string
	if cfg := e.cfg.Load(); cfg != nil {
		mode = cfg.MarkReviewedOnSubmit
	}
	if mode == "" {
		mode = "all"
	}

	switch mode {
	case "all":
		_ = e.MarkAllReviewed()
	case "commented":
		// Mark only files and artifacts that have unresolved comments
		commentedPaths := make(map[string]bool)
		commentedItems := make(map[string]bool)
		for _, c := range session.Comments {
			if c.Resolved {
				continue
			}
			switch c.TargetType {
			case types.TargetFile:
				commentedPaths[c.TargetRef] = true
			case types.TargetContent:
				commentedItems[c.TargetRef] = true
			}
		}
		for _, f := range session.ChangedFiles {
			if commentedPaths[f.Path] {
				_ = e.MarkReviewed(f.Path)
			}
		}
		for _, item := range session.ContentItems {
			if commentedItems[item.ID] {
				_ = e.MarkContentReviewed(item.ID)
			}
		}
	case "manual":
		// Do nothing — respect only explicit r toggles
	}
}

// filesRelativeToSnapshot filters the file list to only include files that
// differ from the snapshot. Files in git diff but unchanged from the snapshot
// are removed. Files reverted to match the git base (not in git diff) but
// different from the snapshot are added. The snapshot acts as the base ref.
func (e *Engine) filesRelativeToSnapshot(session *types.ReviewSession, files []types.ChangedFile, snapshot *types.ReviewSnapshot, priorReviewed map[string]bool) []types.ChangedFile {
	snapshotSHAs := make(map[string]string, len(snapshot.Files))
	for _, sf := range snapshot.Files {
		if sf.BlobSHA != "" {
			snapshotSHAs[sf.Path] = sf.BlobSHA
		}
	}

	seen := make(map[string]bool, len(files))
	var toHash []string
	for _, f := range files {
		seen[f.Path] = true
		if _, ok := snapshotSHAs[f.Path]; ok {
			toHash = append(toHash, f.Path)
		}
	}
	for _, sf := range snapshot.Files {
		if !seen[sf.Path] && sf.BlobSHA != "" {
			toHash = append(toHash, sf.Path)
		}
	}
	currentSHAs := e.hashPaths(toHash)

	var result []types.ChangedFile
	for _, f := range files {
		if snapSHA, inSnapshot := snapshotSHAs[f.Path]; inSnapshot {
			if currentSHA, ok := currentSHAs[f.Path]; ok && currentSHA == snapSHA {
				continue // unchanged from snapshot, hide it
			}
		}
		result = append(result, f)
	}

	// Reverted files: in snapshot but not in git diff, and content differs
	for _, sf := range snapshot.Files {
		if seen[sf.Path] {
			continue
		}
		if sf.BlobSHA != "" {
			if currentSHA, ok := currentSHAs[sf.Path]; ok && currentSHA == sf.BlobSHA {
				continue // content matches snapshot, skip
			}
		}
		// Preserve the reviewed flag captured before the refresh pruned this
		// reverted file's row. RefreshChangedFiles' transactional replace deletes
		// every changed_files row and only re-inserts files still in the git diff,
		// so by now this reverted file's row (and its reviewed bit) is gone. The
		// UpsertChangedFile below takes the INSERT path, which would otherwise
		// hard-code reviewed = false and silently un-review the file.
		merged := types.ChangedFile{
			Path:     sf.Path,
			Status:   types.FileModified,
			Reviewed: priorReviewed[sf.Path],
		}
		_ = e.database.UpsertChangedFile(session.ID, &merged)
		result = append(result, merged)
	}

	return result
}

// hashPaths batches git hash-object calls and returns a map[path]SHA. Paths that
// can't be hashed are omitted. Returns an empty map if the git client is nil.
func (e *Engine) hashPaths(paths []string) map[string]string {
	if e.git == nil || len(paths) == 0 {
		return map[string]string{}
	}
	shas, err := e.git.HashObjectsDry(paths)
	if err != nil {
		return map[string]string{}
	}
	return shas
}

// autoUnmarkChangedFiles compares current files against the latest snapshot and
// marks files as unreviewed if their content changed since the snapshot.
func (e *Engine) autoUnmarkChangedFiles(session *types.ReviewSession, files []types.ChangedFile, snapshot *types.ReviewSnapshot) {
	snapshotBySHA := make(map[string]string)
	for _, sf := range snapshot.Files {
		if sf.BlobSHA != "" {
			snapshotBySHA[sf.Path] = sf.BlobSHA
		}
	}

	var toHash []string
	for _, f := range files {
		if f.Status == types.FileDeleted {
			continue
		}
		if _, ok := snapshotBySHA[f.Path]; ok {
			toHash = append(toHash, f.Path)
		}
	}
	currentSHAs := e.hashPaths(toHash)

	for i := range files {
		f := &files[i]
		snapshotSHA, inSnapshot := snapshotBySHA[f.Path]

		if !inSnapshot {
			// New file since snapshot. It already defaults to reviewed=0 on
			// insert, so there's nothing to auto-unmark here. Force-unmarking
			// would clobber an explicit manual `r` re-mark — the bug (#99).
			continue
		}

		if f.Status == types.FileDeleted {
			// File deleted — mark unreviewed
			if f.Reviewed {
				f.Reviewed = false
				session.FileStatuses[f.Path] = false
				_ = e.database.MarkFileReviewed(session.ID, f.Path, false)
			}
			continue
		}

		currentSHA, ok := currentSHAs[f.Path]
		if !ok {
			continue // can't hash, skip
		}

		if currentSHA != snapshotSHA {
			// Content changed since snapshot — unmark reviewed, but only the
			// first time this file is seen changed against this snapshot. On
			// later refreshes we leave it alone so a manual `r` re-mark sticks
			// instead of reverting on the next 2s tick (#99).
			if e.autoUnmarked == nil {
				e.autoUnmarked = make(map[string]string)
			}
			if e.autoUnmarked[f.Path] == snapshotSHA {
				continue // already auto-unmarked against this snapshot
			}
			if f.Reviewed {
				f.Reviewed = false
				session.FileStatuses[f.Path] = false
				_ = e.database.MarkFileReviewed(session.ID, f.Path, false)
			}
			e.autoUnmarked[f.Path] = snapshotSHA
		}
	}
}

// snapshotFileDiff computes a diff between the snapshot's stored content and the current working tree.
func (e *Engine) snapshotFileDiff(snapshot *types.ReviewSnapshot, path string, full bool) (*types.DiffResult, error) {
	// Find the file in the snapshot
	var snapshotFile *types.SnapshotFile
	if snapshot.FilesByPath != nil {
		snapshotFile = snapshot.FilesByPath[path]
	}

	// Get current working tree content
	currentContent, currentErr := e.git.FileContent("", path)

	if snapshotFile == nil {
		// File is new since snapshot — show as all-added
		if currentErr != nil {
			return &types.DiffResult{Path: path}, nil
		}
		hunks := buildSyntheticDiff(currentContent)
		return &types.DiffResult{Path: path, Hunks: hunks}, nil
	}

	// Get snapshot content
	var oldContent string
	if snapshotFile.BlobSHA != "" {
		var err error
		oldContent, err = e.git.CatFile(snapshotFile.BlobSHA)
		if err != nil {
			return nil, fmt.Errorf("retrieve snapshot content for %s: %w", path, err)
		}
	} else {
		oldContent = snapshotFile.Content
	}

	if currentErr != nil {
		// File was deleted since snapshot — show as all-removed
		hunks := buildSyntheticDeleteDiff(oldContent)
		return &types.DiffResult{Path: path, Hunks: hunks}, nil
	}

	// Both exist — compute text diff
	if oldContent == currentContent {
		return &types.DiffResult{Path: path}, nil // no changes
	}

	ctxLines := 0
	if full {
		ctxLines = -1
	}
	hunks, err := TextDiffContext(oldContent, currentContent, ctxLines)
	if err != nil {
		return nil, fmt.Errorf("compute snapshot diff for %s: %w", path, err)
	}
	return &types.DiffResult{Path: path, Hunks: hunks}, nil
}

// -- Server --

// StartServer starts the Unix domain socket server at the given path.
func (e *Engine) StartServer(socketPath string) error {
	return e.server.Start(socketPath)
}

// -- Feedback (MCP channel) --

// PollFeedback returns pending feedback without blocking.
func (e *Engine) PollFeedback() *FormattedReview {
	return e.feedback.Poll()
}

// WaitForFeedback blocks until the user submits feedback (pause flow).
func (e *Engine) WaitForFeedback() *FormattedReview {
	return e.feedback.WaitForFeedback()
}

// GetReviewStatusInfo returns the current review status for CLI queries.
func (e *Engine) GetReviewStatusInfo() *ReviewStatusInfo {
	if e.feedback.IsPauseRequested() {
		return &ReviewStatusInfo{
			Status:  "pause_requested",
			Summary: "Your reviewer has requested a pause. Use the get_feedback tool with wait=true to receive feedback.",
		}
	}

	if e.feedback.HasPending() {
		e.mu.RLock()
		commentCount := 0
		if e.current != nil {
			commentCount = len(e.current.Comments)
		}
		e.mu.RUnlock()

		return &ReviewStatusInfo{
			Status:       "pending",
			CommentCount: commentCount,
			Summary:      fmt.Sprintf("%d comment(s) pending review.", commentCount),
		}
	}

	return &ReviewStatusInfo{
		Status:  "no_feedback",
		Summary: "No feedback pending.",
	}
}

// SubmitContentForReview adds or updates a content item (plan, doc) for review.
func (e *Engine) SubmitContentForReview(id, title, content, contentType string, isPlan bool) error {
	e.mu.Lock()
	if e.current == nil {
		e.mu.Unlock()
		return fmt.Errorf("no active session")
	}
	session := e.current

	now := time.Now()
	item := types.ContentItem{
		ID:          id,
		Title:       title,
		Content:     content,
		ContentType: contentType,
		IsPlan:      isPlan,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Upsert into session's content items. If the content text changed, clear
	// Reviewed so the user has to re-review the new version. Title-only or
	// metadata-only updates leave Reviewed alone.
	found := false
	for i := range session.ContentItems {
		if session.ContentItems[i].ID == id {
			contentChanged := session.ContentItems[i].Content != content
			session.ContentItems[i].Title = title
			session.ContentItems[i].Content = content
			session.ContentItems[i].ContentType = contentType
			session.ContentItems[i].IsPlan = isPlan
			session.ContentItems[i].UpdatedAt = now
			if contentChanged && session.ContentItems[i].Reviewed {
				session.ContentItems[i].Reviewed = false
				_ = e.database.MarkContentItemReviewed(session.ID, id, false)
			}
			item = session.ContentItems[i]
			found = true
			break
		}
	}
	if !found {
		session.ContentItems = append(session.ContentItems, item)
	}
	e.mu.Unlock()

	// Persist to DB (also creates a new version row and updates item.VersionCount)
	_ = e.database.UpsertContentItem(session.ID, &item)

	// Update in-memory version count
	e.mu.Lock()
	for i := range session.ContentItems {
		if session.ContentItems[i].ID == id {
			session.ContentItems[i].VersionCount = item.VersionCount
			break
		}
	}
	e.mu.Unlock()

	e.emit(EventContentItemAdded, EventPayload{
		Kind:   EventContentItemAdded,
		ItemID: id,
	})

	return nil
}

// RequestPause sets the pause flag so the agent sees "pause_requested" on next status check.
// The in-process implementation cannot fail; the error return exists for
// the socket-backed EngineClient where the round-trip can.
func (e *Engine) RequestPause() error {
	e.feedback.SetPauseRequested(true)

	e.emit(EventPauseChanged, EventPayload{
		Kind:   EventPauseChanged,
		Status: "pause_requested",
	})
	return nil
}

// CancelPause clears the pause flag.
func (e *Engine) CancelPause() error {
	e.feedback.SetPauseRequested(false)

	e.emit(EventPauseChanged, EventPayload{
		Kind:   EventPauseChanged,
		Status: "cancelled",
	})
	return nil
}

func (e *Engine) GetFeedbackStatus() string {
	return e.feedback.GetStatus()
}

func (e *Engine) GetQueuedCount() int {
	return e.feedback.QueuedCount()
}

// ReloadPendingFeedback checks the DB for undelivered submissions from the
// current session and reloads them into the in-memory FeedbackQueue.
// Called on session resume so queued feedback survives restarts.
func (e *Engine) ReloadPendingFeedback() {
	e.mu.RLock()
	session := e.current
	e.mu.RUnlock()
	if session == nil {
		return
	}

	subs, err := e.database.GetUndeliveredSubmissions(session.ID)
	if err != nil || len(subs) == 0 {
		return
	}

	for _, sub := range subs {
		review := &FormattedReview{
			Formatted:    sub.FormattedReview,
			CommentCount: sub.CommentCount,
			Action:       string(sub.Action),
		}
		e.feedback.Submit(review, false)
	}
}

func (e *Engine) GetSubscriberCount() int {
	return e.server.SubscriberCount()
}

func (e *Engine) GetSocketPath() string {
	return e.server.SocketPath()
}

// ServerVersion returns the engine's build version (the in-process value). The
// socket-backed EngineClient overrides this with the remote engine's version.
func (e *Engine) ServerVersion() string {
	return Version
}

// -- Events --

func (e *Engine) On(event EventKind, callback EventCallback) UnsubscribeFunc {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.subscribers[event] == nil {
		e.subscribers[event] = make(map[int]EventCallback)
	}
	id := e.nextSubID
	e.nextSubID++
	e.subscribers[event][id] = callback

	return func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		delete(e.subscribers[event], id)
	}
}

// emit notifies all subscribers for the given event. Must not be called with e.mu held.
func (e *Engine) emit(event EventKind, payload EventPayload) {
	e.mu.RLock()
	subs := make([]EventCallback, 0, len(e.subscribers[event]))
	for _, cb := range e.subscribers[event] {
		subs = append(subs, cb)
	}
	e.mu.RUnlock()

	for _, cb := range subs {
		cb(payload)
	}
}

// -- Lifecycle --

// Shutdown stops the socket server and cleans up resources.
// GetConfig returns a snapshot of the current configuration. Callers may
// hold the returned pointer indefinitely without racing against SaveConfig
// because the engine swaps in a fresh pointer rather than mutating in place.
func (e *Engine) GetConfig() *types.Config {
	return e.cfg.Load()
}

// IsReviewTrackingEnabled returns true when review state tracking is active.
func (e *Engine) IsReviewTrackingEnabled() bool {
	cfg := e.cfg.Load()
	return cfg != nil && cfg.ReviewTracking
}

// SaveConfig persists the current configuration to disk. Guards against a
// nil atomic.Pointer.Load (struct-literal construction paths or any code
// that defers Store) — json.MarshalIndent(nil) would otherwise write the
// literal string "null" to ~/.config/monocle/config.json and silently
// wipe the user's settings.
func (e *Engine) SaveConfig() error {
	cfg := e.cfg.Load()
	if cfg == nil {
		return errors.New("engine: SaveConfig called with no config loaded")
	}
	return SaveConfig(cfg)
}

func (e *Engine) Shutdown() {
	_ = e.server.Shutdown()
}

// SetIdleTimeout configures how long the underlying socket server stays
// alive past the 60s grace window after the last client disconnects. Zero
// or negative disables idle shutdown.
func (e *Engine) SetIdleTimeout(d time.Duration) {
	e.server.SetIdleTimeout(d)
}

// IdleShutdownCh returns a channel that closes when the idle timer fires.
// monocle serve selects on this alongside SIGINT/SIGTERM.
func (e *Engine) IdleShutdownCh() <-chan struct{} {
	return e.server.IdleShutdownCh()
}

// -- Socket message handlers (called by SocketServer) --

// handleGetReviewStatus returns the current review state.
func (e *Engine) handleGetReviewStatus(_ *protocol.GetReviewStatusMsg) *protocol.GetReviewStatusResponse {
	info := e.GetReviewStatusInfo()
	return &protocol.GetReviewStatusResponse{
		Type:         protocol.TypeGetReviewStatusResponse,
		Status:       info.Status,
		CommentCount: info.CommentCount,
		Summary:      info.Summary,
	}
}

// boundedWaitCancel returns a cancel channel for WaitForFeedbackCancellable
// that fires when the original cancel fires OR after maxWaitMs milliseconds.
//
// The bound exists so a hook never blocks indefinitely when the engine
// outlives an active reviewer (autospawn keeps the socket alive for a grace
// period + idle timeout, during which no human is attached). When the timer
// fires, WaitForFeedbackCancellable returns nil without consuming the queue —
// the same shape as a client disconnect — which callers already treat as
// "no feedback, proceed".
//
// maxWaitMs <= 0 means unbounded: the original cancel is returned unchanged.
// The bound is also skipped while a pause was explicitly requested, so a
// reviewer who pressed P keeps the historical block-until-submit behaviour.
// A non-nil stop function must be called (e.g. via defer) to release the
// timer goroutine.
func (e *Engine) boundedWaitCancel(cancel <-chan struct{}, maxWaitMs int) (<-chan struct{}, func()) {
	if maxWaitMs <= 0 || e.feedback.IsPauseRequested() {
		return cancel, func() {}
	}

	bounded := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		timer := time.NewTimer(time.Duration(maxWaitMs) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			// Re-check the pause flag at fire time to close a TOCTOU race: a
			// reviewer may have pressed P after the initial snapshot above but
			// before the bound elapsed. Honoring the late pause means NOT
			// closing `bounded` — instead we keep waiting only on the original
			// cancel/stop so WaitForFeedbackCancellable blocks until the
			// reviewer submits (or genuinely disconnects), preserving the
			// pause flow's block-until-submit guarantee.
			if e.feedback.IsPauseRequested() {
				select {
				case <-cancel:
				case <-stop:
				}
				return
			}
		case <-cancel:
		case <-stop:
		}
		close(bounded)
	}()
	return bounded, func() { close(stop) }
}

// handlePollFeedback returns pending feedback, optionally blocking until available.
// In push (channel) mode, round advancement happens in Submit().
// In queue mode, round advancement happens here when feedback is picked up.
//
// cancel, when non-nil, aborts a Wait if the requesting client disconnects;
// the queue is left intact so the feedback isn't lost to a dead socket.
func (e *Engine) handlePollFeedback(msg *protocol.PollFeedbackMsg, cancel <-chan struct{}) *protocol.PollFeedbackResponse {
	var result *PollResult

	if msg.Wait {
		e.emit(EventWaitStatusChanged, EventPayload{
			Kind:   EventWaitStatusChanged,
			Status: "waiting",
		})
		waitCancel, stop := e.boundedWaitCancel(cancel, msg.MaxWaitMs)
		defer stop()
		result = e.feedback.WaitForFeedbackCancellable(waitCancel)
		e.emit(EventWaitStatusChanged, EventPayload{
			Kind:   EventWaitStatusChanged,
			Status: "",
		})
	} else {
		result = e.feedback.PollWithInfo()
	}

	if result == nil || len(result.Reviews) == 0 {
		return &protocol.PollFeedbackResponse{
			Type:        protocol.TypePollFeedbackResponse,
			HasFeedback: false,
		}
	}

	// If this feedback was NOT already channel-delivered, perform queue delivery
	// side effects: advance round, mark delivered, clear comments, emit events.
	if !result.ChannelDelivered {
		e.completeQueuedDelivery()
	}

	feedback, commentCount, action := result.CombinedFeedback()

	return &protocol.PollFeedbackResponse{
		Type:         protocol.TypePollFeedbackResponse,
		HasFeedback:  true,
		Feedback:     feedback,
		CommentCount: commentCount,
		Action:       action,
	}
}

// handleMarkActivity marks the current session as having unreviewed changes.
// Called from the PostToolUse hook when Claude fires a write-tool. Idempotent
// — repeated calls in the same turn leave the flag set. Cleared when the
// reviewer's feedback queue is next drained.
func (e *Engine) handleMarkActivity(_ *protocol.MarkActivityMsg) *protocol.MarkActivityResponse {
	e.mu.Lock()
	e.hasUnreviewedActivity = true
	e.mu.Unlock()
	// Pulse the TUI so it can show that the agent is actively making changes. The
	// flag (and pulse) clears when the reviewer's feedback is next delivered,
	// which fires EventFeedbackPickedUp.
	e.emit(EventActivityChanged, EventPayload{Kind: EventActivityChanged, Status: "active"})
	return &protocol.MarkActivityResponse{
		Type:    protocol.TypeMarkActivityResponse,
		Success: true,
	}
}

// handleAwaitReview is invoked by the Stop hook when Claude finishes a turn.
// Order of checks:
//  1. Drain any already-queued feedback first — if the reviewer submitted
//     while Claude was still working, return it immediately.
//  2. If no queued feedback AND !hasUnreviewedActivity, the turn had no
//     reviewable writes (pure chat). Return HasActivity=false; the Stop
//     hook exits 0 and the turn ends normally.
//  3. Dirty with no queued feedback: block until the reviewer submits
//     (if Wait=true), then return their verdict.
func (e *Engine) handleAwaitReview(msg *protocol.AwaitReviewMsg, cancel <-chan struct{}) *protocol.AwaitReviewResponse {
	// Step 1: drain any already-queued feedback.
	if result := e.feedback.PollWithInfo(); result != nil && len(result.Reviews) > 0 {
		if !result.ChannelDelivered {
			e.completeQueuedDelivery()
		}
		feedback, _, action := result.CombinedFeedback()
		return &protocol.AwaitReviewResponse{
			Type:        protocol.TypeAwaitReviewResponse,
			HasActivity: true,
			Action:      action,
			Feedback:    feedback,
		}
	}

	// Step 2: clean session, nothing to gate.
	e.mu.RLock()
	dirty := e.hasUnreviewedActivity
	e.mu.RUnlock()
	if !dirty {
		return &protocol.AwaitReviewResponse{
			Type:        protocol.TypeAwaitReviewResponse,
			HasActivity: false,
		}
	}

	// Step 3: dirty, no feedback queued. Block on the reviewer.
	if !msg.Wait {
		return &protocol.AwaitReviewResponse{
			Type:        protocol.TypeAwaitReviewResponse,
			HasActivity: true,
		}
	}
	e.emit(EventWaitStatusChanged, EventPayload{
		Kind:   EventWaitStatusChanged,
		Status: "waiting",
	})
	waitCancel, stop := e.boundedWaitCancel(cancel, msg.MaxWaitMs)
	defer stop()
	result := e.feedback.WaitForFeedbackCancellable(waitCancel)
	e.emit(EventWaitStatusChanged, EventPayload{
		Kind:   EventWaitStatusChanged,
		Status: "",
	})
	// result is nil when the client disconnected mid-wait OR the max-wait
	// bound elapsed; the queue is
	// untouched, so report activity-without-verdict and leave the feedback
	// for the next poll rather than consuming it into a dead connection.
	if result == nil || len(result.Reviews) == 0 {
		return &protocol.AwaitReviewResponse{
			Type:        protocol.TypeAwaitReviewResponse,
			HasActivity: true,
		}
	}
	if !result.ChannelDelivered {
		e.completeQueuedDelivery()
	}
	feedback, _, action := result.CombinedFeedback()
	return &protocol.AwaitReviewResponse{
		Type:        protocol.TypeAwaitReviewResponse,
		HasActivity: true,
		Action:      action,
		Feedback:    feedback,
	}
}

// completeQueuedDelivery performs the side effects of delivering queued feedback:
// advancing the round, marking DB submissions as delivered, clearing comments,
// and emitting events so the TUI can update.
func (e *Engine) completeQueuedDelivery() {
	e.mu.Lock()
	session := e.current
	// The reviewer has responded; any write-tool activity that preceded this
	// submission has now been reviewed. Clearing here centralizes the flag
	// lifecycle: any poller that drains the queue (AwaitReview, PollFeedback,
	// agent's get_feedback MCP call, etc.) takes the same code path.
	e.hasUnreviewedActivity = false
	if session != nil {
		_ = e.sessions.AdvanceRound(session)
	}
	e.mu.Unlock()

	if session != nil {
		_ = e.database.MarkSubmissionsDelivered(session.ID)
	}

	_ = e.ClearComments()

	// Annotations are pinned to a round's exact code ranges; once the reviewer has
	// consumed this round's feedback the agent will revise the code, so the ranges
	// (and their highlights) no longer line up. Wipe them so each round starts with
	// a clean slate and the agent re-annotates against the new code.
	if session != nil {
		if err := e.database.DeleteAnnotations(session.ID); err == nil {
			e.mu.Lock()
			if e.current != nil && e.current.ID == session.ID {
				e.current.Annotations = nil
			}
			e.mu.Unlock()
		}
	}

	e.feedback.ClearStatus()

	e.emit(EventFeedbackPickedUp, EventPayload{
		Kind: EventFeedbackPickedUp,
	})
	e.emit(EventFeedbackStatusChanged, EventPayload{
		Kind:   EventFeedbackStatusChanged,
		Status: "none",
	})
	e.emit(EventFileChanged, EventPayload{
		Kind: EventFileChanged,
	})
}

// handleSubmitContent receives reviewable content (plans, docs) from the
// agent. When the caller supplies an empty ID we mint a UUID server-side
// and echo it back via the response so the caller can address the
// just-submitted item (mark reviewed, dismiss, fetch versions); without
// this, the round-trip stores under an id the caller can never see.
func (e *Engine) handleSubmitContent(msg *protocol.SubmitContentMsg) *protocol.SubmitContentResponse {
	id := msg.ID
	if id == "" {
		id = uuid.New().String()
	}

	err := e.SubmitContentForReview(id, msg.Title, msg.Content, msg.ContentType, msg.IsPlan)
	if err != nil {
		return &protocol.SubmitContentResponse{
			Type:    protocol.TypeSubmitContentResponse,
			Success: false,
			Message: err.Error(),
		}
	}

	return &protocol.SubmitContentResponse{
		Type:    protocol.TypeSubmitContentResponse,
		Success: true,
		Message: fmt.Sprintf("Content submitted for review: %s", msg.Title),
		ID:      id,
	}
}
