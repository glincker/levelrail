---
title: "Connect an AI assistant safely"
description: Wire Claude or another MCP client to Levelrail with a read-only token by default, know what each scope allows, what is audited, and how to revoke access.
"og:url": "https://docs.levelrail.com/mcp-safe-setup"
---

Logs, env values and app output can contain text written by someone else. An assistant that reads that text can be tricked into acting on it (prompt injection), and most MCP clients do not enforce any protection of their own. So the safe default lives in the token: a token that cannot write cannot be talked into writing.

## Tokens are read only by default

A new API token created without naming abilities is read only. Writing is an explicit opt-in, per token.

```sh
levelrail-cli tokens create --name claude-readonly                 # read only
levelrail-cli tokens create --name claude-ops --allow-changes      # read and write
levelrail-cli tokens create --name ci --abilities read,deploy      # explicit abilities
```

In the dashboard, Settings > Tokens > Create token starts with read only selected. Turn on "Allow changes" to add write, and read the warning first. The API takes the same choice: omit `abilities` for the default, send `"allow_changes": true` to opt in, or list `abilities` explicitly (`allow_changes` and `abilities` together are refused).

Every token shows an access label in `tokens list`, the dashboard and the API (`access`): `read_only` when nothing it holds can change anything, `can_change` otherwise. A token with an IAM policy attached always shows `can_change`, because a policy can allow changes its base abilities do not. Tokens that existed before this default keep exactly the abilities they had; only the label is new, so you can spot the ones that can write.

You can only grant what you hold yourself. A user with only `read` cannot create a token with `--allow-changes` or with explicit abilities they lack. When `abilities` is omitted, the default is capped at what the caller holds: under a `read_write` policy, a caller without `write` gets a read token instead of an error.

## What each scope allows

| Scope | What the assistant can do |
| --- | --- |
| `read` | List and inspect apps, deploys, logs, domains, monitors. Tools that return credential material (database users, backup targets, registry credentials, notification targets) are hidden. |
| `read:sensitive` | Adds those credential-bearing read tools. Grant it only when the assistant needs them. |
| `write`, `write:sensitive` | Change app config and env values. Needs the server mode and `--allow-write` too, see [MCP tools and write mode](/mcp). |
| `deploy` | Deploy, roll back and restart. |
| `root` | Everything. Never use it for an assistant. |

`tools/list` only offers tools the token can call, and `tools/call` refuses the same tools with a `forbidden` tool error naming the missing ability, so a hidden tool is also a refused tool. The server checks every call again on top of that.

## The default policy is a setting

The operator controls what an omitted `abilities` means with the runtime setting `mcp.default_token_scope`: `read_only` (the default) or `read_write`. Switching to `read_write` weakens a protection, so the CLI and API ask for confirmation.

```sh
levelrail-cli settings runtime get mcp.default_token_scope
levelrail-cli settings runtime set mcp.default_token_scope read_write --confirm-weakening
```

`APP_MCP_DEFAULT_TOKEN_SCOPE` only seeds the value when nothing is saved. `settings runtime` shows the effective value and where it came from. Changing the policy never alters tokens that already exist. See [runtime settings](/runtime-settings).

## Wire Claude or another client

1. Create the token: `levelrail-cli tokens create --name claude-readonly --agent claude`. Copy it once, it is not shown again.
2. Print the client config: `levelrail-mcp --print-config claude` (also `cursor`, `vscode`, `zed`). It uses `--mode read-only` and a token placeholder.
3. Paste the block into the client's MCP config and put the token in the `APP_API_TOKEN` env value.
4. Ask the assistant to run `whoami` and `list_apps`. It should see read tools only. If you see a tool that changes things, the token is broader than you meant: revoke it and create a new one.

Two layers protect you: the server mode (`read-only` registers read tools only) and the token (read abilities cannot write even if a mode is widened by mistake).

## What is audited

Every tool call writes an audit row with the tool name, the token id, the outcome and the argument names. Argument values are never recorded. A change made through a write token also keeps its normal audit row with the MCP client name:

```sh
levelrail-cli audit-log --client-kind mcp
```

Stdio rows are client-reported labels, not proof. See [MCP tools and write mode](/mcp) for the exact fields.

## Behaviour changes

If you upgrade from a release before these safe defaults, expect:

- `tools/list` for a plain `read` token no longer shows the sensitive tools (database users, backup targets, registry credentials, notification targets and similar), and `tools/call` refuses them. Grant `read:sensitive` if the assistant needs them.
- `POST /auth/tokens` with no `abilities` now creates a read token instead of returning a 400.
- `levelrail-cli tokens create` works without `--abilities` and gives a read token. Add `--allow-changes` for write.
- `levelrail-cli tokens list` has a new ACCESS column (`read_only` or `can_change`), and the API returns `access` on every token.
- Resetting a security runtime setting to a weaker environment seed or default now needs `--confirm-weakening`.

## Revoke a token

```sh
levelrail-cli tokens list            # check the ACCESS column
levelrail-cli tokens revoke <id>
```

Revoking takes effect on the next request. Give assistant tokens a short lifetime (`--expires-in-days 30`) so a forgotten one expires by itself.

## Related

- [AI assistant integration](/ai-assistant): run the server and scope a token
- [Connect an AI assistant safely](/mcp-safe-setup)
- [MCP tools, token budget and write mode](/mcp)
- [MCP recipes](/mcp-recipes)
- [MCP tool surface](/mcp-tool-surface)
