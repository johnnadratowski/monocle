---
description: Send a plan to Monocle for review
---

Submit a plan file to Monocle so your reviewer can see it. This does NOT wait for feedback — use `/review-plan-wait` if you need to block until the reviewer responds.

1. If the user provided a file path as an argument, use that. Otherwise, find the most recently modified plan file in the project.
2. Read the plan file to get its content and filename.
3. Call the monocle `submit_plan` tool with:
   - `title`: The first markdown heading from the plan, or the filename if no heading found
   - `file_path`: Absolute path to the plan file
   - `id`: The plan filename (so updates replace the previous version)
   - `content_type`: `"md"`
4. Confirm to the user that the plan was sent to Monocle.
