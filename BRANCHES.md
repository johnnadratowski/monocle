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
| `fix/diffview-scroll` | `main` | ready for PR | Inline-comment scroll fix, `t`-toggle re-anchor, split-divider-on-wrap fix |
| `feat/full-file-diff` | `fix/diffview-scroll` (stacked) | ready for PR | Full-file diff modifier (`a`) + `full_file_diff` config; reuses the re-anchor helpers from the scroll branch |
| `fix/editor-config` | `main` | ready for PR | `editor` config field overriding `$VISUAL`/`$EDITOR` |
| `fix/remove-additional-files` | `main` | ready for PR | Remove added files (`x` in sidebar + `:clear`); `remove_files` MCP tool + `monocle review remove-files` CLI |
| `feat/diff-search` | `main` | ready for PR; **merged into integration** | Vim-style diff search (`/` `?` `n` `N`) + keybinding reshuffle (Help→`H`, sidebar toggle→`;`, `scroll_left` drops `H`) |
| `feat/theme` | `main` | ready for PR | UI theming via named themes + Molokai (from the user's molokai vim colorscheme), Dracula, Nord; `:theme` command. **Not yet merged into integration.** |
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
