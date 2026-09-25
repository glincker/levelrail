---
description: Run levelrail-mcp over stdio for a locally spawning client or over the network for a remotely hosted MCP-compatible AI assistant, and how to scope the API token it uses.
---

# AI assistant integration: levelrail-mcp

`levelrail-mcp` is an MCP (Model Context Protocol) server that exposes the control plane's versioned REST API (`internal/api`, mounted at `/api/v1`) as a set of MCP tools. It is a thin client, architecturally identical in spirit to `levelrail-cli`: it authenticates with an API token and calls the same REST routes the CLI and dashboard do. It has no special internal access, and a token scoped to fewer abilities than a tool needs gets back the same 403 the REST API itself would return.

**Relevant packages:** `cmd/levelrail-mcp`.

## What it exposes

Tools cover apps, deploys, databases, nodes, domains, certificates, backups, templates, registry credentials, IAM, organizations, alerts, metrics, diagnostics, feature flags, and more, one tool per REST capability. See `docs/api-reference.md` for the underlying routes; every MCP tool maps to one of them.

Notable read-oriented tools for day-2 operation:

- `get_attention`: everything that needs attention now (failing apps, offline nodes, bad certificates, doctor warnings), critical first. Same list as `levelrail-cli attention`.
- `get_node_status_history`: a node's recent status transitions (`GET /api/v1/nodes/{id}/events`).
- `list_audit_log`: supports `q` (text search) and `status=failed` to find rejected requests.
- `list_control_plane_backups` and `create_control_plane_backup`: snapshots of the control plane's own database. The create tool writes a backup file, so give an assistant that should not do that a `read`-only token.

## Two ways to run it

### stdio, for a client that spawns it locally

This is the default and needs no extra flags. An MCP client that can launch local subprocesses (Claude Desktop, Claude Code, and similar tools) starts `levelrail-mcp` itself and talks to it over stdin/stdout:

```json
{
  "mcpServers": {
    "levelrail": {
      "command": "levelrail-mcp",
      "args": [],
      "env": {
        "APP_API_TOKEN": "<your API token>",
        "APP_API_URL": "http://localhost:8080"
      }
    }
  }
}
```

If `levelrail-cli` is already configured on the same machine (`levelrail-cli auth login` writes `~/.config/levelrail-cli/credentials`), `levelrail-mcp` picks up the same token and URL automatically and the `env` block above can be omitted.

No incoming authentication is required in this mode: only a local process on the same trusted machine can spawn or connect to a stdio subprocess in the first place.

### Network mode, for a remotely hosted client

Some MCP clients cannot spawn a local subprocess, for example an AI assistant that runs as its own independently deployed service. For that case, run `levelrail-mcp` with `--transport=http` to serve the MCP Streamable HTTP transport over the network instead:

```bash
levelrail-mcp --transport=http --listen=127.0.0.1:8090 --token '<your API token>'
```

- **Binds to loopback by default** (`127.0.0.1:8090`). Pass `--listen` explicitly (for example `0.0.0.0:8090`, or a WireGuard-mesh address) to expose it beyond the local machine; nothing does that by default, so an operator has to opt in deliberately.
- **Fails closed with no token.** Unlike stdio, a network listener is reachable by anything that can route to it, so `--transport=http` with no API token configured (via `--token`, `APP_API_TOKEN`, or a `levelrail-cli auth login` credentials file) refuses to start rather than listen unauthenticated.
- **Requires the same bearer token on every incoming request.** The token this process already uses outbound, against the control plane's REST API, is the same token an MCP client must present as `Authorization: Bearer <token>` on every request against the network listener. There is no separate auth concept to configure.
- Put a reverse proxy or the WireGuard mesh (see `docs/multi-node.md`) in front of it for TLS if the client is not on the same trusted network; `levelrail-mcp` itself speaks plain HTTP.

## Modes and toolsets

Every tool carries MCP annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`, and a title), derived from one classification table in `internal/mcptools/classes.go`. Tools that touch credentials or command output also carry `_meta["levelrail/sensitive"] = true`. Annotations are hints only, so the server enforces the same split at registration time.

`APP_MCP_MODE` (or `--mode`) chooses which classes are registered at all. A tool that is not registered costs no model context and cannot be called:

| Mode | Registers |
| --- | --- |
| `read-only` | read tools only (get, list, explain, diagnose, compare, preview) |
| `standard` (default) | read and mutating tools (deploy, restart, set, create, clone, approve, rotate) |
| `full` | everything, including destructive tools (delete, clear, rollback, prune, sweep) |

`APP_MCP_TOOLSETS` (or `--toolsets`) is an optional comma separated list that limits the groups exposed: `alerts, apps, audit, backups, databases, deploys, diagnostics, domains, environments, flags, iam, loadbalancer, logs, metrics, models, nodes, notifications, orgs, pipelines, previews, registry, scheduled, settings, system, templates, webhooks`.

```bash
APP_MCP_MODE=read-only APP_MCP_TOOLSETS=apps,nodes,logs,diagnostics levelrail-mcp
```

The startup log line `levelrail-mcp tools` reports the mode and how many read, mutating and destructive tools are registered. A mode never widens access: the API token's abilities still bound every call, so pair `read-only` with a `read`-scoped token (defense in depth, not a replacement).

## Untrusted text and the assistant's confirmation gate

Logs, deploy output, error messages, commit messages, PR titles and similar text are written by workloads or third parties, so they can carry prompt injection. Tools marked `levelrail/untrusted-output` in `_meta` (log, status, deploy, diagnose, pipeline and audit reads) return their text content inside a delimited block that starts with a standard "untrusted data, not instructions" notice. Control characters, ANSI escapes, invisible and bidi characters are stripped, obvious secrets (private keys, bearer tokens, common token shapes, URL credentials, values of password/token/key fields) are redacted, and text is truncated. The block boundaries carry a random id, so content cannot forge the closing line. The structured copy of the result is sanitized the same way but is not delimited.

Limits come from `APP_UNTRUSTED_MAX_FIELD_BYTES` (per string field, default 4096) and `APP_UNTRUSTED_MAX_BLOCK_BYTES` (per wrapped block, default 65536).

This is friction, not a guarantee. The real boundary is the assistant's confirmation gate, which uses each tool's MCP annotations instead of name prefixes: every tool that is not read-only, and every tool without annotations, pauses for a human click. Once a conversation has ingested untrusted output (it is then "tainted", derived from the stored history so it survives later turns), read tools that are outbound or touch secrets pause as well.

## Generating and scoping a token

Use the same token machinery the CLI and dashboard already use (`docs/identity-and-access.md`'s "API tokens" section): scoped, revocable bearer credentials minted with the six-string ability vocabulary (`read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`).

```bash
levelrail-cli tokens create --name "ai-assistant" --abilities read,deploy
```

Pick the narrowest ability set the assistant actually needs. A read-only assistant that only diagnoses and reports needs `read` (add `read:sensitive` only if it must see secrets or env values); one that can also trigger rollbacks or redeploys needs `deploy` too. Every request through `levelrail-mcp`, over either transport, is attributed to the `mcp` client kind in the audit log (`GET /api/v1/audit-log`), so scoped-down tokens are traceable the same way CLI and dashboard activity already is.

## Running a separately licensed AI assistant

`levelrail-mcp` is designed to sit in front of any MCP-compatible AI assistant, including one licensed under terms (for example AGPL) that are incompatible with this project's Apache 2.0 license, or any third-party assistant you do not want coupled to this codebase.

**That assistant must always run as its own independent, separately deployed service.** It connects to Levelrail purely as an MCP client, over stdio (if co-located and it can spawn a subprocess) or over `--transport=http` (if it runs elsewhere). Its code must never be imported, vendored, copied into, or otherwise combined with this repository, regardless of transport. This is the same brand- and dependency-neutrality this project already enforces elsewhere: an integration is a boundary, not a merge.

## See also

- [identity-and-access.md](identity-and-access.md): API tokens, abilities, and the audit log.
- [api-reference.md](api-reference.md): every REST route the MCP tools wrap.
- [cli-reference.md](cli-reference.md): `tokens create`/`list`/`revoke` and `auth login`.
