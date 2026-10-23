---
description: "Estimated model context cost of the MCP tool list, by toolset."
---

# MCP tool surface

Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.

Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.

## By toolset

| Toolset | Tools | Est. tokens |
| --- | ---: | ---: |
| account | 5 | 351 |
| alerts | 10 | 1125 |
| apps | 16 | 1901 |
| audit | 2 | 256 |
| backups | 14 | 1134 |
| builds | 5 | 469 |
| canary | 4 | 501 |
| dashboards | 2 | 133 |
| databases | 11 | 768 |
| deploys | 23 | 2972 |
| diagnostics | 5 | 496 |
| domains | 12 | 952 |
| environments | 3 | 417 |
| flags | 2 | 139 |
| functions | 1 | 97 |
| global-environments | 2 | 203 |
| iac | 2 | 739 |
| iam | 8 | 561 |
| identity | 3 | 104 |
| images | 3 | 365 |
| imports | 1 | 186 |
| inbox | 1 | 50 |
| loadbalancer | 9 | 1234 |
| logs | 13 | 1659 |
| metrics | 10 | 877 |
| migrate | 8 | 489 |
| models | 16 | 2013 |
| nodes | 7 | 595 |
| notifications | 3 | 225 |
| orgs | 2 | 106 |
| pipelines | 3 | 401 |
| plugins | 3 | 171 |
| previews | 2 | 163 |
| registry | 3 | 243 |
| scheduled | 4 | 331 |
| search | 2 | 237 |
| security | 3 | 142 |
| settings | 3 | 188 |
| storage | 5 | 558 |
| system | 13 | 735 |
| teams | 2 | 88 |
| templates | 2 | 126 |
| webhooks | 1 | 145 |
| **total** | **249** | **24645** |

## Default session (progressive disclosure)

`levelrail-mcp` lists only a small core plus `list_toolsets`, `enable_toolset` and `disable_toolset`. Enabling a toolset adds its tools for the rest of the session and sends `tools/list_changed`; the table above is what each toolset costs. `--all` (or `APP_MCP_ALL=1`), `--toolsets` and `--tool-profile` keep the static listing for clients without `list_changed` support. The mode and the token's abilities bound every call either way.

21 tools, 2222 estimated tokens (ratchet 3150, ceiling per toolset 4000).

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

15 tools, 1822 estimated tokens (cap 4000).

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
