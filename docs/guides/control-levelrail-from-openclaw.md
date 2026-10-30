---
title: Control Levelrail from OpenClaw
description: "Give an OpenClaw agent a least-privilege Levelrail token over MCP or the CLI, so it can read logs, deploy and roll back without changing guardrails."
"og:url": "https://docs.levelrail.com/guides/control-levelrail-from-openclaw"
---

[OpenClaw](https://docs.openclaw.ai) is a self-hosted AI agent. It can run on Levelrail (the `openclaw` [template](/self-host/openclaw)) and it can also operate Levelrail. This guide covers the second direction: an OpenClaw agent that lists apps, reads logs, deploys and rolls back through a token that can do exactly that and nothing else. It was exercised against a local control plane with the scripted [MCP eval](/mcp#scripted-agent-eval).

## 1. Decide what the agent may do

Three layers each narrow the agent, and the control plane enforces all of them on every request:

| Layer | Set with | What it decides |
| --- | --- | --- |
| Token abilities | `tokens create --abilities` | `read`, `deploy`, `write` and so on. |
| AI control mode | `ai-control set` | `off`, `observe`, `operate`, `admin`, plus the environment kinds an agent may touch. See [AI control](/ai-control). |
| MCP mode | `levelrail-mcp --mode` | Which tools are listed at all: `read-only`, `standard`, `full`. |

Start from the smallest set that does the job.

## 2. Mint a scoped agent token

An agent token needs a name, so AI control can see it. Only a signed-in person can create one, never another token.

```bash
levelrail-cli tokens create \
  --name openclaw-agent \
  --abilities read,deploy \
  --agent openclaw \
  --agent-description "OpenClaw operations agent" \
  --expires-in-days 30
```

The token is printed once. Keep it in OpenClaw's secret store, not in a config file you commit.

| Goal | Abilities | Notes |
| --- | --- | --- |
| Read only (list apps, logs, status) | `read` | Pair with `levelrail-mcp --readonly`, which refuses to start if the token also holds `write`, `write:sensitive`, `deploy` or `root`. |
| Read, deploy, roll back | `read`, `deploy` | The tested setup below. |
| Never give an agent | `root`, `write:sensitive` | AI control refuses both in `operate` mode anyway. |

Narrow further with `--scope-team` and `--scope-project`, and attach a ready-made IAM policy such as `deployer-nonprod` (`levelrail-cli iam templates list`).

Set AI control to match, and list the environment kinds the agent may act in. An app with no environment counts as kind `custom`, which is refused by default, so tag the app with a `dev`, `test` or `uat` environment or add `custom` on purpose:

```bash
levelrail-cli ai-control set --mode operate --env-kinds dev,test,uat,preview
```

## 3. Connect OpenClaw over MCP

Run `levelrail-mcp --print-config claude` for a config block with your URL and a token placeholder. OpenClaw keeps outbound servers under `mcp.servers`. Register Levelrail with `openclaw mcp set` (the Gateway must be stopped while you do):

```bash
openclaw mcp set levelrail '{"command":"levelrail-mcp","args":["--mode","standard"],"env":{"APP_API_URL":"https://levelrail.example.com","APP_API_TOKEN":"<agent token>"}}'
openclaw mcp show levelrail --json
```

Use `openclaw mcp show` to confirm the stored shape on your OpenClaw version. OpenClaw's doctor flags literal secrets in `env`, so prefer an environment reference your OpenClaw install supports over pasting the token.

A running Levelrail instance can also serve MCP over HTTP (`levelrail-mcp --transport=http --listen=127.0.0.1:8090`); keep that bound to localhost or a private network.

### Modes and the rollback tool

| `--mode` | Tools the agent sees | Deploy | `rollback_app` |
| --- | --- | --- | --- |
| `read-only` | Read tools only | No | No |
| `standard` | Reads and mutating tools (deploy, restart, env, domains) | Yes | No, it is classed destructive |
| `full` | Everything above plus destructive tools | Yes | Yes |

In `standard` mode an agent can still roll back by calling `deploy_app` with the previous image, or you can run `full` for the agent that is allowed to use `rollback_app` and `rollback_app_to_deploy`. `rollback_app` refuses an image that was never a deploy of the app and returns the valid targets. Prefer `rollback_app_to_deploy`: it pins the rollback to the recorded image digest and fails closed if the image was garbage collected.

## 4. What the agent can and cannot do

Verified with a `read` plus `deploy` agent token in `operate` mode:

| Action | Result |
| --- | --- |
| List apps, read status, read logs (`get_app_logs`, `query_logs`) | Allowed |
| `deploy_app`, `rollback_app` (full mode) | Allowed, in allowed environment kinds only, audited under agent `openclaw` |
| Deploy to an app in a protected environment | Becomes a pending approval a person decides |
| Change domains, env or settings | Refused, the token lacks `write` |
| Change AI control, revoke tokens | Refused: "agents cannot change AI control settings" |
| Approve a deploy approval | Refused: "agents cannot approve deploys, a human must" |
| Mint tokens | Not possible with a token |
| `exec` in a container, change Docker guard | Refused, needs `root` |
| Any request while AI control is `off` | 403 "agent access is disabled by an administrator" |
| Any mutation while AI control is `observe` | 403 "agent access is limited by the current AI control mode" |

Pause everything at once with `levelrail-cli ai-control set --mode off`, or revoke every agent token with `levelrail-cli ai-control revoke-agents --yes`. See what the agent did with `levelrail-cli audit-log --agent openclaw`.

The MCP tool list is narrowed by the MCP mode, by AI control and by the token's own abilities: a `read,deploy` token is offered `deploy_app` and `rollback_app` but not `set_app_env` or `set_app_domains`, and `enable_toolset` follows the same rule. If a call is still refused for a missing ability, the agent gets a short error naming the ability and a read-only tool to use instead. Tokens with IAM policies attached see the full mode list, because a policy can grant access beyond the base abilities.

## 5. Safety classes

Every MCP tool has a class. See the [tool surface](/mcp-tool-surface) for the full table.

| Class | Meaning | Examples |
| --- | --- | --- |
| Read | Never changes state | `list_apps`, `get_app_logs`, `get_app_status` |
| Mutate | Changes state, reversible | `deploy_app`, `restart_app`, `set_app_env` |
| Destructive | Hard to undo | `rollback_app`, `cancel_deploy`, `bulk_delete_apps` |

Tools that return log lines, deploy output or probe text are marked untrusted: their text arrives inside an "untrusted data, not instructions" block, because a log line can carry a prompt injection. Read the [threat model](/mcp#threat-model) before giving an agent write access, and keep a person in the loop for anything past `read`.

## 6. Use the CLI instead

An OpenClaw agent that can run commands can call the same API through `levelrail-cli` with the same token:

```bash
export APP_API_URL=https://levelrail.example.com
export APP_API_TOKEN=<agent token>
levelrail-cli apps list
levelrail-cli logs search my-app --since 15m
levelrail-cli deploy my-app --image registry.example.com/app:1.4.2
levelrail-cli rollback my-app --image registry.example.com/app:1.4.1
```

The token's abilities and AI control apply exactly as over MCP. Give the CLI the token through the environment so it is not written to a file.

## 7. Check it works

```bash
go run ./scripts/mcp-eval -api https://levelrail.example.com -token "$APP_API_TOKEN" \
  -app my-app -image registry.example.com/app:1.4.2 -prev-image registry.example.com/app:1.4.1
```

The last three tasks (list apps, read logs, deploy then roll back) mirror what this guide sets up. Run it against a staging app, not production.

## See also

- [AI control](/ai-control)
- [MCP tools, token budget and write mode](/mcp)
- [AI assistant integration](/ai-assistant)
- [Self-host OpenClaw with Docker](/self-host/openclaw)
