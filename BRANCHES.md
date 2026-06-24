# Work branches

Tracking doc for the in-flight feature/fix branches in this repo. This file
lives on **`integration/diffview-improvements`**, the local "merge everything"
branch used to test all the work together before the individual PRs land.

Each feature branch is meant to become its **own PR into `main`**. The
integration branch is **not** meant to be merged into `main` — it just bundles
the branches for local testing.

## Branches

| Branch | Base | Status | Summary |
|--------|------|--------|---------|
| `fix/diffview-scroll` | `main` | ready for PR | Inline-comment scroll fix, `t`-toggle re-anchor, split-divider-on-wrap fix, expanded-comment trailing blank-line fix |
| `feat/full-file-diff` | `fix/diffview-scroll` (stacked) | ready for PR | Full-file diff modifier (`a`) + `full_file_diff` config; reuses the re-anchor helpers from the scroll branch |
| `fix/editor-config` | `main` | ready for PR | `editor` config field overriding `$VISUAL`/`$EDITOR` |
| `fix/remove-additional-files` | `main` | ready for PR | Remove added files (`x` in sidebar + `:clear`); `remove_files` MCP tool + `monocle review remove-files` CLI |
| `feat/diff-search` | `main` | ready for PR; **merged into integration** | Vim-style diff search (`/` `?` `n` `N`) + keybinding reshuffle (Help→`H`, sidebar toggle→`;`, `scroll_left` drops `H`) |
| `feat/theme` | `main` | ready for PR; **merged into integration** | UI theming via named themes + Molokai (from the user's molokai vim colorscheme), Dracula, Nord; `:theme` command |
| `feat/yank-line` | `main` | ready for PR; **merged into integration** | `y` yanks the current line (or visual selection) to the clipboard |
| `feat/mouse-scroll` | `main` | ready for PR; **merged into integration** | Wheel scrolling scrolls the diff for any event not clearly over a visible sidebar — focus- and layout-independent (robust to the empirical mouse-origin offset and a hidden sidebar) |
| `feat/help-panel` | `integration` | **merged into integration** | Clamp help-overlay scrolling (no more overshoot past the bottom) + `g`/`G`; in-help `/` search with match highlighting and `n`/`N` |
| `feat/search-history` | `integration` | **merged into integration** | Shared, de-duplicated search history across the diff and help panels; `n`/`N` reuses the last query in any panel; `↑`/`↓` recall earlier searches while typing |
| `feat/file-grouping` | `integration` | **stage 1 merged into integration** | Grouped sidebar view: `f` cycles flat→tree→grouped; heuristic file categories (code/test/config/docs/build) with group headers; `sidebar_style: "grouped"` default |
| `feat/file-grouping-agent` | `integration` | **stage 2 merged into integration** | Agent grouping pipeline: per-file churn (git numstat), `set_file_groups` MCP tool + `monocle review group-files` CLI, `file_metadata` table (schema 9), grouped view orders by agent group label/order then churn |
| `feat/context-clear-search` | `main` | placeholder | Empty placeholder for future "clear no-longer-needed context" work |
| `integration/diffview-improvements` | `main` | integration only | Merge of all the above for local testing — do not PR into `main` |

## PR / merge order

`feat/full-file-diff` is **stacked on** `fix/diffview-scroll` (it depends on the
`reanchorTo`/`indexForNewLine` helpers). Merge the scroll PR first, then the
full-file PR. The other branches are independent and can merge in any order,
though they touch overlapping files (keybinding tables, `keys.go`, `app.go`,
docs), so expect small union-merge conflicts — resolve by keeping both sides.

## Keybinding changes to be aware of (from `feat/diff-search`)

- `?` → backward diff search; **Help moved to `H`**.
- Toggle sidebar moved from `\` to `;`.
- `scroll_left` lost its default `H` binding (lowercase `h` still scrolls the
  focused diff; the any-pane horizontal scroll keeps `L`).

## Keeping this file current

When a branch lands or a new one is cut, update the table above. Regenerate the
branch heads with:

```bash
for b in fix/diffview-scroll feat/full-file-diff fix/editor-config \
         fix/remove-additional-files feat/diff-search feat/theme \
         integration/diffview-improvements; do
  printf '%-34s %s\n' "$b" "$(git log --oneline -1 "$b" 2>/dev/null)"
done
```
