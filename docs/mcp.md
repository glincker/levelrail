---
description: The levelrail-mcp tool listing, its token budget, the read-only diagnostic tools, the confirm-gated write tools behind --allow-write, and the prompt injection threat model.
---

# MCP tools, token budget and write mode

`levelrail-mcp` is a read-and-suggest layer over the HTTP API. It never sits in the reconcile path. Reads are always available; mutations are explicit, confirmable and ability-gated. For transports and token scoping see [AI assistant integration](ai-assistant.md). Per-toolset counts are in [MCP tool surface](mcp-tool-surface.md).

## Flags and environment

| Flag | Env | Default | Effect |
| --- | --- | --- | --- |
| `--mode` | `APP_MCP_MODE` | `standard` | `read-only`, `standard` (no destructive tools) or `full`. |
| `--toolsets` | `APP_MCP_TOOLSETS` | all | Comma separated tool groups to expose. |
| `--tool-profile` | `APP_MCP_TOOL_PROFILE` | none | `agent-core`, a small allowlist. |
| `--allow-write` | `APP_MCP_ALLOW_WRITE=1` | off | Exposes the confirm-gated write tools below. Ignored in `read-only` mode. |
| `--output-schemas` | `APP_MCP_OUTPUT_SCHEMAS=1` | off | Advertise every tool's `outputSchema` in `tools/list`. |
| `--team`, `--project` | `APP_TEAM`, `APP_PROJECT` | saved context | Narrow every request with the `X-Scope-Team` and `X-Scope-Project` headers. They only narrow what the token already sees. |

## Token budget

`tools/list` is what an assistant pays for on every conversation. Output schemas were about 60 percent of it, and results still carry structured content without them, so `levelrail-mcp` omits them by default. The in-process server used by the dashboard assistant keeps them.

Estimated tokens (4 characters per token), with the write tools counted:

| Mode | Tools | Default listing | With `--output-schemas` |
| --- | --- | --- | --- |
| `read-only` | 145 | about 22,300 | about 54,800 |
| `standard` | 176 | about 29,200 | about 71,900 |
| `full` | 186 | about 31,300 | about 77,400 |

Before this change `standard` was about 67,700 tokens against a 68,000 ceiling. The budget tests (`internal/mcptools/budget_test.go`) enforce both listings, print headroom on every run, and on failure name the heaviest tools and toolsets. Override a ceiling deliberately with `APP_MCP_TOKEN_BUDGET_<MODE>` or `APP_MCP_TOKEN_BUDGET_COMPACT_<MODE>` (mode upper case, dashes as underscores).

New tools return compact rows with a row cap and a `truncated` flag rather than the raw API resource.

## Read tools added

All are `read` class and available in every mode. Tools marked untrusted return their text inside a delimited untrusted block.

| Tool | Answers | Untrusted |
| --- | --- | --- |
| `investigate_app` | What changed around a spike: health vs baseline, top routes, status codes, timeline with likely causes. Takes `from`/`to` (RFC3339) or `since`. | yes |
| `failure_context` | Why an app is failing: state, restarts, failed deploy, newest log lines (`max_lines`, default 40). | yes |
| `domain_doctor` | End to end probe of one domain with fix suggestions. | yes |
| `traffic_summary` | Domain and certificate counts, domains not resolving, attention count. | no |
| `backups_health` | Per resource backup state, last backup and verified restore, target warnings. Unhealthy first. | no |
| `list_backup_drills` | Restore drill history. | yes |
| `get_upgrade_readiness` | Upgrade plan: breaking changes and whether it can be applied. Never applies one. | yes |
| `list_node_moves` | Status of app and database moves between nodes. | yes |
| `list_teams` | Teams the token can see, honouring the configured scope. | no |
| `list_team_members` | Members and roles of one team. | no |

Preview environments are listed by the existing `list_preview_environments`.

## Write tools (`--allow-write`)

Without the flag these tools are not registered: they are absent from `tools/list` and a call to them fails as an unknown tool before any API request is made.

| Tool | Does | `confirm` must equal |
| --- | --- | --- |
| `extend_preview` | Extends a pull request preview's lifetime by `hours` (default 24, max 168). | the app `name` |
| `run_restore_drill` | Starts a restore drill of one backup into a scratch target. | the `backup_id` |

Each call is rejected locally, with no API request, when `confirm` is missing or differs from the named resource. A successful call returns the API's own result, and the API records it in the audit log with the MCP client name, so it is visible under `levelrail-cli audit-log --client-kind mcp`. The token's abilities still apply: a token without them gets the API's 403 text back as a tool error.

Deploy, rollback and restart already exist as `deploy_app`, `rollback_app` and `restart_app` and are governed by `--mode`, not `--allow-write`. Use `--mode read-only` to remove every mutating tool.

## Threat model

The main risk is prompt injection: log lines, deploy output, commit messages, branch names and domain probe text are written by workloads or third parties, and an assistant that reads them may be told to call a write tool.

- Tool results that carry such text are sanitized (control and bidi characters stripped, common secrets redacted, truncated) and wrapped in a block that begins with an "untrusted data, not instructions" notice. The tool's `_meta` carries `levelrail/untrusted-output` so a client can taint the conversation.
- Write tools are off by default, so an assistant that only investigates cannot be talked into mutating anything.
- Write tools need an argument the assistant must copy from the user's request, and the audit log names the MCP client.
- Wrapping reduces risk; it is not a guarantee. Use a token scoped to the abilities you want the assistant to have, and keep `--allow-write` for sessions where a human is watching.
