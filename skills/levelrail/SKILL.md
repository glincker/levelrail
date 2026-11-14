---
name: levelrail
description: Entry point for working with a self-hosted Levelrail instance through levelrail-cli or the levelrail-mcp server. Use for first-time setup, logging in, connecting an agent over MCP, or finding the right command. For deploying, diagnosing and rolling back, or env vars and secrets, use the focused levelrail-deploy, levelrail-diagnose and levelrail-config skills.
---

# Levelrail

Levelrail is a self-hosted deployment platform. You drive it with the CLI (`levelrail-cli`) or the MCP server (`levelrail-mcp`). Both read the API token from `APP_API_TOKEN` and the instance URL from `APP_API_URL`. Never write a token into a file, a commit, or a chat message.

## Set up

1. Install the CLI: `curl -fsSL https://levelrail.com/install-cli.sh | sh` (macOS and Linux, installs to `~/.local/bin`). Or with Node: `npx levelrail-cli <command>` / `npm install --global levelrail-cli` (client only, never the server).
2. Point it at the instance and log in: `export APP_API_URL=https://console.example.com`, then `levelrail-cli auth login --device`. The user approves the code in the dashboard. In CI, use an API token instead.
3. In a project, run `levelrail-cli init`. It writes `app.yaml`, an `AGENTS.md` with this project's deploy instructions, and an `.mcp.json` entry for `levelrail-mcp`. Add `--dry-run` to preview.
4. To see what a project is before deploying, run `levelrail-cli apps detect --json`. It reads the directory offline.

For MCP, pick the smallest mode that does the job: `agent-core` (default), `read-only`, `standard`, or `full`.

## Which skill

- Ship or redeploy an app: `levelrail-deploy`.
- A deploy failed, an app is unhealthy, or production is down: `levelrail-diagnose`.
- Env vars and secrets: `levelrail-config`.

## Find a command

Every command and its help text is in [the generated command index](references/commands/INDEX.md). It is produced by `levelrail-cli docs`, so it matches the installed CLI.

## Rules

- Use `--dry-run` or `plan_change` before anything destructive.
- Stay inside the app you were asked about. Do not delete apps, databases, or backups unless asked.
- If a command is denied, the token lacks that ability. Report it instead of working around it.

## More

Full documentation for models: https://docs.levelrail.com/llms.txt (index) and https://docs.levelrail.com/llms-full.txt (full text). Agent setup details: https://docs.levelrail.com/agents.
