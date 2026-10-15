---
title: "In-app AI assistant chat"
tag: "Experimental"
description: The in-app AI assistant chat, how to enable it behind the ai-chat experimental flag, and the read-and-suggest confirmation gate that bounds what it can do.
---

This is a chat panel in the dashboard, backed by the control plane's own `internal/ai` engine. It reads your apps, deploys, logs, metrics and diagnostics through the same tool surface the MCP server exposes, and it can propose actions. Every action that would change anything pauses for an explicit confirmation click first. AI in Levelrail only reads and suggests on top of the API, and this chat uses the same engine and confirmation gate described in [AI assistant integration](/ai-assistant#untrusted-text-and-the-assistants-confirmation-gate), as a dashboard panel and a CLI command.

This is separate from `levelrail-mcp`. To point an external MCP client (Claude Desktop, Claude Code, your own) at this control plane, read [AI assistant integration](/ai-assistant) instead.

Source: `internal/ai`, `internal/api/ai_chat.go`, `internal/api/ai_settings.go`, `web/src/components/AiChatPanel.tsx`, `cmd/levelrail-cli/ai.go`.

## Quick start in 3 steps

<Steps>
<Step title="Set the experimental flag">

```bash
APP_EXPERIMENTAL=ai-chat
```

Restart the control plane. The **AI assistant** page now appears in the dashboard.

</Step>
<Step title="Add your API key">

CLI: `levelrail-cli settings ai-assistant set --model claude-sonnet-4-5 --api-key sk-ant-...`

Dashboard: Settings > AI assistant, fill in the form.

</Step>
<Step title="Open the AI assistant page and start chatting">

Dashboard: **AI assistant** in the nav. Ask "why did my last deploy fail?" or "show me the errors in logs today". Approve actions if prompted.

</Step>
</Steps>

## Enabling it

Two things are required.

<Steps>
<Step title="Turn on the experimental flag">

The chat is gated behind `ai-chat` (see [Experimental features](/experimental-features)). Set this on the control plane:

```bash
APP_EXPERIMENTAL=ai-chat
```

With the flag off (the default), every route under `/api/v1/ai/sessions` and `/api/v1/settings/ai-assistant` answers `404`, the dashboard hides the **AI assistant** nav entry and the `/ai-assistant` page, and the CLI hides `ai chat` and `ai sessions` from `--help` and completion.

</Step>
<Step title="Configure a provider key">

Nothing runs until you bring your own key. Only `anthropic` is supported today. The key is stored with the same envelope encryption as other secrets and is never returned or logged.

<Tabs>
<Tab title="CLI">

```bash
levelrail-cli settings ai-assistant set --model claude-sonnet-4-5 --api-key sk-ant-...
```

`levelrail-cli settings ai-assistant get` reports only whether a key is configured.

</Tab>
<Tab title="Dashboard">

Open **Settings → AI assistant** and fill in the form.

</Tab>
</Tabs>

</Step>
</Steps>

With both set, the **AI assistant** page appears in the dashboard and `levelrail-cli ai chat` stops returning the experimental-disabled error.

## What it can do

- Read, always: logs, metrics, deploy history, node status, certificates, and crashloop diagnosis, through the read-only tool classification described under [Modes and toolsets](/ai-assistant#modes-and-toolsets). These run automatically, with no confirmation, the moment the model asks for them.
- Propose, never execute directly: a mutating tool call (deploy, rollback, restart, an env change, and so on) is recorded as a pending confirmation and streamed to the UI as a `tool_call_proposed` event. It does not run until a human clicks **Approve** (dashboard) or runs `levelrail-cli ai sessions resolve <session> <confirmation-id> --approve` (CLI). Rejecting it is just as explicit a path (`--reject`), and the model sees "User declined to run this action" as the result either way, so it can explain and move on rather than retry blindly.
- Never in the reconciliation path: the assistant only calls the platform's own REST API (`internal/apiclient`), the same path any external MCP client or the CLI uses. It has no privileged entry point into the reconciler, the Docker client, or the database.

Untrusted-text handling and the confirmation gate's exact rules (what counts as tainted, which annotations make a tool auto-run) are documented once, under [Untrusted text and the assistant's confirmation gate](/ai-assistant#untrusted-text-and-the-assistants-confirmation-gate).

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

`POST /api/v1/ai/sessions`, `GET /api/v1/ai/sessions`, `GET`/`DELETE /api/v1/ai/sessions/{id}`, `POST /api/v1/ai/sessions/{id}/messages` (Server-Sent Events: `text_delta`, `tool_call_proposed`, `tool_result`, `done`), and `POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}` (same SSE framing, for resolving a pending call). All of it requires `AbilityRoot`: a confirmed message can execute any mutating tool the platform exposes once approved, the same blast radius a root-scoped token would need to reach those actions directly, so do not hand this surface to a lower-privileged token.

## Scope and limits

- Single admin user, no per-user session isolation beyond the root-only gate: anyone who can authenticate as root can see and act on any session.
- One provider: Anthropic only, with no model routing or fallback.
- Not yet covered by real-infrastructure testing: `test/e2e/ai_chat_test.go` proves the full session lifecycle (create, send a message with an auto-executed read tool call, get, list, delete, and the flag-off 404) over real HTTP against a real router and a real `ai.Engine`, but with a scripted fake `ai.Provider`, never a live call to Anthropic's API. See [Feature status](/feature-status) for the current evidence tier.

## Troubleshooting

### AI assistant page not showing or returns 404

Likely cause: The `APP_EXPERIMENTAL=ai-chat` flag is not set, or the control plane was not restarted after setting it.

How to confirm: Check the flag: `ps aux | grep levelrail | grep -i experimental` shows environment variables. Or try the API: `curl https://<control-plane>/api/v1/ai/sessions` should return a list (not 404).

Fix: Set `APP_EXPERIMENTAL=ai-chat` on the control plane and restart. Wait 10 seconds for the restart, then reload the dashboard page.

Still stuck: Check control plane logs for startup errors: `levelrail logs --since 1m | grep -i "ai\|experimental"`.

### "API key not configured" error when starting chat

Likely cause: No API key has been set, or the key was not saved correctly.

How to confirm: Check if a key is configured: `levelrail-cli settings ai-assistant get` returns "Key: configured" or "Key: not configured". Try setting the key again.

Fix: Add your API key: `levelrail-cli settings ai-assistant set --model claude-sonnet-4-5 --api-key sk-ant-<your-key>`. Wait a moment, then refresh the AI assistant page and try again.

Still stuck: Verify the key format starts with `sk-ant-`. Keys are never returned by the API, only whether one exists, so there is no way to check if it is correct without trying it.

### Chat returning errors or models unavailable

Likely cause: The API key is invalid, your Anthropic account is out of credits, or the model name is wrong.

How to confirm: Try the chat: `levelrail-cli ai chat "hello" --session test --json 2>&1 | grep -i error`. The error message will indicate if it is auth, quota or model mismatch.

Fix: Verify your API key is active in your Anthropic account. Check your credits. If the model was recently renamed (for example, `claude-opus-4-1` became `claude-opus-5`), update it: `levelrail-cli settings ai-assistant set --model claude-opus-5 --api-key sk-ant-...`.

Still stuck: Log into your Anthropic dashboard to verify the key and account status.

### Pending confirmation never resolves or takes too long

Likely cause: The confirmation request timed out, or the network connection was interrupted.

How to confirm: Check the session state: `levelrail-cli ai sessions get <session-id> --json | jq .state` should show "pending_confirmation". Look for a recent approval/rejection: `levelrail-cli ai sessions get <session-id> --json | jq .confirmations[-1]`.

Fix: Either approve or reject the pending action: `levelrail-cli ai sessions resolve <session-id> <confirmation-id> --approve` or `--reject`. If the session appears stuck, create a new session and chat again: `levelrail-cli ai chat "what happened?" --session <new-id>`.

Still stuck: Check control plane logs for AI session errors: `levelrail logs --since 30m ai`.

### Chat works but tool results are wrong or missing

Likely cause: The AI assistant does not have permission to read the resource, or the tool call failed and the model retried with a workaround.

How to confirm: Check the session transcript: `levelrail-cli ai sessions get <session-id> --json | jq '.messages[] | select(.role == "assistant") | .content'` shows tool calls and results. Look for "tool_error" or "permission_denied" in the results.

Fix: Verify the token you are using has `root` ability (required for all AI assistant routes and tools). If the tool call failed (for example, "app not found"), the input is wrong; try the query again or ask the model to try a different search.

Still stuck: See [AI assistant integration](/ai-assistant#untrusted-text-and-the-assistants-confirmation-gate) for the tool permissions model.

## See also

- [ai-assistant.md](/ai-assistant): `levelrail-mcp`, the standalone MCP server for an external AI client, and the confirmation/untrusted-text mechanism this chat shares with it.
- [experimental-features.md](/experimental-features): the `APP_EXPERIMENTAL` switch and what "off" means everywhere.
- [identity-and-access.md](/identity-and-access): API tokens and abilities (`ai` CLI commands and `/api/v1/ai/...` routes need `root`).
- [cli-reference.md](/cli-reference): full flag reference for `ai chat` and `ai sessions`.
