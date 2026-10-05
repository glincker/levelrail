# Deploying this project

This project deploys to your Acme instance. Use the CLI (acme-cli) or the MCP server (acme-mcp, configured in .mcp.json, mode agent-core). Both read the API token from `APP_API_TOKEN`. Never write a token into a file, a commit or a chat message.

The app spec is app.yaml in this directory. Validate a change before you ship it.

## Deploy

1. Edit app.yaml or the code, commit, and push. A push to the tracked branch deploys through the webhook.
2. To deploy by hand: `acme-cli apps deploy my-app`.
3. Preview a spec change without applying it: `acme-cli apps preflight my-app` (MCP: `preflight_app`, `plan_change`).

## Wait for the result

Do not assume a deploy worked. Block until it converges:

```
acme-cli apps wait my-app
```

The exit code says whether it succeeded. MCP: `wait_for_deploy`.

## Check status

```
acme-cli apps status my-app
acme-cli attention
```

## Read a failure

```
acme-cli apps deploys list my-app
acme-cli apps deploys show my-app <deploy-id>
acme-cli apps diagnose my-app
acme-cli apps logs my-app --follow
```

`apps deploys show` returns the structured failure (stage, reason, suggested fix). Read it before changing anything, and change one thing per attempt. Add `--json` for machine-readable output.

## Roll back

```
acme-cli apps deploys list my-app
acme-cli apps deploys rollback-to my-app <deploy-id>
```

Roll back first when production is down, then investigate.

## Env vars and secrets

- Plain config: `acme-cli apps env import my-app --file .env --dry-run`, then without `--dry-run`.
- Secrets: put them in a git-ignored file and run `acme-cli apps secrets set my-app --env-file <path>`, so values never appear in shell history or chat. Do not read secret values back or print them.
- Env changes take effect on the next restart: `acme-cli apps apply my-app`.

## Rules for agents

- Use `--dry-run` or `plan_change` before anything destructive.
- Stay inside this app. Do not delete apps, databases or backups without being asked.
- If a command is denied, the token lacks that ability: report it instead of working around it.
