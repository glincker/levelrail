---
title: Working with AI agents
description: Connect an AI coding agent to your instance, scope its token, and give it the instructions it needs to deploy safely.
---

# Working with AI agents

An agent talks to the platform the same way you do: through the CLI (`levelrail-cli`) or the MCP server (`levelrail-mcp`). Everything an agent does goes through the normal API with the token you give it, so its access is exactly that token's abilities, and every change lands in the audit log under the agent's name.

## Set up a project with `init`

Run this in the project root:

```
levelrail-cli init
```

It detects the stack and writes three files:

| File | What it is |
| --- | --- |
| `app.yaml` | The app spec, validated with the same parser the control plane uses before it is written. |
| `AGENTS.md` | Instructions for an agent: deploy, wait, read a failure, roll back, set env and secrets safely. Uses the CLI name you ran and your API URL when the CLI knows it. |
| `.mcp.json` | A stdio MCP server entry. The token is an env var reference (`${APP_API_TOKEN}`), never a value. |

Detection order: Dockerfile, compose file, `package.json` (Next.js, NestJS, Express, Fastify, Vite), `go.mod`, Java (`pom.xml`, Gradle), Python (`requirements.txt`, `pyproject.toml`, `Pipfile`), then a plain `index.html` static site. Provider names match the build system's own, so what `init` reports lines up with what a deploy will detect. Nothing from the project is executed.

Flags:

- `--dry-run` shows the plan and changes nothing.
- `--force` overwrites files that already exist. Without it existing files are skipped, and the diff of what would change is shown.
- `--yes` skips the confirmation prompt. A script without a terminal must pass `--yes` or `--dry-run`.
- `--json` prints the plan and result as one JSON value.
- `--mode` picks the tool exposure written to `.mcp.json`: `agent-core` (default), `read-only`, `standard` or `full`.
- `--dir`, `--api-url`, `--profile` and `--mcp-binary` override the directory, the URL written into the files, the credentials profile and the MCP binary name.

## Modes

The MCP server exposes a subset of its tools depending on the mode. Token counts are estimates of the `tools/list` payload the client loads into context.

| Mode | Tools | Estimated tokens | Use it for |
| --- | --- | --- | --- |
| `agent-core` (a tool profile, `--tool-profile agent-core`) | about 15 | about 2,500 | Shipping and debugging one app. The default for `init`. |
| `read-only` | 109 | 41,600 | Observers that must not change anything. |
| `standard` | 135 | 55,600 | Read and mutating tools, no destructive ones. |
| `full` | 144 | 59,800 | Every tool, including destructive ones. |

See [Agent tooling audit](agent-tooling-audit.md) for how the numbers are measured.

## Tokens

Give each agent its own token so its actions are attributable and it can be revoked alone.

```
levelrail-cli tokens create --name ci-agent --abilities read,deploy --agent "Claude Code"
```

The `--agent` label is shown on the tokens page and recorded on every audit entry the token makes. Filter the audit log by it with `audit-log --agent NAME` or the agent chip on the audit log page. Settings > Agents in the dashboard has the same flow with three presets:

| Preset | Abilities | Can do |
| --- | --- | --- |
| Read-only observer | `read` | Look at apps, logs, metrics and deploys. |
| Deployer | `read`, `deploy` | Also trigger deploys and rollbacks. |
| Full operator | `read`, `write`, `deploy` | Also change config, env and domains. |

The token is shown once, when it is created. Store it in your shell profile or secret manager as `APP_API_TOKEN`, not in the repo.

## Dry run before changing anything

- `plan_change` (MCP) previews a change to the declared resources without applying it, and `plan_apply` does the same for resource files. `levelrail-cli apply --dry-run` is the CLI form.
- `levelrail-cli apps preflight NAME` checks DNS, ports, disk, the image and required env before a deploy.

## Wait for a deploy

Agents should never assume a deploy worked. `wait_for_deploy` (MCP) or `levelrail-cli apps wait NAME` blocks until the deploy converges and reports success or the structured failure. On failure, read it with `levelrail-cli apps deploys show NAME DEPLOY_ID`, fix one thing, and retry. Roll back first if production is down.
