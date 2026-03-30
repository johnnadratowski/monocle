# Plugins

Agent-specific plugin configurations and skills for Monocle. Each subdirectory contains the skills and configuration files installed by `monocle register`.

| Directory | Agent | Push notifications | Details |
|-----------|-------|---------------------|---------|
| [`claude/`](claude/README.md) | [Claude Code](https://claude.com/claude-code) | MCP channel | Skills + MCP channel server for push notifications |
| [`codex/`](codex/README.md) | [Codex CLI](https://github.com/openai/codex) | - | Skills (pull-based feedback via `/get-feedback`) |
| [`gemini/`](gemini/README.md) | [Gemini CLI](https://github.com/google/gemini-cli) | - | Skills + extension manifest (pull-based feedback via `/get-feedback`) |

All agents share the same three skills:

| Skill | Description |
|-------|-------------|
| `/get-feedback` | Retrieve pending review feedback |
| `/review-plan` | Send a plan file to Monocle for review (returns immediately) |
| `/review-plan-wait` | Send a plan file and block until the reviewer responds |

See the [main README](../README.md#automatic-content-review) for how to configure agents to automatically submit content for review.
