---
title: Agent tooling audit
description: Size, overlap and annotation audit of the levelrail-mcp tool surface, with the token budget test that keeps it from regressing.
---

# Agent tooling audit

An MCP client loads every tool definition from `tools/list` into the model's context before the first user message. This page records what that costs for `levelrail-mcp` per mode, which tools overlap, and the tests that stop the listing growing unnoticed.

Numbers are estimates: the serialized `tools/list` JSON length divided by 4 (no tokenizer is in `go.mod`). They are good for comparing modes and catching regressions, not for billing. They are a snapshot of the 156 registered tools and drift as tools are added, so rerun the commands below for current figures.

This page complements [MCP tool surface](mcp-tool-surface.md). That report counts each tool's name, description and input schema per toolset. This page measures the whole serialized `tools/list` response a client receives, which also carries output schemas and annotations, so its totals are larger.

<InlineToc default-open />

## Tools and tokens per mode

Measured by `go test -run TestToolListTokenBudget -v ./internal/mcptools` (or `scripts/mcp-token-budget.sh`).

| Mode | Tools | Estimated tokens | Test budget |
| --- | --- | --- | --- |
| `read-only` | 118 | about 46,200 | 47,000 |
| `standard` (default) | 146 | about 61,200 | 62,000 |
| `full` | 156 | about 66,700 | 68,000 |

Modes are defined in `internal/mcptools/modes.go` and driven by the class table in `internal/mcptools/classes.go`: 118 read, 28 mutating and 10 destructive tools. `read-only` registers read tools, `standard` adds mutating tools, and `full` adds destructive ones. The API token's own abilities still apply on top of the mode.

## The agent-core profile

The `agent-core` profile lists 15 tools and omits output schemas from `tools/list` (results are still returned as text and structured content). Its full listing is about 2,600 estimated tokens against about 66,700 for `full` mode. The tools are listed in [MCP tool surface](mcp-tool-surface.md#agent-core-profile). A test holds the listing under a budget (default 3,500, override with `APP_MCP_TOKEN_BUDGET_AGENT_CORE`).

Use this profile for autonomous agents: `levelrail-mcp --tool-profile agent-core` or `APP_MCP_TOOL_PROFILE=agent-core`.

## Overlapping tools

| Tools | Overlap | Guidance |
| --- | --- | --- |
| `get_app_status`, `list_deploys` | Identical handler and result (current reconcile conditions). | `agent-core` includes both; in a custom set, one is enough. |
| `deploy_app`, `rollback_app` | Same request and handler. | Both kept: intent is useful and the classes differ. |
| `list_deploys`, `list_deploy_attempts`, `list_deployments`, `list_failed_deploys` | Four ways to list deploy history at different scopes (one app, fleet, failures only). | Prefer `list_deploys` or `list_deploy_attempts` in compact setups. |
| `get_resource_recommendation`, `get_database_resource_recommendation` | Same shape for apps and databases. | Separate by design. |
| `get_app_logs`, `get_model_logs`, `list_archived_logs`, `query_logs` | Four log entry points. | `query_logs` returns a capped, filtered excerpt and is the one in `agent-core`. |
| `preview_promote_app` and `promote_app`, `preview_clone_environment` and `clone_environment` | Preview and apply pairs. | By design: call the read-only preview first. |

## Annotations

- Every registered tool carries a title, `readOnlyHint`, `openWorldHint` and, for non-read tools, `destructiveHint`. `TestEveryToolClassified` in `internal/mcptools/modes_test.go` fails the build if a tool is unclassified.
- `idempotentHint` is set only for tools whose name starts with `set_`. Tools such as `approve_*`, `reject_deploy_approval`, `expire_alert_silence` and `restart_app` are also safe to repeat but are marked non-idempotent.
- Sensitive and untrusted-output tools carry `_meta` flags (`levelrail/sensitive` and its untrusted counterpart), set from the class table.

## Where tokens go

Output schemas dominate. Typed handlers derive them from the Go result struct, so the same large result schema repeats across tools (for example the app resource in `get_app`, `list_apps` and `restart_app`, and the deploy result in `deploy_app`, `rollback_app` and `approve_deploy_approval`). Input schemas and descriptions are the next largest parts. The cheapest reduction is a smaller listing, which is what the `agent-core` profile and the `APP_MCP_TOOLSETS` filter do.

## Regression tests

`internal/mcptools/budget_test.go` serializes `tools/list` for each mode over an in-memory MCP session and fails when the estimate exceeds that mode's budget.

| Env var | Effect |
| --- | --- |
| `APP_MCP_TOKEN_BUDGET_READ_ONLY` | Override the `read-only` ceiling (estimated tokens). |
| `APP_MCP_TOKEN_BUDGET_STANDARD` | Override the `standard` ceiling. |
| `APP_MCP_TOKEN_BUDGET_FULL` | Override the `full` ceiling. |
| `APP_MCP_TOKEN_BUDGET_AGENT_CORE` | Override the `agent-core` profile ceiling. |

Run it with a report of the heaviest tools:

<CopyCommand command="scripts/mcp-token-budget.sh" />

Raise a budget only with a reason in the PR that changes it.
