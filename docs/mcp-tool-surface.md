# MCP tool surface

Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.

Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.

## By toolset

| Toolset | Tools | Est. tokens |
| --- | ---: | ---: |
| alerts | 10 | 1265 |
| apps | 15 | 1975 |
| audit | 1 | 259 |
| backups | 11 | 1031 |
| databases | 3 | 167 |
| deploys | 20 | 3114 |
| diagnostics | 3 | 328 |
| domains | 10 | 1145 |
| environments | 3 | 556 |
| flags | 2 | 165 |
| iac | 2 | 800 |
| iam | 2 | 149 |
| loadbalancer | 9 | 1347 |
| logs | 12 | 1663 |
| metrics | 4 | 626 |
| models | 13 | 1644 |
| nodes | 4 | 315 |
| notifications | 3 | 324 |
| orgs | 2 | 170 |
| pipelines | 3 | 457 |
| previews | 2 | 214 |
| registry | 3 | 325 |
| scheduled | 2 | 200 |
| settings | 2 | 187 |
| system | 4 | 284 |
| templates | 2 | 126 |
| webhooks | 1 | 155 |
| **total** | **148** | **18991** |

## agent-core profile

Select with `--tool-profile agent-core` or `APP_MCP_TOOL_PROFILE=agent-core`. It is an allowlist independent of the read-only, standard and full modes; the API token's abilities still apply.

15 tools, 1773 estimated tokens (cap 4000).

- `cancel_deploy`
- `deploy_app`
- `diagnose_app_failure`
- `get_app_env`
- `get_app_status`
- `get_attention`
- `list_apps`
- `list_deploys`
- `list_domains`
- `preflight_app`
- `query_logs`
- `rollback_app`
- `set_app_domains`
- `set_app_env`
- `unset_app_env`
