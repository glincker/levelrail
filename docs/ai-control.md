---
description: Decide what AI agents and agent tokens may do on your instance, from off to admin, enforced on the server for every request.
---

# AI control

AI control is a switch in Settings that decides what agents may do on this instance. It is enforced on the control plane for every request, so it does not depend on the agent, the MCP client or the token's own abilities. The page is always visible, because it is the guardrail.

<InlineToc default-open />

## What counts as an agent

An agent is an API token that has an **agent name**, or the built-in assistant token (agent name `ai-assistant`). Agent tokens can only be created by a signed-in person, never by another token.

AI control does **not** govern:

- people signed in with a session,
- API tokens that have no agent name (they are limited only by their abilities and IAM policies).

So give every automation or AI client token an agent name when you create it. Otherwise this switch cannot see it.

## Modes

| Mode | What an agent may do |
| --- | --- |
| `off` | Nothing. Every request answers 403 "agent access is disabled by an administrator". |
| `observe` | The `read` ability only, in any environment. No secrets, no changes. |
| `operate` | `read`, `read:sensitive`, `write` and `deploy`. Never `root`, never `write:sensitive` (secrets, backups, instance settings). On an app or database, only if its environment kind is allowed. |
| `admin` | Any ability the token holds, with the same environment limit. Needs `APP_EXPERIMENTAL=ai-control`. Without the flag it behaves as `operate`. |

In `operate` and `admin`:

- An agent can never approve a deploy approval, a pipeline approval or a preview approval. A person must.
- An agent can never change AI control settings or revoke tokens.
- Moving or deploying into a protected environment still creates an approval that a person decides.

The environment limit is a list of kinds: `dev`, `test`, `uat`, `production`, `preview`, `custom`. The default allowed list is `dev`, `test`, `uat` and `preview`. A resource with no environment counts as kind `custom`, which is not allowed by default.

## Set the mode

In the dashboard: Settings, AI control. Pick a mode and the allowed kinds. **Pause all agents** sets the mode to `off` in one click.

From the CLI:

```bash
levelrail-cli ai-control status
levelrail-cli ai-control set --mode operate --env-kinds dev,test,uat,preview
```

`--env-kinds` is optional: leave it out and the current list is kept. `--mode` is required.

To let agents act on resources with no environment, add `custom`:

```bash
levelrail-cli ai-control set --mode operate --env-kinds dev,test,uat,preview,custom
```

## Revoke agent tokens

```bash
levelrail-cli ai-control revoke-agents --yes
```

This revokes every token that has an agent name and reports how many. The built-in assistant token is not revoked, because that would break the in-app assistant. Set the mode to `off` to stop it instead.

## Defaults and upgrades

- A fresh instance starts in `off`. The in-app assistant needs a mode set before it works.
- An instance that already had live agent tokens when it was upgraded starts in `operate`, so existing agents keep working.

## Audit

Every request an agent makes that this switch refuses is written to the audit log with the agent name and the reason. Changes to the setting and token revocations are audited too. To see one agent:

```bash
levelrail-cli audit-log --agent my-agent
```

The AI control page also links to the audit log filtered by agent.

## MCP

The MCP server reads the mode each time a client asks for its tool list and hides what the mode would refuse: `off` shows only `ai_control_status`, `observe` shows read tools, `operate` and `admin` show everything. This only tidies the list. The control plane enforces the mode whatever the client does.

## Error messages

| Message | Meaning |
| --- | --- |
| `agent access is disabled by an administrator` | The mode is `off`. |
| `agent access is limited by the current AI control mode` | The mode does not allow this ability, for example a write in `observe` or `root` in `operate`. |
| `agent access is not allowed in this environment` | The resource's environment kind is not in the allowed list. |
| `agents cannot approve deploys, a human must` | Approvals always need a person. |
| `agents cannot change AI control settings` | Only a person with `root` can change the switch. |

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/settings/ai-control` | `read` |
| `PUT` | `/api/v1/settings/ai-control` | `root` |
| `POST` | `/api/v1/settings/ai-control/revoke-agent-tokens` | `root` |

`PUT` takes `{"mode": "...", "allowed_env_kinds": [...]}` and answers 400 for `admin` unless the `ai-control` flag is on.

## Next steps

<CardGroup :cols="2">
<Card title="AI assistant integration" href="/ai-assistant">

Scope an API token for an MCP client or an AI agent.

</Card>
<Card title="Working with AI agents" href="/agents">

Build an agent that deploys and monitors your apps.

</Card>
</CardGroup>
