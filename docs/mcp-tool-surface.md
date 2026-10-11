---
description: "Estimated model context cost of the MCP tool list, by toolset."
---

# MCP tool surface

Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.

Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.

## By toolset

| Toolset | Tools | Est. tokens |
| --- | ---: | ---: |
| alerts | 10 | 1265 |
| apps | 16 | 2124 |
| audit | 1 | 309 |
| backups | 13 | 1190 |
| databases | 8 | 571 |
| deploys | 26 | 3996 |
| diagnostics | 5 | 598 |
| domains | 12 | 1269 |
| environments | 3 | 556 |
| flags | 2 | 165 |
| global-environments | 2 | 206 |
| iac | 2 | 800 |
| iam | 7 | 587 |
| loadbalancer | 9 | 1347 |
| logs | 12 | 1663 |
| metrics | 5 | 759 |
| models | 15 | 2243 |
| nodes | 5 | 418 |
| notifications | 3 | 324 |
| orgs | 2 | 170 |
| pipelines | 3 | 457 |
| previews | 2 | 214 |
| registry | 3 | 325 |
| scheduled | 2 | 200 |
| settings | 3 | 236 |
| system | 9 | 602 |
| teams | 2 | 88 |
| templates | 2 | 126 |
| webhooks | 1 | 155 |
| **total** | **185** | **22963** |

## agent-core profile

Select with `--tool-profile agent-core` or `APP_MCP_TOOL_PROFILE=agent-core`. It is an allowlist independent of the read-only, standard and full modes; the API token's abilities still apply.

15 tools, 1985 estimated tokens (cap 4000).

- `cancel_deploy`
- `deploy_app`
- `diagnose_app_failure`
- `get_app_env`
- `get_app_status`
- `get_attention`
- `list_apps`
- `list_deploys`
- `preflight_app`
- `query_logs`
- `rollback_app`
- `set_app_domains`
- `set_app_env`
- `unset_app_env`
- `wait_for_deploy`
