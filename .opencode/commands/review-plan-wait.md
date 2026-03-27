---
description: Send a plan to Monocle and wait for review feedback
---

Submit a plan file to Monocle and block until the reviewer responds with feedback. Use this when you need reviewer approval before proceeding.

1. If the user provided a file path as an argument, use that. Otherwise, find the most recently modified plan file in the project.
2. Read the plan file to get its content and filename.
3. Call the monocle `submit_plan_and_wait` tool with:
   - `title`: The first markdown heading from the plan, or the filename if no heading found
   - `file_path`: Absolute path to the plan file
   - `id`: The plan filename (so updates replace the previous version)
   - `content_type`: `"md"`
4. Handle the response:
   - If approved with no comments, inform the user and continue.
   - If feedback requests changes, act on it — update the plan, then call `submit_plan_and_wait` again.
   - Keep iterating until the reviewer approves.
