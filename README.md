# o_(◉) monocle

**Review your AI agent's code as it writes it.** Leave comments on diffs, submit structured feedback, and watch the agent fix things in real time — all from your terminal.

![IMG_0689](https://github.com/user-attachments/assets/92a877a8-ac89-4561-bd05-9bb916820943)


[More screenshots →](screenshots/)

Monocle is a TUI that runs alongside your AI coding agent. You review diffs in real time as the agent writes code, leave line-level comments — issues, suggestions, notes — and submit a structured review in one batch. The agent receives your feedback and starts fixing things immediately, just like a PR review but live.

Monocle connects to your agent via MCP tools or CLI commands over a Unix socket. With [Claude Code](https://claude.com/claude-code) and [MCP channels](https://code.claude.com/docs/en/channels-reference), it pushes feedback directly into the agent's context the moment you submit. Other agents — [OpenCode](https://opencode.ai), [Codex CLI](https://github.com/openai/codex), [Gemini CLI](https://github.com/google/gemini-cli), and [Pi](https://pi.dev) — retrieve feedback on demand. MCP channels just make the process smoother.

## Why

Without something like Monocle, reviewing agent-written code means rubber-stamping diffs you didn't read, copy-pasting feedback into a chat window, or just hoping the agent got it right. There's no way to say "fix these three issues and show me again."

Monocle gives you a proper review loop without slowing the agent down. It doesn't gate each file change behind an approval — your agent keeps working while you review at your own pace. When you're ready, leave line-level comments and submit. The agent receives your feedback immediately and starts addressing it. You see the updated diffs, review again, and iterate — like PR reviews, but in real time.

## Requirements

- A coding agent: [Claude Code](https://claude.com/claude-code), [OpenCode](https://opencode.ai), [Codex CLI](https://github.com/openai/codex), [Gemini CLI](https://github.com/google/gemini-cli), [Pi](https://pi.dev), or any MCP-compatible agent
- A terminal with 256-color or true color support
- A [Nerd Font](https://www.nerdfonts.com/) for file icons (optional but recommended)

## Installation

### Homebrew (macOS/Linux)

```bash
brew install --cask josephschmitt/tap/monocle
```

> **Upgrading from a previous (formula) install?** monocle is now distributed
> as a Homebrew **cask** rather than a formula. If you previously ran
> `brew install josephschmitt/tap/monocle`, uninstall the old formula first,
> then reinstall as a cask:
>
> ```bash
> brew uninstall monocle        # remove the old formula
> brew install --cask josephschmitt/tap/monocle
> ```
>
> See [Why a cask?](docs/installation.mdx) for the rationale.

<details>
<summary>Other installation methods</summary>

#### Pre-built Binaries

Download from [GitHub Releases](https://github.com/josephschmitt/monocle/releases):

**macOS:**
```bash
# Apple Silicon
# x-release-please-start-version
curl -Lo monocle.tar.gz https://github.com/josephschmitt/monocle/releases/download/v0.48.1/monocle_darwin_arm64.tar.gz
# x-release-please-end
tar xzf monocle.tar.gz
sudo mv monocle /usr/local/bin/

# Intel
# x-release-please-start-version
curl -Lo monocle.tar.gz https://github.com/josephschmitt/monocle/releases/download/v0.48.1/monocle_darwin_amd64.tar.gz
# x-release-please-end
tar xzf monocle.tar.gz
sudo mv monocle /usr/local/bin/
```

**Linux:**
```bash
# x86_64
# x-release-please-start-version
curl -Lo monocle.tar.gz https://github.com/josephschmitt/monocle/releases/download/v0.48.1/monocle_linux_amd64.tar.gz
# x-release-please-end
tar xzf monocle.tar.gz
sudo mv monocle /usr/local/bin/

# ARM64
# x-release-please-start-version
curl -Lo monocle.tar.gz https://github.com/josephschmitt/monocle/releases/download/v0.48.1/monocle_linux_arm64.tar.gz
# x-release-please-end
tar xzf monocle.tar.gz
sudo mv monocle /usr/local/bin/
```

#### From Source

```bash
git clone https://github.com/josephschmitt/monocle.git
cd monocle
devbox run -- make build
# Binaries are in bin/
```

</details>

## Quick Start

### 1. Register Monocle with your agent

```bash
monocle register          # interactive picker
monocle register claude   # or: opencode, codex, gemini, pi, all
```

This configures MCP tools or skills depending on the agent. Claude Code gets an MCP server and slash commands. Pi uses existing project or global `pi-mcp-adapter` setups automatically, otherwise it falls back to skills plus prompt templates; pass `--integration-mode mcp` to add the pinned adapter explicitly. Other agents get skill files by default. Use `--global` to write to the user-level config directory instead of the project. If Pi is already running, restart it or run `/reload` after registering.

#### Other agents

If your agent isn't natively supported, you can set up Monocle manually:

- **MCP tools**: If your agent supports MCP servers via stdio, point it at `monocle serve-mcp`. This exposes review tools (`review_status`, `get_feedback`, `send_artifact`, `add_files`, `remove_files`, `set_file_groups`, `add_annotations`, `set_review_name`, `set_base_ref`, `set_repo`, `send_diff`) over stdio.
- **Skills**: Download `skills.tar.gz` from the [latest release](https://github.com/josephschmitt/monocle/releases/latest) and extract the skill files into wherever your agent expects its skills.

### 2. Start reviewing

Start your agent and Monocle in separate terminals:

```bash
monocle
```

For Claude Code, Monocle registers an MCP server that exposes review tools directly — no bash permissions or skills needed. For Pi, Monocle uses `pi-mcp-adapter` only when it is already configured, or when you explicitly request MCP mode; otherwise Pi uses prompts and skills that run CLI commands. Other agents get [skills](#agent-operations) that instruct them to run CLI commands.

#### Push notifications (Claude Code only)

Claude Code supports [MCP channels](https://code.claude.com/docs/en/channels-reference), which deliver feedback automatically. When you submit a review, a push notification prompts the agent to retrieve your feedback immediately instead of waiting for the next poll.

> **Tip:** If you start or restart Monocle while Claude Code is already running, the MCP server may need to reconnect. Type `/mcp` in Claude Code and select Monocle to reconnect.

### The review loop

Navigate with `j`/`k`, add comments with `c`, and use `v` for visual (multi-line) selections. Press `H` to see all keybindings, or see the full [Keybindings](#keybindings) reference.

**Submit** (`S`): Your review is formatted and queued for delivery. With Claude Code channels, a push notification prompts the agent to retrieve it immediately. With other agents, the review waits in the queue until the agent runs `/get-feedback` or calls `monocle review get-feedback`. Multiple reviews can accumulate in the queue — the agent receives them all combined when it pulls. If there are no comments, the review is treated as an approval. Toggle the "Copy to clipboard" checkbox with `Shift+Tab` in the submit modal to also copy the formatted review when submitting.

**External editor** (`Ctrl+g`): In the comment or submit modal, opens the current text in your `$VISUAL` or `$EDITOR` (falls back to `vi`). Edit in your preferred editor, save and quit, and the text is brought back into the modal.

**Yank** (`Ctrl+y`): In the submit modal, copies the formatted review to your system clipboard without submitting, then closes the modal.

**Pause** (`P`): The agent receives a push notification to stop and wait. It runs `monocle review get-feedback --wait` and blocks until you submit your review. This is for when you want to review before the agent moves on. Pause requires MCP channel support (currently Claude Code only).

### Plan review and focus mode

Monocle isn't limited to reviewing file changes. Your agent can submit **plans, architecture decisions, summaries, and other content** directly to Monocle for review using `monocle review send-artifact`. These show up alongside your file diffs in the sidebar, and you can leave line-level comments on them the same way. You can also trigger this yourself with `/review-plan` or `/review-plan-wait` — useful when you want to send the agent's plan to Monocle without waiting for the agent to do it on its own.

This means you can review the agent's *thinking* before it writes code — not just the output. Ask the agent to submit its content first, review it, leave feedback, and only then let it proceed.

`/review-plan-wait` submits content to your TUI **and blocks** until you respond with feedback. If you approve, the agent continues. If you request changes, the agent updates and submits again — iterating across as many rounds as it takes until you're satisfied.

> **Note:** Monocle's operations are available to your agent but the agent decides when to use them on its own. If you want the agent to automatically submit plans for review, add instructions to your agent's project configuration. See [Automatic content review](#automatic-content-review) below for a suggested prompt.

## Features

- **Works with any coding agent** — Claude Code, OpenCode, Codex CLI, Gemini CLI, Pi, or any MCP-compatible agent
- **Push notifications** — With Claude Code channels, feedback is pushed directly into the agent's context the moment you submit
- **Pull-based feedback** — Agents without channel support retrieve feedback via `/get-feedback` or `monocle review get-feedback`; multiple reviews queue up and are delivered together
- **Plan & architecture review** — Your agent can submit plans, architecture decisions, and other content for review with markdown rendering. When iterating, Monocle shows diffs between plan versions so you can see exactly what changed. Use focus mode (`F`) for distraction-free reading
- **Guided tours** — Your agent can walk you through a change as numbered stops: `.`/`,` step, the diff lands on each stop with the agent's note beside it, and you ask about any stop by its id
- **Review gating** — `/review-plan-wait` blocks the agent until you approve the submitted content before it proceeds
- **Pause flow** — Ask your agent to stop and wait while you review, then release it when ready (requires MCP channel support)
- **Live diff viewer** — Unified and split (side-by-side) views with syntax highlighting and intra-line diffs
- **Structured comments** — Tag feedback as issues, suggestions, notes, or praise with line-level or file-level precision
- **Suggested edits** — Press `s` to propose exact code changes with GitHub-style `suggestion` blocks
- **Visual selection** — Select line ranges for comments with vim-style visual mode
- **Markdown rendering** — Plans and changed `.md` files render with styled headings, bold, italic, lists, and code blocks
- **Horizontal scrolling & line wrapping** — Navigate wide diffs with `h`/`l` or toggle wrapping with `w`
- **Diff search** — Search within a diff with `/` (forward) or `?` (backward) when the diff pane is focused; jump between matches with `n`/`N` (matches are highlighted)
- **Responsive layout** — Automatically stacks panes vertically in narrow terminals
- **Ref picker** — Change the base ref on the fly to compare against any branch or commit
- **Version history** — Browse all versions of a plan or artifact and diff any version against the latest
- **Comment resolution** — Mark individual comments as resolved (`x`); resolved comments are excluded from submitted reviews
- **Submission history** — View past review submissions with `:history`
- **Themes** — Choose a color scheme (`dark`, `light`, `molokai`, `dracula`, `nord`) via the `theme` config option, or switch live with `:theme <name>` / cycle with `:theme`
- **Mouse support** — Click to focus panes, scroll with the wheel, click files to select, drag to make visual selections, interact with modal controls, click a tour stop's labels, and scroll a long tour note
- **External editor** — Open comment or submit text in `$VISUAL`/`$EDITOR` with `Ctrl+g` for full editing power
- **Configurable keybindings** — Override any navigation or action key via config
- **Feedback queue** — Submit reviews while the agent is working; delivered when the agent next runs `/get-feedback`
- **Connection indicator** — See at a glance whether your agent is connected, with manual socket override for troubleshooting
- **Review tracking** — Mark files as reviewed with `r` (auto-advances to next), filter sidebar with `/`. When you submit feedback, monocle snapshots file state so it can automatically detect what changed on the next round — filter to unreviewed to see only what's new
- **Session persistence** — Reviews survive restarts via SQLite

## Agent Operations

Monocle exposes review operations via **MCP tools** (default for Claude Code, and for Pi when `pi-mcp-adapter` is already configured) or **skills** (default for other agents, and Pi's fallback). Both are configured automatically by `monocle register`.

| Operation | MCP tool | Skill | Description |
|-----------|----------|-------|-------------|
| Get feedback | `get_feedback` | `/get-feedback` | Retrieve pending review feedback |
| Send artifact | `send_artifact` | `/review-plan` | Submit content (plans, decisions, summaries) for review |
| Send artifact (blocking) | `send_artifact`, then `get_feedback` with `wait: true` | `/review-plan-wait` | Submit content and iterate on feedback until approved |
| Check status | `review_status` | — | Check if feedback is pending or a pause was requested |
| Add files | `add_files` | — | Add files to the current review session |
| Remove files | `remove_files` | — | Remove previously-added files from the review session |
| Group files | `set_file_groups` | `monocle review group-files` | Categorize / group / order changed files for the grouped sidebar view |
| Annotate code | `add_annotations` | `monocle review annotate` | Attach agent rationale + doc links to code ranges (shown to reviewer, not feedback) |
| Review committed work | `set_base_ref` | — | Diff against a base commit so already-committed changes are reviewed (reverts to `HEAD` after the review) |
| Point at a repo/worktree | `set_repo` | `-C`/`--workdir` flag | Bind to the engine for a specific repo — call once after entering a git worktree so review tools target it, not the launch directory |
| Guided tour | `set_walkthrough` | `monocle review set-walkthrough` | Walk the reviewer through the change as ordered stops (file + lines, a note, related files, views) they step through with `.` / `,` |
| Show a tour stop | `goto_stop` | `monocle review goto-stop` | Move the reviewer to a stop by its id — answer "1.2 — why?" by showing 1.2 |
| Open a file in the editor | `open_editor` | `monocle review open-editor` | Open any file of the repo at a line in the editor beside Monocle (the related-files pane), giving it the keyboard; `full` zooms it to fill the window. The diff stays where it is |
| Highlight a range | `highlight_range` | `monocle review highlight` | Tint one range of lines in a file of the review so its start and end read at a glance; replaces any earlier one, `clear` removes it. Moves nothing — pair it with `goto_line` |
| Show a file at a line | `goto_line` | `monocle review goto-line` | Open a file of the review (changed or added) with the diff cursor on a new-file line — `top` places it that many rows below the top of the diff, for as much as possible of what follows; ctrl+o returns, and the tour's stop does not change |
| Show a before/after comparison | `send_diff` | — | Render an agent-supplied contrast (pseudocode before/after, competing design options) as a side-by-side diff — reads no files and runs no git |

## Keybindings

Bindings are grouped by the task they serve, and within a group ordered by key: lowercase, uppercase, digits, punctuation, modified keys, then `:` commands. Press `H` in monocle for this same list with your own overrides applied, and `/` inside it to search. The arrow keys mirror `h`/`j`/`k`/`l` throughout.

#### Move

| Key | Action |
|-----|--------|
| `g` / `G` | Jump to top/bottom |
| `h` / `l` | Scroll diff left/right |
| `j` / `k` | Move up/down (arrow keys too) |
| `J` / `K` | Scroll diff up/down (any pane) |
| `L` | Scroll diff right (any pane) |
| `0` | Scroll to column 0 (any pane) |
| `$` | Scroll to line end (any pane) |
| `^` | Scroll to first non-space (any pane) |
| `ctrl+d` / `ctrl+u` | Scroll diff half page (any pane) |

#### Jump

| Key | Action |
|-----|--------|
| `n` / `N` | Next/previous search match |
| `%` | Between the current block's start and end (vim %) |
| `(` | Out one level, to the enclosing block's opening line (vim [{) |
| `)` | Out to the outermost enclosing block (vim 99[{) |
| `/` / `?` | Search the diff forward/backward (diff focused) |
| `<` / `>` | Previous/next comment or annotation |
| `[` / `]` | Previous/next diff chunk in the diff pane, else previous/next file |
| `{` / `}` | Previous/next file (any pane) |
| `ctrl+i` | Forward again (vim ctrl+i) |
| `ctrl+o` | Back to where you jumped from (vim ctrl+o) |

#### Guided tour

| Key | Action |
|-----|--------|
| `W` | Tour mode on/off (hides the note and marks, keeps your stop) |
| `X` | Close the pane holding a stop's related files |
| `,` | Previous stop |
| `.` | Next stop of the agent's tour (resumes it when off) |
| `backspace` / `f18` | Back to the stop you came from, as a browser goes back — through every stop entered, however you got there. Tour mode only |
| `f19` | Forward again, until you enter another stop |
| `f17` | Reset the layout, as `:layout reset` does: monocle goes back to the stop's first line, the stop's related files come back, fresh, then `walkthrough_layout_reset` runs. For a key remapper to send |
| `:back` / `:forward` | The same as `backspace` / `f19` |
| `:stop <id>` | Jump to a stop by id, e.g. `:stop 1.2`; `:stop 5` goes to chapter 5's first stop, `:stop` alone to the tour's first |
| `:view [n]` | Open the stop's nth view (default the first): through `walkthrough_on_stop` when set, else in the media / markdown viewer. Clicking a view's label under the note does the same |
| `:related [n]` | Bring up the related-files pane — every related file of the stop, as on arriving — with file n (default the first) the active window, unzooming monocle's tmux window if it is zoomed. Clicking a related file's label does the same |
| `:call [n]` | Go to the stop the stop's nth call (default the first) leads to. Clicking a call's label under the note does the same |
| `:layout [reset]` | Say whether the tour's windows are in a saved layout; `reset` respawns the related-files pane with the stop's own related files, then runs `walkthrough_layout_reset` to put back the default. Clicking `reset` in the note's `Layout: saved · reset` does the same |

When the agent sends a tour (`set_walkthrough`), monocle enters tour mode on its first stop: the diff cursor lands on the stop's lines, which stay marked in the gutter (magenta; the previous stop blue, the next green, the one after bright green), the doc pane shows `1.2 · Title` (marked `new` on a stop you have not been on before, `✓` on one you have, kept across restarts) and the agent's note, and the status bar shows `tour 1.2 · 3 of 7` (the stop id, then its position in the tour). Ask the agent about a stop by its id. The ends of the tour clamp rather than wrap.

#### View & panes

| Key | Action |
|-----|--------|
| `a` | Toggle full-file diff (whole file vs. changed lines) |
| `f` | Cycle sidebar view (flat/tree/grouped) |
| `t` | Cycle diff style (unified/split/file) (any pane) |
| `w` | Toggle line wrapping (any pane) |
| `z` / `e` | Collapse/expand all (tree view) |
| `F` | Toggle focus mode (hide sidebar, wrap lines) |
| `O` | Hide/show inline comments + annotations |
| `T` | Cycle layout (auto/side-by-side/stacked) |
| `1` / `2` | Jump straight to a pane |
| `m` | Cycle source-code comments: dim → hide → show |
| `M` | The same cycle in reverse |
| `/` | Sidebar: cycle the reviewed filter |
| `;` | Show/hide the sidebar |
| `=` | Size the doc pane against the diff: the focused pane biggest, then smallest, then back |
| `enter` | Focus the diff pane / toggle a directory open |
| `ctrl+h` / `ctrl+j` / `ctrl+k` / `ctrl+l` | Move pane focus left/down/up/right; at the edge, moves the tmux pane |
| `tab` / `shift+tab` | Switch pane focus (sidebar/diff/doc) |

#### Comment

| Key | Action |
|-----|--------|
| `c` | Add a comment at the cursor |
| `ctrl+x` | Delete a comment (on a comment) |
| `s` | Suggest an edit at the cursor |
| `v` | Visual select mode (multi-line comments) |
| `x` | Toggle a comment resolved (on a comment) |
| `y` | Yank the line / selection to the clipboard |
| `A` | Answer at the cursor (replies to the agent; asks nothing back) |
| `C` | Add a file-level comment |
| `Q` | Ask a question at the cursor (a comment that wants an answer) |
| `+` | Tag the line, or every line of the selection, to send to the agent; on lines all tagged, untag them. Tagged lines are marked yellow in the gutter and stay tagged across files and stops until sent |
| `@` | Send the selection and the tagged lines — else the tags, else the cursor's line — to the agent through `walkthrough_ask`, then clear them |
| `u` / `U` | In a tour, move to the next / previous underlined symbol in the stop: what one of its related files or calls is about |
| `o` / `ctrl+]` | On an underlined symbol in a tour, open what it is about: its related file in the related-files pane, with the keyboard, or the stop its call leads to. Elsewhere, open what the lines `@` would send reference — the SQL file a query loads, the file a function lives in — in the related-files pane, through `walkthrough_resolve`, beside what it already holds (at most 8 files), and move focus there |
| `p` / `f16` | Preview the first of them instead, in a passing preview in the related-files pane's editor (`related_editor_preview`), which takes the keyboard; nothing is added to the pane |
| `space` | Expand/collapse a comment under the cursor |
| `E` | Expand/collapse **all** comments in the open file (any view) |

#### Review

| Key | Action |
|-----|--------|
| `b` | Change the base ref |
| `r` | Toggle the file reviewed (advances to the next unreviewed) |
| `x` | Dismiss an artifact / remove an added file (in the sidebar) |
| `B` / `:base-artifact-version` | Base artifact version to diff against |
| `D` / `:clear` | Clear the review (comments, plans, added files, reviewed) |
| `P` / `:pause` | Toggle pause (ask the agent to wait) |
| `R` | Force reload files |
| `Ctrl+R` / `:relaunch` | Restart onto a newly installed build (only when one is) |
| `S` / `:submit` | Submit the review |
| `ctrl+y` | Copy the review to the clipboard without submitting |

#### Open elsewhere

| Key | Action |
|-----|--------|
| `d` | Open/cycle an annotation's doc links in the doc pane. In tour mode, off an annotation, it shows and hides the stop's note |
| `!` | Run a shell command on the current file |
| `ctrl+g` | Open in your editor — the path on this line, else the file under review; in a tour, in the related-files pane |
| `ctrl+shift+g` | Same, always taking over the screen |
| `ctrl+p` | Open the artifact/file in an external viewer (markdown or media) |
| `ctrl+t` | Open a terminal at the current file's directory |
| `ctrl+shift+t` | Open a terminal, always taking over the screen |

#### Commands

| Key | Action |
|-----|--------|
| `:` | Enter command mode (Tab completes and cycles names) |
| `:base-ref` | Base ref to diff against (same as b) |
| `:cancel-feedback` | Cancel submitted feedback still queued for the agent |
| `:discard` | Discard all pending comments |
| `:history` | View submission history |
| `:mark-all-reviewed` | Mark all files as reviewed |
| `:mark-all-unreviewed` | Mark all files as unreviewed |
| `:ref <rev>` | Set the base ref directly (:ref auto follows new commits) |
| `:submit!` | Submit immediately, no modal (approve, or request changes) |
| `:theme [name]` | Switch theme live (dark/light/molokai/dracula/nord; no arg cycles) |
| `:unpause` | Cancel a pending pause request |

#### App

| Key | Action |
|-----|--------|
| `q` | Quit |
| `H` | Show this help |
| `i` | Review summary: what this round fixed, and the commits it contains (press again to close) |
| `I` | Connection info (socket path, subscriber count) |
| `esc` | Close the current modal / leave visual or search mode |

#### Text editing (comment/submit modals)

| Key | Action |
|-----|--------|
| `←` / `→ or ctrl+b` / `f` | Move cursor left/right |
| `↑` / `↓ or ctrl+p` / `n` | Move cursor up/down |
| `home` / `ctrl+a` | Line start (smart toggle) |
| `end` / `ctrl+e` | Line end |
| `alt+← or alt+b` | Move back one word |
| `alt+→ or alt+f` | Move forward one word |
| `ctrl+d` / `delete` | Delete char at cursor |
| `ctrl+k` | Kill to end of line |
| `ctrl+u` | Kill to start of line |
| `ctrl+w` / `alt+bksp` | Delete word before cursor |
| `alt+d` | Delete word after cursor |
| `alt+enter` | Insert a newline (shift+enter too, where the terminal sends it) |
| `tab` / `shift+tab` | Next / previous comment type |
| `ctrl+g` | Open in your external editor |

### Comment editor

The comment editor supports standard emacs-style shortcuts:

| Key                                    | Action                                          |
|----------------------------------------|-------------------------------------------------|
| `<-`/`->` or `Ctrl+B`/`Ctrl+F`         | Move cursor left/right                          |
| `up`/`down` or `Ctrl+P`/`Ctrl+N`       | Move cursor up/down (multiline)                 |
| `Home`/`Ctrl+A`                        | First non-whitespace, then start of line        |
| `End`/`Ctrl+E`                         | End of line                                     |
| `Ctrl+D` or `Delete`                   | Delete character at cursor                      |
| `Ctrl+K`                               | Kill to end of line                             |
| `Ctrl+U`                               | Kill to start of line                           |
| `Ctrl+W` or `Alt+Backspace`            | Delete word before cursor                       |
| `Alt+D`                                | Delete word after cursor                        |
| `Alt+<-` or `Alt+B`                    | Move cursor back one word                       |
| `Alt+->` or `Alt+F`                    | Move cursor forward one word                    |
| `Shift+Enter` or `Alt+Enter`           | Insert newline                                  |
| `Ctrl+G`                               | Open in external editor (`$VISUAL`/`$EDITOR`)   |
| `Tab`                                  | Cycle comment type                              |
| `Enter`                                | Save comment                                    |
| `Esc`                                  | Cancel                                          |

## CLI

```
monocle [--socket PATH]              Start a review session (auto-spawns monocle serve)
monocle serve [--idle-timeout DUR]   Run a headless engine for this repo (socket server, no TUI)
monocle stop                         Stop the running monocle serve for this repo
monocle register [agent] [--global]  Register Monocle for an agent
monocle unregister [agent] [--global] Remove Monocle registration
monocle --version                    Print version
```

The `agent` argument is one of `claude`, `opencode`, `codex`, `gemini`, `pi`, or `all`. If omitted, an interactive picker lets you select which agents to register. The `--global` flag writes to the user-level config directory instead of the project.

### How the engine runs

Monocle 0.46+ splits the engine from the frontend. `monocle serve` owns the SQLite database, the review session, and the Unix socket; the `monocle` TUI, agent CLI commands, and future frontends all attach as thin socket clients. You don't usually need to run `monocle serve` yourself — launching `monocle` auto-spawns one in the background for the current repo. It exits on its own 60 seconds + `--idle-timeout` (default 30 min) after the last client disconnects.

### Agent-Facing Commands

These commands are used by agents (via MCP tools or skills) to interact with a running Monocle session:

```
monocle review status [--json]                          Check review status
monocle review get-feedback [--wait] [--json]            Retrieve review feedback
monocle review send-artifact --title T [--file F] [--id ID] [--type EXT] [--wait] [--json]
                                                         Send content for review
monocle review add-files <paths...> [--json]             Add files to review session
monocle review remove-files <paths...> [--json]          Remove previously-added files
monocle review group-files [--file M] [--replace] [--json]  Group/order changed files (grouped view)
monocle review annotate [--file M] [--replace] [--json]    Annotate code ranges with doc links
monocle review set-name <name> [--force] [--json]        Start/name a review (refused if one is open with comments)
monocle review set-label [label] [--json]                Name this Monocle in the TUI's top-left (omit to clear)
monocle review set-base-ref <ref> [--reset] [--json]     Review already-committed work (diff against <ref>)
monocle review set-walkthrough [--file F] [--clear] [--json]  Send a guided tour of the review (stops stepped with . and ,)
monocle review goto-stop <id> [--json]                   Move the reviewer to a tour stop
monocle review goto-line <path> <line> [--top N] [--json]  Show the reviewer a file of the review at a line
monocle review highlight <path> <start> <end> [--json]   Highlight a range of lines (replaces any earlier one)
monocle review highlight --clear [--json]                Remove the highlighted range
monocle review open-editor <path> <line> [--full] [--json]  Open any repo file at a line in the editor beside Monocle
```

- `--wait` blocks until the reviewer responds (used by `/review-plan-wait`)
- `--json` outputs structured JSON for programmatic use
- `send-artifact` reads from `--file` or stdin

### Manual Socket Override

If auto-pairing fails (e.g., the agent's working directory differs from Monocle's), you can manually specify the socket path:

- **Monocle:** `monocle --socket /tmp/monocle-abc123.sock`
- **Agent commands:** `MONOCLE_SOCKET=/tmp/monocle-abc123.sock monocle review status`
- **MCP channel (Claude):** Set `MONOCLE_SOCKET` in `.mcp.json` env

Press `I` in the TUI to see the current socket path and connection status.

## Configuration

Monocle loads settings from JSON config files:

1. **Global:** `~/.config/monocle/config.json` (or `$XDG_CONFIG_HOME/monocle/config.json`)
2. **Project:** `.monocle/config.json` in the working directory (overrides global)

> **Note:** The background engine (`monocle serve`) reads the config files again whenever they change, and a running `monocle` picks the change up within a second. Settings read when they are used (the `walkthrough_*` commands, `related_editor_args`, the editor and viewers, `context_lines`, `mark_reviewed_on_submit`) apply straight away; those applied when the TUI starts (theme, keybindings, layout, display settings) apply the next time `monocle` starts; `ignore_patterns` and `idle_timeout` apply after `monocle stop` restarts the engine. A file that does not parse is skipped and the last good config kept.

```json
{
  "layout": "auto",
  "diff_style": "unified",
  "sidebar_style": "flat",
  "theme": "dark",
  "wrap": false,
  "tab_size": 4,
  "context_lines": 3,
  "full_file_diff": false,
  "editor": "",
  "editor_mode": "terminal",
  "editor_focus": true,
  "markdown_viewer": "",
  "media_viewer": "",
  "walkthrough_on_stop": "",
  "walkthrough_view_status": "",
  "walkthrough_layout_reset": "",
  "walkthrough_ask": "",
  "walkthrough_resolve": "",
  "cursor_command": "",
  "highlight_color": "",
  "related_editor_args": [],
  "related_editor_add": "",
  "related_editor_preview": "",
  "ignore_patterns": [],
  "keybindings": {},
  "mouse": true,
  "min_diff_width": 80,
  "auto_focus_mode": false,
  "comment_expand": true,
  "comment_expand_delay": 2000,
  "review_tracking": true,
  "mark_reviewed_on_submit": "all",
  "idle_timeout": "30m",
  "review_format": {
    "include_snippets": true,
    "max_snippet_lines": 10,
    "include_summary": true
  }
}
```

| Setting                              | Values                                     | Default      | Description                                                              |
|--------------------------------------|--------------------------------------------|--------------|--------------------------------------------------------------------------|
| `layout`                             | `"auto"`, `"side-by-side"`, `"stacked"`    | `"auto"`     | Pane arrangement (`auto` switches based on terminal width)               |
| `diff_style`                         | `"unified"`, `"split"`, `"file"`           | `"unified"`  | Diff display mode (`file` shows raw content)                             |
| `sidebar_style`                      | `"flat"`, `"tree"`, `"grouped"`                         | `"flat"`     | File list display mode (grouped = files grouped by category)                                                   |
| `theme`                              | `"dark"`, `"light"`, `"molokai"`, `"dracula"`, `"nord"` | `"dark"` | Color scheme for the TUI. Switch live with `:theme <name>` (or `:theme` to cycle). |
| `wrap`                               | `true`, `false`                            | `false`      | Word-wrap long lines in diffs                                            |
| `tab_size`                           | integer                                    | `4`          | Spaces per tab character                                                 |
| `context_lines`                      | integer                                    | `3`          | Unchanged lines shown around diff hunks                                  |
| `full_file_diff`                     | `true`, `false`                            | `false`      | Show the whole file with diff coloring instead of compact hunks (toggle with `a`) |
| `editor`                             | string                                     | `""`         | External editor command (overrides `$VISUAL`/`$EDITOR`); may include flags, e.g. `"code --wait"` |
| `editor_mode`                        | `"terminal"`, `"tmux_vertical"`, `"tmux_horizontal"`, `"tmux_window"` | `"terminal"` | How `Ctrl+g`/`Ctrl+o` open: take over the screen, or (inside tmux) open in a side-by-side split, stacked split, or new window/tab. Falls back to `terminal` outside tmux |
| `editor_focus`                       | `true`, `false`                            | `true`       | Whether a new tmux split/window takes focus                              |
| `markdown_viewer`                    | string                                     | `""`         | Rendered-markdown viewer for `Ctrl+p` (artifacts / `.md` files); may include flags and quoted args (e.g. `open -a "Google Chrome"`). Empty falls back to `glow -p` |
| `media_viewer`                       | string                                     | `""`         | Viewer for media artifacts / files opened with `Ctrl+p` (images, video, audio); may include flags and quoted args. Empty falls back to Google Chrome (`open -a "Google Chrome"` on macOS) |
| `walkthrough_on_stop`                | string                                     | `""`         | Shell command run (`sh -c`, in the repo root, fire-and-forget, 30s timeout) on every guided-tour stop you settle on, with `MONOCLE_STOP_ID`, `MONOCLE_REPO_ROOT` and `MONOCLE_STOP_JSON` (the stop, view targets resolved to absolute paths) in its environment. Use it to show a stop's screenshots/recordings and arrange windows. `:view N` runs it too, with `MONOCLE_VIEW_INDEX` / `MONOCLE_VIEW_NAME` naming the one view asked for. Empty runs nothing |
| `walkthrough_view_status`            | string                                     | `""`         | Shell command that says how the windows around the current tour stop stand, so each view's label is marked `(open)`, `(hidden)` or `(not opened)`. Run like `walkthrough_on_stop` (same environment) but with a 500ms timeout, when a stop is shown and after a view is opened; prints one line of JSON: `{"views": {"view": "open"\|"hidden"\|"closed", "view2": …}, "layout": "saved"\|"default"}`. Empty or a failure shows nothing |
| `walkthrough_layout_reset`           | string                                     | `""`         | Shell command that puts the windows around a tour back in their default layout. When `walkthrough_view_status` says `"layout": "saved"`, the tour note shows `Layout: saved · reset`; clicking `reset`, or `:layout reset`, runs it (`sh -c`, in the repo root, with the same environment as the status command, 10s timeout) and then asks the view status again. Empty runs nothing |
| `walkthrough_ask`                    | string                                     | `""`         | Shell command that hands the agent the lines you send with `@`: the selection and the tagged lines, else the cursor's line. Runs `sh -c` in the repo root, 10s timeout, with `MONOCLE_REPO_ROOT` and `MONOCLE_ASK_JSON` — `{"tour", "stop", "repo", "refs": [{"path", "start", "end", "side"}]}`, `side` `"old"` for removed lines in old-file numbers, `tour`/`stop` empty outside tour mode — in its environment. It puts the reference in the agent's prompt and moves focus there. Empty sends nothing |
| `walkthrough_resolve`                | string                                     | `""`         | Shell command that says what the lines you point at with `ctrl+]` reference (the SQL file a query loads, the file a function lives in). Runs `sh -c` in the repo root, 10s timeout, with `MONOCLE_REPO_ROOT` and `MONOCLE_RESOLVE_JSON` (the shape of `MONOCLE_ASK_JSON`), and prints a JSON array `[{"path": "db/queries/x.sql", "line": 12}]`, paths repo-relative or absolute. monocle opens them in the related-files pane beside what it holds, at most 8 files, and moves focus there. Empty opens nothing |
| `cursor_command`                     | string                                     | `""`         | Shell command run when the diff cursor rests on a new file:line (about 200 ms after the last move, never twice in a row for the same place, never while a modal is open), with `MONOCLE_FILE` (repo-relative), `MONOCLE_LINE` (new-file line, `0` when the row has none) and `MONOCLE_REPO_ROOT`. `sh -c` in the repo root, fire-and-forget, 2s timeout, output discarded. Empty runs nothing |
| `highlight_color`                    | string                                     | `""`         | Background of the range an agent highlights (`highlight_range`), as a colour such as `"#2d3b6b"` or an ANSI number. Empty uses the theme's: `#2d3b6b` on the dark themes, `#cdd8ff` on `light` |
| `related_editor_args`                | string[]                                   | `[]`         | Extra arguments appended to the editor command of a tour stop's related-files pane, e.g. `["-S", "/path/setup.vim"]`; `{owner}` in them becomes monocle's tmux pane id (e.g. `--listen /tmp/nvim-{owner}.sock`). With vim/nvim, `+cmd`, `-c` and `-S` share a limit of ten (the pane uses one `-c`) and `--cmd` has its own ten; arguments past either are left out, with a status-bar note |
| `related_editor_add`                 | string                                     | `""`         | Shell command that adds one file to the editor already in the related-files pane, so files you closed there stay closed. With it set and the pane alive, `ctrl+]`, `ctrl+g` in a tour, `:related N` and a click on a related file run it once per file instead of respawning the pane. `{file}` (absolute path), `{line}` (`0` for none) and `{owner}` (monocle's tmux pane) are substituted shell-quoted. It should bring an open file forward rather than open it twice. Empty respawns |
| `related_editor_preview`             | string                                     | `""`         | Shell command that shows one file in a passing preview in the editor already in the related-files pane (a popup the editor closes once you move on) instead of adding it. `p` / `f16` runs it for the first file `walkthrough_resolve` finds and moves focus there. Placeholders as `related_editor_add`'s. Empty, or no live pane: the key opens the files as `ctrl+]` does |
| `ignore_patterns`                    | string array                               | `[]`         | Glob patterns for files to exclude                                       |
| `min_diff_width`                     | integer                                    | `80`         | Minimum character width for the diff viewer in side-by-side layout       |
| `mouse`                              | `true`, `false`                            | `true`       | Enable mouse interactions (click, scroll, drag), including clicking a tour stop's views                          |
| `auto_focus_mode`                    | `true`, `false`                            | `false`      | Auto-enter focus mode (hide sidebar, enable wrap) when reviewing plans   |
| `comment_expand`                     | `true`, `false`                            | `true`       | Auto-expand comments on hover                                            |
| `comment_expand_delay`               | integer (ms)                               | `2000`       | Delay before auto-expanding a selected comment (0 = instant)             |
| `review_tracking`                    | `true`, `false`                            | `true`       | Enable review state tracking, snapshots, and change detection. Set to `false` to get raw diffs with no reviewed indicators. |
| `mark_reviewed_on_submit`            | `"all"`, `"commented"`, `"manual"`         | `"all"`      | Which files to mark as reviewed when submitting (requires `review_tracking`) |
| `idle_timeout`                       | duration string (e.g. `"30m"`, `"1h"`)     | `"30m"`      | How long `monocle serve` stays alive after the last client disconnects (plus a 60s grace window). Overridden by `--idle-timeout`. |
| `keybindings`                        | object                                     | `{}`         | Custom key overrides (see below)                                         |
| `review_format.include_snippets`     | `true`, `false`                            | `true`       | Include code snippets in formatted reviews                               |
| `review_format.max_snippet_lines`    | integer                                    | `10`         | Truncate snippets longer than this                                       |
| `review_format.include_summary`      | `true`, `false`                            | `true`       | Include comment count summary in formatted reviews                       |

Toggle keybindings (`T`, `t`, `a`, `w`, `f`) change settings for the current session only. Edit the config file to persist your preferences.

### Custom Keybindings

Override any action key by mapping the action name to a new key string:

```json
{
  "keybindings": {
    "quit": "Q",
    "submit": "ctrl+s",
    "scroll_down": "ctrl+j"
  }
}
```

Available action names: `answer`, `artifact_versions`, `base_ref`, `block_match`, `block_top`, `block_up`, `bottom`, `clear_review`, `close_related`, `collapse_all`, `command_mode`, `comment`, `cycle_layout`, `dismiss_artifact`, `down`, `expand_all`, `expand_all_comments`, `expand_comment`, `file_comment`, `filter_reviewed`, `focus_swap`, `half_down`, `half_up`, `help`, `hide_comments`, `hide_comments_back`, `jump_back`, `jump_forward`, `layout_reset`, `next_file`, `next_mark`, `next_section`, `open_doc_ref`, `open_in_editor`, `open_in_editor_takeover`, `open_in_markdown_viewer`, `open_refs`, `open_refs_preview`, `next_symbol`, `prev_symbol`, `open_terminal`, `open_terminal_takeover`, `pane_down`, `pane_size`, `pane_left`, `pane_right`, `pane_up`, `pause`, `prev_file`, `prev_mark`, `prev_section`, `question`, `quit`, `refresh`, `relaunch`, `review_summary`, `reviewed`, `scroll_down`, `scroll_end`, `scroll_first_char`, `scroll_home`, `scroll_left`, `scroll_right`, `scroll_up`, `search_backward`, `search_next`, `search_prev`, `select`, `send_lines`, `shell_command`, `submit`, `suggest`, `tag_lines`, `toggle_diff`, `toggle_focus_mode`, `toggle_full_diff`, `toggle_overlays`, `toggle_sidebar`, `toggle_tour`, `top`, `tour_back`, `tour_forward`, `tour_next`, `tour_prev`, `tree_mode`, `up`, `visual`, `wizard_advance`, `wizard_back`, `wizard_toggle`, `wrap`, `yank_line`.

The help overlay (`H`) dynamically reflects your custom bindings. Modal keys (Enter, Esc, Tab in overlays) are not configurable.

## Automatic content review

By default, Monocle's operations are available to your agent but the agent decides when to use them on its own. If you want the agent to automatically submit plans or other content for review, add instructions to your agent's project configuration (e.g. `CLAUDE.md`, `AGENTS.md`, etc.):

````markdown
## Monocle Integration

When Monocle is running:
- Use the `/review-plan` skill to send content (plans, decisions, summaries) for the reviewer to see
- Use the content's filename as the identifier so updates replace the previous version
- In plan mode, use `/review-plan-wait` instead — it blocks until the reviewer responds. If they request changes, update and resubmit until approved.
````

You can also invoke `/review-plan` and `/review-plan-wait` manually at any time.

## How it works

```
┌─────────────┐                ┌───────────────┐              ┌──────────┐
│   Agent     │<--stdio/MCP--->│  serve-mcp    │<---socket--->│ monocle  │
│             │                │ (MCP server)  │              │  (TUI)   │
└─────────────┘                └───────────────┘              └──────────┘
```

1. You leave line-level comments on diffs — issues, suggestions, notes, praise
2. You press `S` to submit your review
3. Monocle queues the review for delivery
4. The agent picks up the feedback — automatically via push notification (Claude Code with channels) or when you trigger `/get-feedback` — and starts addressing your comments
5. You see the updated diffs in real time, review again, and iterate

Feedback is always queued for reliability. How the agent learns about it depends on the integration:

- **Claude Code with channels:** A push notification is sent through the MCP channel with a summary of the review (e.g., "Your reviewer requested changes — 2 issues, 1 suggestion"). The agent runs `/get-feedback` to retrieve the full review. If the push fails silently (channels not enabled), the review stays in the queue.
- **Any agent:** The agent calls `get_feedback` (MCP tool) or `monocle review get-feedback` (CLI), either via a `/get-feedback` slash command or on its own. Multiple reviews accumulate in the queue and are delivered together.

If you want the agent to pause and wait for you to finish reviewing, press `P` — the agent receives a pause notification and blocks until your review is ready.

## License

MIT
