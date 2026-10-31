---
title: "In-app AI assistant chat"
tag: "Experimental"
description: "The in-app AI assistant with your own key for Anthropic, OpenAI compatible, Gemini or self-hosted models, behind the ai-chat experimental flag."
"og:url": "https://docs.levelrail.com/ai-assistant-chat"
---

This is a chat panel in the dashboard, backed by the control plane's own `internal/ai` engine. It reads your apps, deploys, logs, metrics and diagnostics through the same tool surface the MCP server exposes, and it can propose actions. Every action that would change anything pauses for an explicit confirmation click first. AI in Levelrail only reads and suggests on top of the API, and this chat uses the same engine and confirmation gate described in [AI assistant integration](/ai-assistant#untrusted-text-and-the-assistants-confirmation-gate), as a dashboard panel and a CLI command.

This is separate from `levelrail-mcp`. To point an external MCP client (Claude Desktop, Claude Code, your own) at this control plane, read [AI assistant integration](/ai-assistant) instead.

Source: `internal/ai`, `internal/api/ai_chat.go`, `internal/api/ai_settings.go`, `internal/api/ai_providers.go`, `web/src/components/AiChatPanel.tsx`, `cmd/levelrail-cli/ai.go`.

## Quick start in 3 steps

<Steps>
<Step title="Set the experimental flag">

```bash
APP_EXPERIMENTAL=ai-chat
```

Restart the control plane. The **AI assistant** page now appears in the dashboard.

</Step>
<Step title="Add your API key">

CLI: `levelrail-cli settings ai-assistant providers set anthropic --api-key-env ANTHROPIC_API_KEY` then `levelrail-cli settings ai-assistant use anthropic claude-sonnet-5-5`

Dashboard: Settings > AI assistant, pick a provider and paste its key.

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

Nothing runs until you bring your own key. Pick a provider from the list under [Providers](#providers), save its key, then choose the active provider and model. The key is checked with the provider before it is stored, kept with the same envelope encryption as other secrets, and never returned, logged, or written to the audit log.

<Tabs>
<Tab title="CLI">

```bash
export OPENAI_API_KEY=sk-...
levelrail-cli settings ai-assistant providers set openai --api-key-env OPENAI_API_KEY
levelrail-cli settings ai-assistant providers models openai
levelrail-cli settings ai-assistant use openai gpt-4.1
```

`levelrail-cli settings ai-assistant get` reports the active provider, the fallback chain and whether a key is configured.

Over the API, `GET /api/v1/settings/ai-assistant` returns `configured`, `active`, `fallback`, `limits` and `token_usage_month`, and `PUT` takes `active` and/or `fallback` (each `{provider, model}`). Keys are saved per provider with `PUT /api/v1/settings/ai-assistant/providers/{id}` and removed with `DELETE` on the same path.

</Tab>
<Tab title="Dashboard">

Open **Settings → AI assistant**, pick a provider, paste its key, then choose the active model.

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

`POST /api/v1/ai/sessions`, `GET /api/v1/ai/sessions`, `GET`/`DELETE /api/v1/ai/sessions/{id}`, `POST /api/v1/ai/sessions/{id}/messages` (Server-Sent Events: `text_delta`, `tool_call_proposed`, `tool_result`, `error`, `done`), and `POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}` (same SSE framing, for resolving a pending call). All of it requires `AbilityRoot`: a confirmed message can execute any mutating tool the platform exposes once approved, the same blast radius a root-scoped token would need to reach those actions directly, so do not hand this surface to a lower-privileged token.

## Scope and limits

- Single admin user, no per-user session isolation beyond the root-only gate: anyone who can authenticate as root can see and act on any session.
- One active provider and model at a time, plus an ordered fallback chain used only for transient failures. There is no per-request model routing.
- Not yet covered by real-infrastructure testing: `test/e2e/ai_chat_test.go` proves the full session lifecycle (create, send a message with an auto-executed read tool call, get, list, delete, and the flag-off 404) over real HTTP against a real router and a real `ai.Engine`, but with a scripted fake `ai.Provider`, never a live call to a provider's API. See [Feature status](/feature-status) for the current evidence tier.

## Providers

Three adapters cover every provider. All are native Go over `net/http`, with no vendor SDK, so keys stay on the control plane and never reach the browser.

| Adapter | Providers |
| --- | --- |
| `anthropic` | Anthropic (Messages API, tool use, streaming, prompt caching) |
| `openai_compat` | OpenAI, Azure OpenAI, OpenRouter, Groq, Together AI, Mistral, DeepSeek, xAI, Fireworks AI, Ollama, vLLM, LM Studio, and "Other (OpenAI compatible)" for any server speaking Chat Completions |
| `gemini` | Google Gemini (native `generateContent`) |

`levelrail-cli settings ai-assistant providers list` shows each provider, whether a key is saved and the result of its last test. Self-hosted providers (Ollama, vLLM, LM Studio, Other) can be saved without a key. Azure OpenAI needs a base URL (your resource's endpoint) and a key; it authenticates with the `api-key` header.

The model is a validated choice: `providers models <id>` lists what the provider offers (cached for `APP_AI_MODELS_CACHE_TTL`, default 10m), and you can still type any id the provider did not list. Default models in the catalog are suggestions only.

### Self-hosted and private endpoints

A base URL is checked before anything is sent to it:

- Public endpoints must be `https`, so a key is never sent in clear text.
- Every connection is checked against the resolved IP at dial time, so a public hostname that later resolves to a private address (DNS rebinding) is refused, as is a redirect to a different host.
- Private, loopback and link-local addresses (Ollama on `localhost`, vLLM on a LAN address) are allowed only for `openai_compat` providers, only for a root caller, and only for the host that caller typed in. Saving such a URL writes an `ai_assistant.private_base_url` audit entry with the provider and host (never the key).
- Credentials inside the URL are rejected.

### Example: Ollama on a Levelrail GPU node

Deploy the model as described in [AI models](/ai-models), then point the assistant at it:

```bash
levelrail-cli models deploy --name chat --engine ollama --model llama3.1:8b
levelrail-cli settings ai-assistant providers set ollama --base-url http://<node-private-address>:11434/v1
levelrail-cli settings ai-assistant providers models ollama
levelrail-cli settings ai-assistant use ollama llama3.1:8b
```

Pick a model that supports tool calling; a model without it can chat but cannot read your apps. If the control plane cannot reach the node address, `providers set` answers with an `unreachable` error and nothing is saved. A hosted `levelrail` model provider is not built in: use this form with the model's endpoint instead.

## Fallback chain

```bash
levelrail-cli settings ai-assistant fallback set groq:llama-3.3-70b-versatile anthropic:claude-haiku-5-5
```

When the active provider fails with a rate limit, an overload, a timeout or a network error, or its account is out of quota, the next entry is tried. It never falls back on an invalid key, a content refusal, a too-long conversation or an unknown model, and never after the answer has started streaming, so output is not duplicated. Each provider is also retried first with jittered backoff that honors `Retry-After`. At most 5 entries. Removing a provider's key removes it from the chain.

## Usage and budget

Every model call records prompt and completion tokens, provider, model, latency and error class. `levelrail-cli settings ai-assistant usage --days 30` (or `GET /api/v1/ai/usage?days=30`) aggregates by day, provider and model. Set `APP_AI_MONTHLY_TOKEN_BUDGET` to block new turns once the calendar month's tokens reach it; the default is no budget. A blocked turn is not stored, and the chat shows how many tokens were used.

## Error meanings

| Code | Meaning | Fix |
| --- | --- | --- |
| `invalid_key` | The provider rejected the key. | Save a valid key. |
| `quota_exhausted` | Out of credit or quota. | Add credit, or add a fallback. |
| `rate_limited` | Too many requests. | Wait, or add a fallback. |
| `overloaded` | Provider overloaded or down. | Retry, or add a fallback. |
| `context_too_long` | Conversation exceeds the model's window even after trimming. | Start a new chat or pick a larger model. |
| `model_not_found` | The provider has no such model. | Pick one from `providers models`. |
| `network` | Provider unreachable or timed out. | Check outbound network and base URL. |
| `stream_interrupted` | The connection dropped mid-answer. | Send the message again. |
| `blocked_address` | The URL failed the address policy. | See the endpoint rules above. |
| `budget_exhausted` | Monthly token budget used. | Raise the budget or wait. |

The chat stream sends an `error` event with `code`, `message`, `fix`, `provider` and `retryable` just before `done`. A cut-off answer is kept in the session marked as interrupted, and a tool call that never finished streaming is dropped, never run.

## Harness limits

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_AI_STEP_TIMEOUT` | `120s` | Longest a single model call may take. |
| `APP_AI_MAX_STEPS` | `12` | Model calls per turn before it stops. |
| `APP_AI_TOOL_OUTPUT_MAX_BYTES` | `32768` | Cap per tool result; cut output ends with `[truncated N bytes]`. |
| `APP_AI_DEFAULT_CONTEXT_TOKENS` | `128000` | Window assumed when the provider does not report one. |
| `APP_AI_MONTHLY_TOKEN_BUDGET` | none | Monthly token budget. |
| `APP_AI_RETRY_MAX_ATTEMPTS`, `APP_AI_RETRY_BASE_DELAY`, `APP_AI_RETRY_MAX_DELAY` | `3`, `1s`, `30s` | Retry policy. |
| `APP_AI_VALIDATE_TIMEOUT` | `10s` | Key check and model list timeout. |

Before each call the oldest tool results are replaced with a placeholder, then the oldest whole exchanges are dropped, until the conversation fits the model's window. The system prompt and your latest message are never dropped. Tool arguments are checked against the tool's schema before it runs, and a malformed or unknown call goes back to the model as an error. Whatever the provider, any tool that is not known read-only still waits for your approval.

## What is unverified

Everything here was tested against fake HTTP servers. No adapter has been run against a live Anthropic, OpenAI, Gemini, Azure, OpenRouter or any other real service, so wire details such as Azure URL shapes, Gemini schema acceptance, Ollama tool-call streaming quirks and the catalog's default model ids and brand marks may need adjusting on first real use. The self-hosted example above has not been run on a real GPU node. Prompt caching is requested for Anthropic but cache hits were not observed.

## Troubleshooting

### AI assistant page not showing or returns 404

Likely cause: The `APP_EXPERIMENTAL=ai-chat` flag is not set, or the control plane was not restarted after setting it.

How to confirm: Check the flag: `ps aux | grep levelrail | grep -i experimental` shows environment variables. Or try the API: `curl https://<control-plane>/api/v1/ai/sessions` should return a list (not 404).

Fix: Set `APP_EXPERIMENTAL=ai-chat` on the control plane and restart. Wait 10 seconds for the restart, then reload the dashboard page.

Still stuck: Check control plane logs for startup errors: `levelrail logs --since 1m | grep -i "ai\|experimental"`.

### "No usable AI provider is configured" when starting chat

Likely cause: No provider key is saved, or the active provider or model is not chosen.

How to confirm: `levelrail-cli settings ai-assistant get` shows the active provider and whether it is configured. `levelrail-cli settings ai-assistant providers list` shows which providers have a key.

Fix: `levelrail-cli settings ai-assistant providers set <id> --api-key-env VAR`, then `levelrail-cli settings ai-assistant use <id> <model>`.

Still stuck: `levelrail-cli settings ai-assistant providers test <id>` re-checks the saved key and reports the status.

### Chat shows an error with a suggested fix

Likely cause: The provider rejected the call. The error shown names the class (see [Error meanings](#error-meanings)) and a fix.

How to confirm: `levelrail-cli ai chat "hello" --json` prints the `error` event with its `code`. `levelrail-cli settings ai-assistant usage` shows error counts per provider and model.

Fix: Follow the fix in the error. For `invalid_key` or `quota_exhausted`, check the key and credit at the provider; for `model_not_found`, pick a model from `providers models <id>`.

Still stuck: Add a fallback provider so a transient provider problem does not stop the chat.

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
