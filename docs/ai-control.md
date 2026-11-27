---
title: "AI control"
description: Set what AI agents and agent tokens may do on your instance, from off to admin, with a server-side limit by environment kind, plus pause and revoke controls.
"og:url": "https://docs.levelrail.com/ai-control"
---

Use AI control to decide what agents may do on this instance, and to pause or revoke them in one step. The control plane enforces it on every request, so it does not depend on the agent, the MCP client or the token's own abilities. The page is always visible because it is the guardrail.

<CardGroup cols={3}>
<Card title="Use it when" icon="shield">

An AI client, MCP server or automation holds an API token.

</Card>
<Card title="Default" icon="power">

A fresh instance starts in `off`. Upgraded instances with live agent tokens start in `operate`.

</Card>
<Card title="Where" icon="gear">

Settings, AI control, or `levelrail-cli ai-control`.

</Card>
</CardGroup>

## What counts as an agent?

An agent is an API token that has an agent name, including the short-lived token the in-app assistant mints for each chat turn (agent name `ai-assistant`). That turn token is owned by the person who sent the message and carries only their abilities and IAM policies, so the mode below narrows what the assistant may do on their behalf, never widens it. Agent tokens can only be created by a signed-in person, never by another token.

AI control does not govern:

- people signed in with a session,
- API tokens that have no agent name (they are limited only by their abilities and IAM policies).

<Warning>
Give every automation or AI client token an agent name when you create it. Otherwise this switch cannot see it.
</Warning>

## Which mode should I pick?

| Mode | What an agent may do |
| --- | --- |
| `off` | Nothing. Every request answers 403 "agent access is disabled by an administrator". |
| `observe` | The `read` ability only, in any environment. No secrets, no changes. |
| `operate` | `read`, `read:sensitive`, `write` and `deploy`. Never `root`, never `write:sensitive` (secrets, backups, instance settings). On an app or database, only if its environment kind is allowed. |
| `admin` | Any ability the token holds, with the same environment limit. Needs `APP_EXPERIMENTAL=ai-control`. Without the flag it behaves as `operate`. |

Rules that hold in `operate` and `admin`:

- An agent can never approve a deploy approval, a pipeline approval or a preview approval. A person must.
- An agent can never change AI control settings or revoke tokens.
- Moving or deploying into a protected environment still creates an approval that a person decides.

The environment limit is a list of kinds: `dev`, `test`, `uat`, `production`, `preview`, `custom`. The default allowed list is `dev`, `test`, `uat` and `preview`. A resource with no environment counts as kind `custom`, which is not allowed by default.

## How do I set the mode?

<Tabs>
<Tab title="Dashboard">

Go to Settings, AI control. Pick a mode and the allowed kinds. **Pause all agents** sets the mode to `off` in one click.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli ai-control status
levelrail-cli ai-control set --mode operate --env-kinds dev,test,uat,preview
```

`--env-kinds` is optional: leave it out and the current list is kept. `--mode` is required.

To let agents act on resources with no environment, add `custom`:

```bash
levelrail-cli ai-control set --mode operate --env-kinds dev,test,uat,preview,custom
```

</Tab>
</Tabs>

## How do I revoke every agent token?

```bash
levelrail-cli ai-control revoke-agents --yes
```

This revokes every token that has an agent name and reports how many. The in-app assistant's per-turn tokens are skipped: each one is already revoked when its turn ends. Set the mode to `off` to stop the assistant instead.

## Defaults and upgrades

- A fresh instance starts in `off`. The in-app assistant needs a mode set before it works.
- An instance that already had live agent tokens when it was upgraded starts in `operate`, so existing agents keep working.

## How do I see what an agent did?

Every request an agent makes that this switch refuses is written to the audit log with the agent name and the reason. Changes to the setting and token revocations are audited too. Requests from the in-app assistant also record `on_behalf_of`, the person whose chat turn made them. To see one agent:

```bash
levelrail-cli audit-log --agent my-agent
```

The AI control page also links to the audit log filtered by agent.

## How does MCP react to the mode?

The MCP server reads the mode each time a client asks for its tool list, and hides what the mode would refuse, but only when its own token is an agent: `off` shows only `ai_control_status`, `observe` shows read tools, `operate` and `admin` show everything. A token with no agent name is not governed by this switch, so its tool list is never trimmed. This only tidies the list. The control plane enforces the mode whatever the client does.

## What do the agent error messages mean?

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

`GET` also reports `caller_is_agent`, whether the token making the request is an agent. `PUT` takes `{"mode": "...", "allowed_env_kinds": [...]}` and answers 400 for `admin` unless the `ai-control` flag is on.

## Next steps

<CardGroup cols={2}>
<Card title="AI assistant integration" href="/ai-assistant">

Scope an API token for an MCP client or an AI agent.

</Card>
<Card title="Working with AI agents" href="/agents">

Build an agent that deploys and monitors your apps.

</Card>
</CardGroup>
