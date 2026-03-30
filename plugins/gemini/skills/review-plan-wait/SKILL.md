---
name: review-plan-wait
description: Sends a plan file to Monocle and blocks until the reviewer responds with feedback. Use in plan mode or whenever reviewer approval is needed before proceeding.
---

# Send Plan to Monocle and Wait for Review

Submits a plan file to Monocle and blocks until the reviewer responds with feedback.

**Important:** This is for content that isn't already a tracked file change — plans, architecture docs, summaries, etc. You do NOT need to send regular code files; Monocle automatically picks up file changes in the workspace and shows them as diffs.

## Steps

1. **Find the plan file** — if the user provided a path via `$ARGUMENTS`, use that. Otherwise, find the most recently modified plan file in the project.

2. **Read the plan file** to confirm it exists and get its filename.

3. **Run `monocle review send-artifact --wait`** with:
   - `--title`: The first markdown heading from the plan, or the filename if no heading found
   - `--file`: Absolute path to the plan file
   - `--id`: The plan filename (e.g. `my-plan.md`) — ensures updates replace the previous version
   - `--type`: `md`
   - `--wait`: Blocks until the reviewer responds

4. **Handle the response:**
   - If the reviewer approved with no comments, inform the user and continue
   - If the reviewer provided feedback requesting changes, share the feedback with the user and act on it — update the plan, then run `monocle review send-artifact --wait` again
   - Keep iterating until the reviewer approves

If any command fails with a message that Monocle is not running, let the user know they need to start Monocle with `monocle` in the same directory as the project.
