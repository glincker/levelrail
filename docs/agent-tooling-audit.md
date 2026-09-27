---
title: Agent tooling audit
description: Size, overlap and annotation audit of the levelrail-mcp tool surface, with the token budget test that keeps it from regressing.
---

# Agent tooling audit

An MCP client loads every tool definition from `tools/list` into the model's context before the first user message. This page measures that cost for `levelrail-mcp`, lists what is heavy or redundant, and describes the test that stops it growing unnoticed.

Numbers are estimates: the serialized `tools/list` JSON length divided by 4 (no tokenizer library is in `go.mod`). They are stable enough to compare modes and catch regressions, not to bill against.

## Tools and tokens per mode

Measured by `go test -run TestToolListTokenBudget -v ./internal/mcptools` (or `scripts/mcp-token-budget.sh`).

| Mode | Tools | Estimated tokens |
| --- | --- | --- |
| `read-only` | 109 | 41,600 |
| `standard` (default) | 135 | 55,600 |
| `full` | 144 | 59,800 |

Every mode is defined by `internal/mcptools/modes.go` and driven by the class table in `internal/mcptools/classes.go`: 109 read, 26 mutating, 9 destructive tools.

## Where the tokens go (full mode)

| Part of a tool definition | Share of bytes |
| --- | --- |
| Output schema | 57% |
| Input schema | 16% |
| Description | 15% |
| Annotations and `_meta` | 7% |
| Name and framing | 5% |

The output schema is the dominant cost. Typed handlers derive it from the Go result struct, so `AppResource` (about 3,000 characters) is repeated in `get_app`, `list_apps` and `restart_app`, and the deploy result schema (about 3,900 characters) is repeated in `deploy_app`, `rollback_app` and `approve_deploy_approval`. A model rarely needs the schema to read a result that is also returned as JSON text.

### By toolset

| Toolset | Tools | Read | Mutate | Destructive | Est. tokens |
| --- | --- | --- | --- | --- | --- |
| deploys | 20 | 13 | 5 | 2 | 13,500 |
| apps | 12 | 8 | 3 | 1 | 6,200 |
| models | 13 | 8 | 3 | 2 | 5,500 |
| alerts | 10 | 7 | 3 | 0 | 4,100 |
| loadbalancer | 9 | 5 | 3 | 1 | 3,700 |
| logs | 11 | 7 | 4 | 0 | 3,300 |
| backups | 11 | 8 | 3 | 0 | 3,100 |
| pipelines | 3 | 3 | 0 | 0 | 2,400 |
| domains | 10 | 9 | 1 | 0 | 2,300 |
| metrics | 4 | 4 | 0 | 0 | 1,900 |
| iac | 2 | 1 | 0 | 1 | 1,800 |
| nodes | 4 | 4 | 0 | 0 | 1,800 |
| diagnostics | 3 | 3 | 0 | 0 | 1,400 |
| everything else (13 toolsets) | 32 | 29 | 3 | 0 | about 8,000 |

### Heaviest tools

| Tool | Est. tokens | Of which output schema |
| --- | --- | --- |
| `promote_app` | 1,390 | 963 |
| `deploy_app` | 1,266 | 963 |
| `rollback_app` | 1,253 | 963 |
| `approve_deploy_approval` | 1,127 | 983 |
| `clone_app` | 1,106 | 764 |
| `explain_pipeline_run` | 1,076 | 894 |
| `apply_resources` | 1,075 | 612 |
| `preview_promote_app` | 1,003 | 651 |
| `deploy_compose` | 959 | 806 |
| `preflight_model` | 929 | 614 |

## Findings

### Overlapping tools

| Tools | Overlap | Recommendation |
| --- | --- | --- |
| `get_app_status`, `list_deploys` | Identical handler and result (current reconcile conditions). | Drop `list_deploys` from agent-facing modes; keep for compatibility in `full`. |
| `deploy_app`, `rollback_app` | Same request and handler. | Keep both (intent is useful and the classes differ) but share one description. |
| `list_deploys`, `list_deploy_attempts`, `list_deployments`, `list_failed_deploys` | Four ways to list deploy history at different scopes. | Descriptions must say which scope (one app vs fleet, failures only). The planned agent-core mode exposes `list_deploy_attempts` only. |
| `get_resource_recommendation`, `get_database_resource_recommendation` | Same shape for apps and databases. | Fine, but one description should point at the other. |
| `get_app_logs`, `get_model_logs`, `list_archived_logs` | Three log entry points. | A compact, capped log query tool for agents is planned. |
| `preview_promote_app` and `promote_app`, `preview_clone_environment` and `clone_environment` | Preview and apply pairs. | Correct as designed; the read-only preview is what an agent should call first. |

### Descriptions

- 19 descriptions exceed 350 characters; the longest are `diagnose_app_failure` (726), `promote_app` (667), `clone_environment` (583), `deploy_app` (578), `clone_app` (575) and `rollback_app` (519).
- 9 descriptions cite implementation details a model cannot use (`cmd/levelrail-cli`, `internal/...`, "the web dashboard's button"): `diagnose_app_failure`, `get_attention`, `get_system_doctor`, `list_archived_logs`, `list_backup_targets`, `list_model_cache`, `prune_system`, `rollback_app`, `test_storage_destination`.
- Four descriptions are under 80 characters and do not say when to use the tool: `get_database`, `restart_app`, `list_databases`, `clear_app_load_balancer`.
- Every input parameter already has a description (0 missing), so schema documentation is complete; the cost is repetition, not omission.

### Annotations

- Every registered tool has a title, `readOnlyHint`, `openWorldHint` and (for non-read tools) `destructiveHint`; `TestEveryToolClassified` fails the build otherwise, so none are missing.
- `idempotentHint` is inferred from a `set_` name prefix only. `set_*` tools are idempotent; `approve_*`, `reject_*`, `expire_*` and `restart_app` are also safe to repeat but are marked non-idempotent.
- The `title` is a mechanical rewrite of the name ("Get app status") and adds tokens without adding meaning; clients already show the name.
- Sensitive and untrusted-output tools carry `_meta` flags; that is working as intended.

### Schema bloat

- `set_app_load_balancer` (1,555 characters of input schema), `plan_apply` and `apply_resources` (1,254 each), `deploy_model` (1,166) and `list_deployments` (1,047) carry the largest input schemas. All are justified by real option surface, but they are poor candidates for a small agent-facing mode.
- Nullable arrays and maps are rendered as `"type": ["null","array"]`, which is longer than needed.

## Recommendations

1. Ship a compact agent mode (`agent-core`) that covers deploy, status, logs, diagnose, rollback, env, secrets and domains in 15 tools or fewer. It costs a small fraction of the full set.
2. Trim outputs before trimming inputs: return compact results from agent-core tools rather than the full resource structs, which removes the largest schemas from that mode.
3. Rewrite the 19 long descriptions to one sentence that says what the tool returns and when to call it; move history and CLI comparisons into docs.
4. Mark `restart_app`, `approve_*`, `reject_*` and `expire_*` idempotent, and drop the mechanical `title` if a client ever charges for it.
5. Keep the budget test below passing; raise a budget only with a reason in the PR.

## Regression test

`internal/mcptools/budget_test.go` serializes `tools/list` for each mode over an in-memory MCP session and fails when the estimate exceeds the mode's budget.

| Env var | Effect |
| --- | --- |
| `APP_MCP_TOKEN_BUDGET_READ_ONLY` | Override the `read-only` ceiling (estimated tokens). |
| `APP_MCP_TOKEN_BUDGET_STANDARD` | Override the `standard` ceiling. |
| `APP_MCP_TOKEN_BUDGET_FULL` | Override the `full` ceiling. |

Run it with the report of the heaviest tools:

```bash
scripts/mcp-token-budget.sh
```
