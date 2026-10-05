---
name: levelrail
description: Deploy and operate apps on a self-hosted Levelrail instance with levelrail-cli or the levelrail-mcp server. Use when asked to deploy, redeploy, roll back, read logs, diagnose a failed deploy, set env vars or secrets, check app status, or connect an agent to a Levelrail control plane.
---

# Levelrail

Levelrail is a self-hosted deployment platform. You drive it with the CLI (`levelrail-cli`) or the MCP server (`levelrail-mcp`). Both read the API token from `APP_API_TOKEN` and the instance URL from `APP_API_URL`. Never write a token into a file, a commit, or a chat message.

## Set up

1. Install the CLI: `curl -fsSL https://levelrail.com/install-cli.sh | sh` (macOS and Linux, installs to `~/.local/bin`).
2. Point it at the instance and log in: `export APP_API_URL=https://console.example.com`, then `levelrail-cli auth login --device`. The user approves the code in the dashboard. In CI, use an API token instead.
3. In a project, run `levelrail-cli init`. It writes `app.yaml`, an `AGENTS.md` with this project's deploy instructions, and an `.mcp.json` entry for `levelrail-mcp`. Add `--dry-run` to preview.

For MCP, pick the smallest mode that does the job: `agent-core` (default), `read-only`, `standard`, or `full`.

## Deploy and wait

1. Commit and push. A push to the tracked branch deploys through the webhook. To deploy by hand: `levelrail-cli apps deploy <app>`.
2. Preview a spec change without applying it: `levelrail-cli apps preflight <app>` (MCP: `preflight_app`, `plan_change`).
3. Do not assume a deploy worked. Block until it converges: `levelrail-cli apps wait <app>`. The exit code says whether it succeeded (MCP: `wait_for_deploy`).

## Check status and read a failure

```
levelrail-cli apps status <app>
levelrail-cli attention
levelrail-cli apps deploys list <app>
levelrail-cli apps deploys show <app> <deploy-id>
levelrail-cli apps diagnose <app>
levelrail-cli apps logs <app> --follow
```

`apps deploys show` returns the structured failure (stage, reason, suggested fix). Read it before changing anything, and change one thing per attempt. Add `--json` for machine-readable output.

## Roll back

```
levelrail-cli apps deploys list <app>
levelrail-cli apps deploys rollback-to <app> <deploy-id>
```

Roll back first when production is down, then investigate.

## Env vars and secrets

- Plain config: `levelrail-cli apps env import <app> --file .env --dry-run`, then again without `--dry-run`.
- Secrets: put them in a git-ignored file and run `levelrail-cli apps secrets set <app> --env-file <path>`, so values never appear in shell history or chat. Do not read secret values back or print them.
- Env changes take effect on the next restart: `levelrail-cli apps apply <app>`.

## Rules

- Use `--dry-run` or `plan_change` before anything destructive.
- Stay inside the app you were asked about. Do not delete apps, databases, or backups unless asked.
- If a command is denied, the token lacks that ability. Report it instead of working around it.

## More

Full documentation for models: https://levelrail.com/llms.txt (index) and https://levelrail.com/llms-full.txt (full text). Agent setup details: https://levelrail.com/agents.
