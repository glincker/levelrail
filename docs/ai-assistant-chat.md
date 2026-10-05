---
description: The in-app AI assistant chat, how to enable it behind the ai-chat experimental flag, and the read-and-suggest confirmation gate that bounds what it can do.
---

# In-app AI assistant chat

A chat panel in the dashboard, backed by the control plane's own `internal/ai` engine. It reads your apps, deploys, logs, metrics and diagnostics through the same tool surface the MCP server exposes, and it can propose actions, but **every action that would change anything pauses for an explicit confirmation click first**. AI in Levelrail is a read-and-suggest layer on top of the API: this is the same engine and the same confirmation gate described in [AI assistant integration](ai-assistant.md#untrusted-text-and-the-assistant-s-confirmation-gate), surfaced as a dashboard panel and a CLI command.

This is separate from `levelrail-mcp`. To point an external MCP client (Claude Desktop, Claude Code, your own) at this control plane, read [AI assistant integration](ai-assistant.md) instead.

Source: `internal/ai`, `internal/api/ai_chat.go`, `internal/api/ai_settings.go`, `web/src/components/AiChatPanel.tsx`, `cmd/levelrail-cli/ai.go`.

## Enabling it

Two things are required.

<Steps>
<Step title="Turn on the experimental flag">

The chat is gated behind `ai-chat` (see [Experimental features](experimental-features.md)). Set this on the control plane:

```bash
APP_EXPERIMENTAL=ai-chat
```

With the flag off (the default), every route under `/api/v1/ai/sessions` and `/api/v1/settings/ai-assistant` answers `404`, the dashboard hides the **AI assistant** nav entry and the `/ai-assistant` page, and the CLI hides `ai chat` and `ai sessions` from `--help` and completion.

</Step>
<Step title="Configure a provider key">

Nothing runs until you bring your own key. Only `anthropic` is supported today. The key is stored with the same envelope encryption as other secrets and is never returned or logged.

<Tabs :items="['CLI', 'Dashboard']">
<Tab value="CLI">

```bash
levelrail-cli settings ai-assistant set --model claude-sonnet-4-5 --api-key sk-ant-...
```

`levelrail-cli settings ai-assistant get` reports only whether a key is configured.

</Tab>
<Tab value="Dashboard">

Open **Settings → AI assistant** and fill in the form.

</Tab>
</Tabs>

</Step>
</Steps>

With both set, the **AI assistant** page appears in the dashboard and `levelrail-cli ai chat` stops returning the experimental-disabled error.

## What it can do

- **Read, always.** Logs, metrics, deploy history, node status, certificates, and crashloop diagnosis, through the read-only tool classification described under [Modes and toolsets](ai-assistant.md#modes-and-toolsets). These run automatically, with no confirmation, the moment the model asks for them.
- **Propose, never execute directly.** A mutating tool call (deploy, rollback, restart, an env change, and so on) is recorded as a pending confirmation and streamed to the UI as a `tool_call_proposed` event. It does not run until a human clicks **Approve** (dashboard) or runs `levelrail-cli ai sessions resolve <session> <confirmation-id> --approve` (CLI). Rejecting it is just as explicit a path (`--reject`), and the model sees "User declined to run this action" as the result either way, so it can explain and move on rather than retry blindly.
- **Never in the reconciliation path.** The assistant only calls the platform's own REST API (`internal/apiclient`), the same path any external MCP client or the CLI uses. It has no privileged entry point into the reconciler, the Docker client, or the database.

Untrusted-text handling and the confirmation gate's exact rules (what counts as tainted, which annotations make a tool auto-run) are documented once, under [Untrusted text and the assistant's confirmation gate](ai-assistant.md#untrusted-text-and-the-assistant-s-confirmation-gate).

## Using it

### Dashboard

**AI assistant** in the global nav (hidden unless the flag is on) opens a chat panel: a message composer, a transcript with the assistant's replies rendered as markdown (code blocks, lists, links; raw HTML and images are stripped, since a reply can echo untrusted tool output), and a confirmation card with Approve/Reject buttons for any pending mutating tool call. **History** lists past sessions so you can resume or delete one; **New chat** starts a fresh one.

### CLI

```bash
# One-shot: start a session, send a message, stream the reply to stdout.
levelrail-cli ai chat "why did the last deploy of web fail?"

# Continue the same conversation (the first run printed its session id to stderr).
levelrail-cli ai chat "roll it back then" --session aisess_abc123

# A mutating call proposed above pauses; approve or reject it explicitly.
levelrail-cli ai sessions resolve aisess_abc123 aiconf_xyz789 --approve

# Manage sessions.
levelrail-cli ai sessions list
levelrail-cli ai sessions get aisess_abc123
levelrail-cli ai sessions delete aisess_abc123
```

`--json` on any `ai` subcommand switches to one JSON object per stream event (or the plain resource for `sessions list`/`get`/`delete`), the same `--json`/`--output`/`--query` convention every other `levelrail-cli` command follows.

### REST API

`POST /api/v1/ai/sessions`, `GET /api/v1/ai/sessions`, `GET`/`DELETE /api/v1/ai/sessions/{id}`, `POST /api/v1/ai/sessions/{id}/messages` (Server-Sent Events: `text_delta`, `tool_call_proposed`, `tool_result`, `done`), and `POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}` (same SSE framing, for resolving a pending call). All of it requires `AbilityRoot`: a confirmed message can execute any mutating tool the platform exposes once approved, the same blast radius a root-scoped token would need to reach those actions directly, so this is not a surface to hand a lower-privileged token.

## Scope and limits

- **Single admin user, no per-user session isolation beyond the existing root-only gate.** Anyone who can authenticate as root can see and act on any session.
- **One provider.** Anthropic only; no model routing or fallback.
- **Not yet covered by real-infrastructure testing.** `test/e2e/ai_chat_test.go` proves the full session lifecycle (create, send a message with an auto-executed read tool call, get, list, delete, and the flag-off 404) over real HTTP against a real router and a real `ai.Engine`, but with a scripted fake `ai.Provider`, never a live call to Anthropic's API. See [Feature status](feature-status.md) for the current evidence tier.

## See also

- [ai-assistant.md](ai-assistant.md): `levelrail-mcp`, the standalone MCP server for an external AI client, and the confirmation/untrusted-text mechanism this chat shares with it.
- [experimental-features.md](experimental-features.md): the `APP_EXPERIMENTAL` switch and what "off" means everywhere.
- [identity-and-access.md](identity-and-access.md): API tokens and abilities (`ai` CLI commands and `/api/v1/ai/...` routes need `root`).
- [cli-reference.md](cli-reference.md): full flag reference for `ai chat` and `ai sessions`.
