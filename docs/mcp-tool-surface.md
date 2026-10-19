---
description: "Estimated model context cost of the MCP tool list, by toolset."
---

# MCP tool surface

Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.

Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.

## By toolset

| Toolset | Tools | Est. tokens |
| --- | ---: | ---: |
| alerts | 10 | 1259 |
| apps | 16 | 2103 |
| audit | 1 | 306 |
| backups | 13 | 1134 |
| builds | 5 | 477 |
| dashboards | 2 | 133 |
| databases | 10 | 803 |
| deploys | 30 | 4570 |
| diagnostics | 5 | 595 |
| domains | 12 | 1150 |
| environments | 3 | 541 |
| flags | 2 | 139 |
| functions | 1 | 99 |
| global-environments | 2 | 203 |
| iac | 2 | 800 |
| iam | 7 | 532 |
| images | 3 | 368 |
| loadbalancer | 9 | 1336 |
| logs | 13 | 1786 |
| metrics | 9 | 1042 |
| models | 15 | 2222 |
| nodes | 6 | 464 |
| notifications | 3 | 287 |
| orgs | 2 | 145 |
| pipelines | 3 | 432 |
| plugins | 2 | 172 |
| previews | 2 | 202 |
| registry | 3 | 309 |
| scheduled | 2 | 171 |
| search | 2 | 234 |
| settings | 3 | 208 |
| storage | 5 | 558 |
| system | 12 | 735 |
| teams | 2 | 88 |
| templates | 2 | 126 |
| webhooks | 1 | 145 |
| **total** | **220** | **25874** |

## agent-core profile

Select with `--tool-profile agent-core` or `APP_MCP_TOOL_PROFILE=agent-core`. It is an allowlist independent of the read-only, standard and full modes; the API token's abilities still apply.

15 tools, 1984 estimated tokens (cap 4000).

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
