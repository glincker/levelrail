---
description: The in-app AI assistant chat, how to enable it behind the ai-chat experimental flag, and the read-and-suggest confirmation gate that bounds what it can do.
---

# In-app AI assistant chat

A chat panel embedded in the dashboard, backed by the control plane's own `internal/ai` engine: it reads your apps, deploys, logs, metrics, and diagnostics through the same tool surface the MCP server exposes, and it can propose actions, but **every action that would change anything always pauses for an explicit confirmation click first**. This is the same AI non-goal the project plan states in [`CLAUDE.md`](../CLAUDE.md) section 2: "AI is a read-and-suggest layer on top of the API, nothing more." It is not a different implementation of that rule, it is the same engine and the same confirmation gate [`docs/ai-assistant.md`](ai-assistant.md) describes for `levelrail-mcp`, surfaced as a first-party dashboard panel and CLI command instead of (or alongside) an external MCP client.

This is a separate feature from `levelrail-mcp`. Read [`docs/ai-assistant.md`](ai-assistant.md) if you want to point an external MCP-compatible assistant (Claude Desktop, Claude Code, or your own) at this control plane instead of, or in addition to, the chat described here.

**Relevant packages:** `internal/ai`, `internal/api/ai_chat.go`, `internal/api/ai_settings.go`, `web/src/components/AiChatPanel.tsx`, `cmd/levelrail-cli/ai.go`.

## Enabling it

Two independent switches, both required:

1. **The experimental flag.** The in-app chat is gated behind `ai-chat` (see [`docs/experimental-features.md`](experimental-features.md)):

   ```
   APP_EXPERIMENTAL=ai-chat
   ```

   Off (the default), every route under `/api/v1/ai/sessions` and `/api/v1/settings/ai-assistant` answers `404`, the dashboard hides the **AI assistant** nav entry and the `/ai-assistant` page, and the CLI hides `ai chat`/`ai sessions` from `--help` and completion.

2. **A BYOK (bring your own key) provider, model, and API key.** The flag alone is not enough; nothing runs until a key is configured too:

   ```bash
   levelrail-cli settings ai-assistant set --model claude-sonnet-4-5 --api-key sk-ant-...
   ```

   Only `anthropic` is supported server-side today (`store.AIProviderAnthropic`); the key is stored via the same envelope encryption every other secret in this platform uses (section 4.10 of `CLAUDE.md`) and is never returned or logged. `levelrail-cli settings ai-assistant get` reports only whether one is configured, never the key itself. The dashboard's **Settings → AI assistant** page does the same thing through a form.

With both set, the dashboard's **AI assistant** page (and command palette entry) appears, and `levelrail-cli ai chat` stops returning the experimental-disabled error.

## What it can do

- **Read, always.** Logs, metrics, deploy history, node status, certificates, and crashloop diagnosis, through the same read-only MCP tool classification `docs/ai-assistant.md`'s "Modes and toolsets" section describes. These run automatically, with no confirmation, the moment the model asks for them.
- **Propose, never execute directly.** A mutating tool call (deploy, rollback, restart, an env change, and so on) is recorded as a pending confirmation and streamed to the UI as a `tool_call_proposed` event. It does not run until a human clicks **Approve** (dashboard) or runs `levelrail-cli ai sessions resolve <session> <confirmation-id> --approve` (CLI). Rejecting it is just as explicit a path (`--reject`), and the model sees "User declined to run this action" as the result either way, so it can explain and move on rather than retry blindly.
- **Never in the reconciliation path.** This bounds what the feature is allowed to become, not just what it does today: the assistant only ever calls the platform's own versioned REST API (`internal/apiclient`), the exact same path any external MCP client or the CLI already uses. It has no privileged internal entry point into the reconciler, the Docker client, or the database, matching the project plan's non-goal in section 2 ("No AI in the reconciliation path") and the architecture note in section 4.11.

Untrusted text handling (log content, deploy output, commit messages) and the confirmation gate's exact rules (what counts as "tainted," which annotations make a tool auto-run) are the same mechanism `docs/ai-assistant.md`'s "Untrusted text and the assistant's confirmation gate" section documents in detail; this page doesn't repeat it.

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
- **No reconciler access.** Worth repeating: this is architecturally the same constraint as the MCP server, not a weaker one. See "What it can do" above.
- **Not yet covered by real-infrastructure testing.** `test/e2e/ai_chat_test.go` proves the full session lifecycle (create, send a message with an auto-executed read tool call, get, list, delete, and the flag-off 404) over real HTTP against a real router and a real `ai.Engine`, but with a scripted fake `ai.Provider`, never a live call to Anthropic's API. See [`docs/feature-status.md`](feature-status.md) for the current evidence tier.

## See also

- [ai-assistant.md](ai-assistant.md): `levelrail-mcp`, the standalone MCP server for an external AI client, and the confirmation/untrusted-text mechanism this chat shares with it.
- [experimental-features.md](experimental-features.md): the `APP_EXPERIMENTAL` switch and what "off" means everywhere.
- [identity-and-access.md](identity-and-access.md): API tokens and abilities (`ai` CLI commands and `/api/v1/ai/...` routes need `root`).
- [cli-reference.md](cli-reference.md): full flag reference for `ai chat` and `ai sessions`.
