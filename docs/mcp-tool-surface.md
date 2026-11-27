---
title: "MCP tool surface"
description: "Estimated model context cost of the Levelrail MCP tool list, by toolset, so you can choose which toolsets to enable."
---

Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.

Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.

## By toolset

| Toolset | Tools | Est. tokens |
| --- | ---: | ---: |
| account | 5 | 315 |
| alerts | 12 | 1141 |
| apps | 19 | 2272 |
| audit | 2 | 253 |
| backups | 14 | 992 |
| builds | 5 | 454 |
| canary | 4 | 450 |
| dashboards | 2 | 133 |
| databases | 11 | 743 |
| deploys | 25 | 2957 |
| diagnostics | 5 | 470 |
| domains | 12 | 852 |
| environments | 3 | 351 |
| flags | 2 | 139 |
| functions | 1 | 92 |
| global-environments | 2 | 182 |
| iac | 2 | 652 |
| iam | 8 | 515 |
| identity | 3 | 104 |
| images | 3 | 347 |
| imports | 1 | 183 |
| inbox | 1 | 50 |
| loadbalancer | 9 | 1110 |
| logs | 13 | 1478 |
| metrics | 11 | 836 |
| migrate | 8 | 489 |
| models | 16 | 1858 |
| nodes | 7 | 540 |
| notifications | 3 | 213 |
| orgs | 2 | 81 |
| pipelines | 3 | 371 |
| plugins | 3 | 158 |
| previews | 2 | 114 |
| registry | 3 | 206 |
| scheduled | 4 | 279 |
| search | 2 | 233 |
| security | 3 | 142 |
| settings | 3 | 140 |
| stacks | 4 | 438 |
| storage | 5 | 549 |
| system | 13 | 653 |
| teams | 2 | 88 |
| templates | 2 | 118 |
| webhooks | 1 | 124 |
| **total** | **261** | **23865** |

## Default session (progressive disclosure)

`levelrail-mcp` lists only a small core plus `list_toolsets`, `enable_toolset` and `disable_toolset`. Enabling a toolset adds its tools for the rest of the session and sends `tools/list_changed`; the table above is what each toolset costs. `--all` (or `APP_MCP_ALL=1`), `--toolsets` and `--tool-profile` keep the static listing for clients without `list_changed` support. The mode and the token's abilities bound every call either way.

21 tools, 1924 estimated tokens (ratchet 3150, ceiling per toolset 4000).

- `cancel_deploy`
- `deploy_app`
- `diagnose_app_failure`
- `disable_toolset`
- `enable_toolset`
- `get_app`
- `get_app_env`
- `get_app_status`
- `get_attention`
- `list_apps`
- `list_deploys`
- `list_toolsets`
- `preflight_app`
- `query_logs`
- `rollback_app`
- `search`
- `set_app_domains`
- `set_app_env`
- `unset_app_env`
- `wait_for_deploy`
- `whoami`

## agent-core profile

Select with `--tool-profile agent-core` or `APP_MCP_TOOL_PROFILE=agent-core`. It is an allowlist independent of the read-only, standard and full modes; the API token's abilities still apply.

15 tools, 1577 estimated tokens (cap 4000).

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
