# Guided tours — design decisions

Why the walkthrough feature behaves the way it does. This is for people changing it; the
user-facing guide is [`guides/guided-tours.mdx`](../guides/guided-tours.mdx). Tunables (timeouts,
pane sizes) are deliberately not repeated here — read them from the code, where they cannot drift.

## The tour and the engine

- **Stop ids are the reviewer's vocabulary.** The id is what the reviewer types (`:stop 1.2`) and
  what the agent answers to, so a duplicate id is refused rather than silently renamed. A stop sent
  without an id is numbered by its position.
- **The current stop lives in the engine, not the TUI.** A TUI restart resumes on the same stop, and
  an agent's `goto_stop` can report where the reviewer was.
- **Only agent moves announce.** `goto_stop` persists the stop and emits an event; the TUI's own
  `.`/`,` persist it silently. An echo would bounce the TUI back to where it already is and re-run
  every side effect of arriving.
- **Re-sending a tour keeps the reviewer's place** when their stop's id survives, and otherwise
  starts at the first stop. An empty stop list withdraws the tour, as an empty summary does.
- **Problems warn; they do not refuse.** A stop whose file is not in the review, or a related file
  that does not exist, is reported back to the agent, and the tour is still set. The agent is the
  authority on what it is showing, and a file can legitimately arrive after the tour does.
- **The tour belongs to the review.** It is dropped on clear and on approve, like the summary and
  artifacts: the next review opening on stale stops would mislead.
- **Setting a tour is a handover; moving through one is not.** `set_walkthrough` starts the "sent N
  ago" clock; `goto_stop` does not.
- **One JSON shape everywhere.** The CLI's tour file, the socket message, the stored row and
  `MONOCLE_STOP_JSON` all use the domain type's fields. `related` reuses the annotation `DocRef`.
- **The CLI refuses unknown fields** in a tour file. `"line"` instead of `"line_start"` fails naming
  the field, rather than silently landing the stop at line 1.

## Stepping

- **The ends clamp; they do not wrap.** Wrapping from the last stop to the first on one keypress
  loses the reader's place, which is what a tour exists to prevent.
- **Turning the tour off hides it without forgetting it.** `W` keeps the tour and the stop; with it
  off, the first `.`/`,` resumes the stop you left instead of stepping.
- **Restoring is not arriving.** A tour arriving enters its stop and runs the stop's side effects. A
  TUI restart restores the stop and runs none — nothing moved.
- **A stop's range is marked in the gutter in its own colour.** The annotation rail and the summary
  bar already own the other gutter marks; removed lines have no new-file number and are not marked.
- **Landing reuses jump history,** so `ctrl+o` returns from a stop. A file-level stop lands on the
  file's first change.
- **A stop's file load wins races.** While it is loading, a diff load for any other file is dropped:
  an agent typically sends files, then the tour, and a refresh it triggered a moment earlier could
  otherwise finish after the jump and put the note under the wrong file.
- **The file list hides when a tour starts, not when one is re-sent.** An agent correcting a note
  mid-review must not undo the reviewer's choice to show the list again.
- **Labels are pinned; the note scrolls.** Related files, views and layout sit at the bottom of the
  note pane so they stay reachable under a long note, and the header says how much is out of sight.

## The related-files pane

- **One `-c`, steps joined with `|`.** vim and nvim run at most ten `-c` commands, so a
  command per window would break at five related files. Each step uses `exe 'normal! …'` because a
  bare line-number range before `|` is not a reliable jump.
- **Only vim-like editors get splits.** Any other editor opens the first related file the way
  `ctrl+g` opens any file.
- **Arriving at a stop does not take the keyboard.** The tour is driven from Monocle, and a split
  that grabbed focus on the first stop with related files would swallow the next `.`. Opening a file
  explicitly — its label or `:related N` — does take it, because that is a request to look at it.
- **Each Monocle finds only its own pane,** tagged with a tmux pane option, so after a restart the
  old split is reused rather than a second one opened. Monocle's own pane is never respawned or
  killed, whatever the tracked id says.
- **Side effects are debounced.** Holding `.` to skip ahead opens one pane for the stop you land on,
  not one per stop passed. The diff and the note update at once; only the out-of-Monocle effects wait.
- **Outside tmux there is no fallback.** Taking the screen over on every step would be worse than
  saying related files need tmux.

## Hooks

- **The on-stop command runs in the TUI,** which is the process that knows a stop was entered.
- **`sh -c`, not `$SHELL`,** so no interactive rc file runs and it behaves the same for everyone.
- **stdout is discarded; stderr is kept.** stdout would draw over the TUI. The last stderr line is
  what a failure reports.
- **A timeout kills the whole process group,** so a viewer the command spawned cannot outlive it
  holding the pipes.
- **Monocle never opens a stop's views on its own.** Opening a browser on every `.` would be hostile
  without a configured command. `:view N` (or a click on a view's label) opens one view — through
  the on-stop command when one is set, since it opened the windows and is the only thing that can
  raise one already showing; otherwise through the configured viewer.
- **A label click is the same message as its command.** Each label carries exactly what `:view N`,
  `:related N` or `:layout reset` would send, so click and command cannot diverge.
- **The view-status command fails closed.** Output without a `views` object is a failure, so an older
  flat shape cannot be misread as "everything closed". Any failure clears the markers rather than
  leaving stale ones.

## Stop-tagged comments

- **A comment made while the tour is on records its stop,** shown as `[1.2]` in the TUI and prefixed
  to the delivered body. A body that starts with a code fence gets the tag on its own line, since
  text before ```` ``` ```` breaks the fence.
- **Comments made with the tour off are not tagged,** even though the engine still remembers a stop:
  off means the reviewer is not following the tour.

## Testing

- **Test binaries never touch the developer's real state.** A `TestMain` in the client, core, cmd
  and tui packages points `XDG_CONFIG_HOME`, `XDG_DATA_HOME` and `MONOCLE_DB` at a temp dir. This
  exists because an upstream round-trip test saved config through a real engine — on a branch that
  added a config key, every test run wrote it into the developer's own `config.json`.
- **Tests never reach a real tmux server or viewer.** `TMUX` is unset, and tests that could open a
  view configure `true` as the viewer.

## Known issues, not fixed here

- `docs/reference/keybindings.mdx` repeats its "View & panes" section several times — a merge
  artefact that predates this feature.
- `dropSQL` in `internal/db/schema.go` does not drop `summary_items`. Only reachable from a schema
  older than 13.
