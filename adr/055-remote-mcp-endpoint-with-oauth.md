# ADR 055: Remote MCP endpoint on the control plane, with OAuth 2.1

Status: Proposed (slice 1 shipped, see below)

Date: 2026-10-29

## Status: slice 1 shipped

Slice 1 landed on `release/v0.8.12` before this ADR was edited, and the code
differs from the slice 1 text above. The code is the source of truth:

- Path is `POST /mcp`, not `/api/mcp`. The feature key is `mcp-http`, not
  `remote-mcp`. Env vars are `APP_MCP_HTTP_RPM`, `APP_MCP_HTTP_MAX_BODY_BYTES`,
  `APP_MCP_HTTP_MAX_RESPONSE_BYTES`, `APP_MCP_HTTP_CALL_TIMEOUT_SECONDS` and
  `APP_MCP_HTTP_ALLOWED_ORIGINS`, not `APP_MCP_REMOTE_*`.
- The code lives in `internal/api` (`mcp_http.go`, `routes_mcp_http.go`), not
  in `internal/mcpremote`. It reuses `mcptools.NewServerWithOptions` in
  `read-only` mode and an in process round tripper that keeps the caller's
  address.
- Any token holding `read` is accepted, agent named or not. There is no
  `agent-core` profile, no server cache and no `APP_MCP_REMOTE_AUDIT_READS`
  switch yet. Responses use `JSONResponse`.
- The `mcp.tool_call` audit row records the tool name, token id, outcome,
  duration and argument names, never argument values. There is no argument
  hash.
- The `Cookie` header is stripped before the request reaches the handler and
  the inner REST calls, so a session cookie never authenticates here.
- A write capable token still sees only read tools, and a call to a write
  tool fails. A revoked token gets 401 on its next call. Both are covered by
  tests in `internal/api/mcp_http_test.go`.

Threat gaps not yet closed:

- **SSRF through tools.** A read tool that makes the control plane fetch a
  URL the caller controls (for example a webhook or health probe test) runs
  from the control plane's network position, which a remote caller can reach
  without a local client. Before widening the toolset, list every read tool
  that dials a caller supplied address and either drop it from the remote set
  or apply the existing outbound address checks to it.
- **Passkey wording.** The step-up text in section 4 says "MFA when the
  account has it". Accounts that sign in with a passkey must be covered too,
  so the consent screen and docs should say "passkey or MFA", and the step-up
  check must accept either as a fresh sign in.

## Context

The question: can a hosted agent client, or a custom connector someone
writes, talk MCP to a Levelrail control plane over HTTPS with a normal
OAuth sign in, without the user installing and running `levelrail-mcp`
next to it? Today the answer is no. Verified on `release/v0.8.11` by
reading the code:

1. **The MCP server is a separate binary bound to one token.**
   `cmd/levelrail-mcp/main.go` resolves one API token (flag, env or the
   credentials file), builds one `apiclient.Client` with it, and calls
   `mcptools.NewServerWithOptions(client, opts)` once
   (`internal/mcptools/server.go:28`). Every tool call reuses that client,
   so the process acts with that single token's authority.
2. **An HTTP transport exists, but it is single tenant.**
   `--transport=http` (`main.go`, the `transportHTTP` case) refuses to
   start without a token and binds loopback by default (`--listen`).
   `cmd/levelrail-mcp/serve.go` wraps `mcp.NewStreamableHTTPHandler` in
   `auth.RequireBearerToken` with `bearerTokenVerifier`, a constant time
   compare against the process's own static API token, with
   `AllowMissingExpiration: true`. It is stateful (default handler
   options), there is no OAuth, no discovery metadata and no per caller
   identity: whoever holds the token is the process.
3. **Exposure is filtered client side, enforced server side.**
   `internal/mcptools/modes.go` defines `read-only`, `standard` (no
   destructive tools) and `full`, plus `Toolsets`, `Profile`
   (`agent-core`), `AllowWrite` (confirm gated write tools, never in read
   only) and `Dynamic` (core tools plus `list_toolsets`,
   `enable_toolset`, `disable_toolset` in `toolsets.go`, which add tools
   for the rest of the session and send `tools/list_changed`).
   `cmd/levelrail-mcp/readonly.go` makes `--readonly` ask the API
   (`Whoami`) and refuse to start if the token holds `write`,
   `write:sensitive`, `deploy` or `root`.
4. **Tool listing follows abilities.** `internal/mcptools/abilities.go`
   maps each tool to the abilities that can satisfy it (`read` or
   `read:sensitive` for reads, `deploy` for `deployTierTools`, `root` for
   `rootTierTools`, otherwise `write` or `write:sensitive`).
   `abilityProbe` reads `/api/v1/auth/my-access` with a 30 s cache and
   returns nil, meaning do not trim, when the token has IAM policies.
   `abilityMiddleware` only trims `tools/list`; the comment and the code
   agree that calls are left to the API to decide.
5. **AI control is enforced by the control plane.** `docs/ai-control.md`
   says an agent is an API token with an agent name, modes are `off`,
   `observe`, `operate`, `admin` (the last behind
   `APP_EXPERIMENTAL=ai-control`), agents can never approve deploys or
   change AI control, and a fresh instance starts in `off`.
   `internal/mcptools/ai_control.go` reads the mode on each `tools/list`
   and hides tools above it, only for agent tokens.
6. **Audit records the MCP client, from the initialize handshake.**
   `withAgentClient` (`internal/mcptools/classes.go:421`) reads
   `req.Session.InitializeParams().ClientInfo` and sends it as
   `X-Agent-Client`; `internal/api/audit.go:173` stores it, sanitized,
   capped at 128 characters (`internal/api/agent_identity.go`).
   `internal/mcptools/route_audit_test.go` (`TestMCPToolRoutesAreAudited`)
   checks that every mutating or sensitive tool hits an audited route.
7. **Token budget is ratcheted in tests.** `internal/mcptools/budget_test.go`
   and `budget_headroom_test.go` enforce per mode budgets and a per
   toolset ceiling; `docs/mcp-tool-surface.md` reports the dynamic core
   at 21 tools and about 2036 estimated tokens (ratchet 3150, ceiling per
   toolset 4000), and `agent-core` at 15 tools and about 1684.
8. **Tokens, device login and IAM exist; an OAuth authorization server
   does not.** `internal/authengine/tokens_service.go` has `MintToken`
   (`MintInput` with owner, name, abilities, optional `ExpiresAt`,
   `AgentName`), `LookupBearer`, `Authenticate`, `RevokeToken`.
   `device_service.go` implements a device flow (`StartDevice`,
   `DecideDevice`, `RedeemDevice`) with approver capped abilities, routes
   under `/api/v1/auth/device/*`, lifetime from `APP_DEVICE_TOKEN_TTL_DAYS`.
   `internal/api/iam.go` evaluates policy documents per ability and
   resource. The `oauth_*.go` files in `internal/authengine` are the
   control plane acting as an OAuth or OIDC client to sign people in
   (`docs/sso-oidc.md`), and `internal/api/pipelines_oidc.go` is an OIDC
   issuer for pipeline jobs only (`APP_OIDC_ISSUER_URL`,
   `/.well-known/jwks.json`). A grep for `code_challenge` and
   `authorization_endpoint` finds no authorization server. Nothing
   narrows a token to a project on the server: `MintInput` has no project
   field, and `X-Scope-Team` / `X-Scope-Project`
   (`internal/apiclient/scope.go`) are request narrowing sent by the
   client, not a grant.
9. **The SDK can do most of the transport work.** `go.mod` pins
   `github.com/modelcontextprotocol/go-sdk v1.8.0`. Its
   `StreamableHTTPOptions.Stateless` drops `Mcp-Session-Id`, uses "a
   temporary session with default initialization parameters for each
   request", rejects server to client requests and answers GET and DELETE
   with 405. `auth.RequireBearerTokenOptions` has `ResourceMetadataURL`
   and `Scopes`, `auth.ProtectedResourceMetadataHandler` serves RFC 9728
   metadata, and `oauthex` has client side discovery and registration
   helpers. It does not ship an authorization server.

Two consequences of item 9 matter for the design. In stateless mode
`InitializeParams` is the default, so `withAgentClient` would send an
empty client name. And `enable_toolset` cannot persist across requests
nor deliver `tools/list_changed` outside the request that caused it.

## Decision (recommended)

Serve MCP from the control plane itself at `/api/mcp`, stateless
streamable HTTP, behind a new experimental feature `remote-mcp`.
Authenticate every request with an opaque, short lived, agent named API
token issued by a small OAuth 2.1 authorization server built into
`internal/authengine`. Each request builds (or reuses from a short per
token cache) an MCP server whose API client carries the caller's own
token, so every tool call is re-authorized by the existing API
middleware, abilities, IAM and AI control. No ambient authority exists in
the MCP layer.

### 1. Transport and mount

- Path `/api/mcp` on the main API listener, same origin as the dashboard
  and the issuer. POST only in practice: stateless mode answers GET and
  DELETE with 405, so no SSE stream and no session table to scale or
  expire.
- `JSONResponse` stays off so a long tool result can still stream within
  one POST. Server to client requests (sampling, elicitation) are not
  offered remotely.
- Validate `Origin` when present against the configured dashboard URL
  plus an env allowlist, to block DNS rebinding from a browser.
- The API client inside the handler does not dial the control plane over
  the network. It uses an in process `http.RoundTripper` that calls the
  router's handler directly, with `RemoteAddr` and forwarded headers set
  from the original request. Reason: a loopback self call would make
  every MCP request look like it came from 127.0.0.1, which breaks client
  IP access policies (`internal/api/access_policy.go`) and audit IPs, and
  could even let a loopback trust rule widen access.

### 2. Per request server, no ambient authority

- `RequireBearerToken` with a verifier backed by `authengine.LookupBearer`
  plus `Authenticate`. The verifier returns `TokenInfo` with the real
  expiry, so `AllowMissingExpiration` stays false here.
- The MCP server for the request is built with
  `NewServerWithOptions(clientForThisToken, optsFromGrant)`. Building it
  registers every tool, so the cost per request is unknown (see What
  cannot be verified locally). If it is too slow, cache built servers by
  token id with `APP_MCP_REMOTE_SERVER_CACHE_TTL`, evicted on revoke and
  on expiry. The cache key is the token id, never the client id, so no two
  people ever share a server.
- The MCP layer never holds a control plane credential of its own. A tool
  can only do what the caller's token can do over REST.

### 3. Authorization server

New files in `internal/authengine`, opaque tokens minted through the
existing `MintToken`, so `LookupBearer`, `RevokeToken`, the token list in
the dashboard, `ai-control revoke-agents` and AI control all apply with no
new enforcement path.

- Discovery: `/.well-known/oauth-protected-resource` (RFC 9728, via the
  SDK handler, `resource` is the absolute `/api/mcp` URL) and
  `/.well-known/oauth-authorization-server` (RFC 8414). The 401 from
  `/api/mcp` carries `WWW-Authenticate` with `resource_metadata`.
- Endpoints: `/oauth/authorize` (redirects to a dashboard consent page),
  `/oauth/token`, `/oauth/revoke` (RFC 7009), and `/oauth/register`
  (RFC 7591) only when dynamic registration is on.
- Authorization code with PKCE, `S256` only, `plain` refused. Code
  lifetime `APP_MCP_OAUTH_CODE_TTL` (default 60s), single use, bound to
  client id, redirect URI and challenge. `state` is required and echoed.
- Resource indicators (RFC 8707): the token is bound to the `/api/mcp`
  resource and refused on any other path. In practice that means the
  minted token carries a marker the API router checks, so an MCP token
  cannot be replayed against the full REST API from outside. This is new
  code in the auth middleware, not a reuse.
- Clients: pre-registered clients are created by an admin in the
  dashboard (name, redirect URIs, allowed scopes). Dynamic registration is
  off by default (`APP_MCP_OAUTH_DCR=off`) because some hosted clients
  need it and others do not, and an open registration endpoint is an
  abuse surface. Registered clients are public clients (no secret) unless
  an admin creates a confidential one.
- Access token lifetime `APP_MCP_OAUTH_ACCESS_TTL` (default 1h). Refresh
  tokens rotate on every use, lifetime `APP_MCP_OAUTH_REFRESH_TTL`
  (default 30 days), absolute grant lifetime
  `APP_MCP_OAUTH_GRANT_MAX_AGE`. Reuse of a rotated refresh token revokes
  the whole grant family.
- Each grant mints tokens with agent name `mcp:<client name>` (truncated
  to the 64 character agent name limit), so AI control always governs a
  remote MCP caller. An unnamed token can never be issued through OAuth.

### 4. Consent screen

A dashboard page, shown only to a signed in person (existing session,
existing CSRF protection on the POST). It shows the client name marked
as self declared for dynamically registered clients, the redirect host,
the requested scopes as plain abilities, the toolsets with their
estimated token cost from the tool surface table, the team or project the
grant is narrowed to, the current AI control mode and what it allows, and
the token lifetimes. The user can lower but never raise what is asked.
Write scopes require a fresh sign in, and MFA when the account has it, no
older than `APP_MCP_OAUTH_STEPUP_MAX_AGE` (default 10 minutes).

### 5. Scopes, abilities and modes

| OAuth scope | Ability minted | MCP mode |
| --- | --- | --- |
| `mcp:read` | `read` | `read-only` |
| `mcp:read:sensitive` | `read:sensitive` | `read-only` |
| `mcp:write` | `write` | `standard` |
| `mcp:deploy` | `deploy` | `standard` |

- Abilities are capped by the approver's own, exactly like
  `DecideDevice`. `write:sensitive` and `root` are never grantable
  through OAuth. `full` mode (destructive tools) and `AllowWrite` (confirm
  gated tools) are not offered remotely in this ADR.
- The mode is computed from the minted abilities with the same rule as
  `checkReadonlyAbilities`, so a read grant gets exactly what
  `--readonly` gets today.
- AI control narrows further on the server: in `off` the endpoint lists
  only `ai_control_status`, which matters because a fresh instance starts
  in `off`. The consent screen must say so.
- Project narrowing: the grant stores team and project, and the handler
  sends them as `X-Scope-Team` / `X-Scope-Project`. That is only request
  narrowing. A real server side bound needs either an IAM policy attached
  to the minted token or a new grant column the auth middleware enforces.
  Which one is open; until it exists the consent screen must not promise a
  project boundary.

### 6. Toolsets over a stateless transport

`Dynamic` is off for remote. The grant stores the chosen toolsets (or the
`agent-core` profile), and every request lists exactly that set.
`list_toolsets` stays as a read that reports what the grant includes and
what each other toolset would cost. `enable_toolset` and
`disable_toolset` return an error that names the dashboard page where the
user edits the grant, instead of pretending to change state. The default
grant is `agent-core`, so the default remote listing sits inside today's
budget. The consent screen refuses a toolset selection whose estimate
exceeds `APP_MCP_REMOTE_TOKEN_BUDGET` (default equal to the current
dynamic core ratchet, 3150).

### 7. Audit and client identity

- `X-Agent-Client` is set by the handler from the grant (client id and
  client name), not from `ClientInfo`, which is empty in stateless mode
  and self declared anyway. `withAgentClient` keeps its behavior for
  stdio.
- One audit row per remote tool call, action `mcp.tool_call`, with token
  id, client id, tool name, class, outcome and duration. Arguments are not
  stored, only a hash, because tool inputs can hold secrets. Read calls
  are included; `APP_MCP_REMOTE_AUDIT_READS=on` is the default and can be
  turned off if volume hurts. The existing route audit rows still happen
  underneath for mutating and sensitive routes.
- Grant created, refreshed (first refresh per day only), revoked and
  refresh reuse detected are audited.

### 8. Rate limits

- Every inner REST call already passes the existing API rate limiter
  (`internal/api/api_rate_limit.go`, `APP_API_RATE_LIMIT_READ_RPM`,
  `APP_API_RATE_LIMIT_WRITE_RPM`), keyed by token.
- New outer limits: `APP_MCP_REMOTE_RPM` per token on `/api/mcp`,
  `APP_MCP_OAUTH_TOKEN_RPM` per client on `/oauth/token`,
  `APP_MCP_OAUTH_REGISTER_PER_HOUR` per source IP on `/oauth/register`,
  and `APP_MCP_OAUTH_MAX_CLIENTS` total dynamically registered clients.

### 9. Redirect URIs, CSRF and CORS

- Redirect URIs match exactly, no wildcards, no fragments. HTTPS only,
  except `http://127.0.0.1` or `http://[::1]` with any port for native
  clients (RFC 8252). Fixed at registration.
- The consent POST is a dashboard session request with the existing CSRF
  token; the authorize request is bound to it by a server side record,
  not by query parameters replayed from the browser.
- No CORS on `/api/mcp` by default: hosted clients call server to server.
  `APP_MCP_REMOTE_CORS_ORIGINS` is an explicit allowlist for browser based
  connectors. Discovery documents allow `GET` from any origin.

### 10. Ingress, public domain and the issuer URL

- The issuer and resource URLs come from `APP_DASHBOARD_URL`
  (`internal/authengine/oauth_redirect.go`), never from the `Host` header.
  Behind the built-in ingress the control plane already sits on that
  hostname; `X-Forwarded-Proto` is trusted only from the ingress.
- With no public HTTPS URL configured the feature refuses to turn on and
  the dashboard and doctor say why. The supported paths are a public
  domain, the existing `cloudflare-tunnel` experimental feature, or
  staying on local stdio. A plain HTTP issuer is allowed only when it is
  loopback, for development.

## Alternatives rejected

- **Run `levelrail-mcp --transport=http` publicly.** One static token per
  process, no OAuth, no per caller identity. Fine for one person on
  loopback, wrong for many callers.
- **Separate listener or separate binary.** Another port, another TLS
  setup, another origin for the issuer and consent page, and a network
  self call that loses the client IP. Same listener is simpler and safer.
- **Stateful sessions.** Needs a session store, sticky routing for a
  warm standby, and expiry. Most of what sessions buy here (toolset
  changes) can be stored on the grant instead.
- **JWT access tokens.** Revocation would wait for expiry and AI control
  `revoke-agents` would not bite. Opaque tokens checked by `LookupBearer`
  revoke at once.
- **Delegate authorization to the configured external OIDC provider.**
  Tokens from it carry no Levelrail abilities, and not every install has
  one. Signing in to the dashboard may still use it; issuing MCP tokens
  stays local.
- **Reuse the device flow.** It works for a CLI on a terminal, but hosted
  clients expect the authorization code flow with discovery. The device
  code approval UI is a good template for the consent page, nothing more.

## Security

- Token theft: short access lifetime, rotating refresh with reuse
  detection, resource binding so an MCP token is useless against the rest
  of the API, instant revocation.
- Malicious or look-alike client: dynamic registration off by default,
  client name shown as self declared, redirect host shown, exact redirect
  match, PKCE required.
- Confused deputy: no server credential in the MCP layer, every call
  re-authorized with the caller's token.
- Prompt injection driving writes: write needs a write scope granted with
  step-up, AI control still caps it, agents still cannot approve deploys,
  and destructive and confirm gated tools are not offered remotely.
- Secret leakage through results: `write:sensitive` is never granted;
  `read:sensitive` is a separate, visible scope. Tool results already pass
  `wrapUntrustedResult` for untrusted tools.
- Audit tampering: client identity comes from the grant, not from the
  client.

## Failure modes

- AI control `off`: the endpoint works but lists only
  `ai_control_status`. Confusing unless the consent page and the tool
  explain it.
- Clock skew between ingress and control plane: expiry is checked on the
  control plane against its own clock; set the SDK `ClockSkew` small from
  env if needed.
- Per request build cost too high: latency on every call; the token keyed
  cache is the fallback.
- Warm standby promotion: grants and refresh tokens live in the database,
  so they survive; in flight requests fail and clients retry.
- A client that only speaks stateful sessions or needs server to client
  requests: unsupported, it gets 405 on GET.
- No public URL: feature stays off, with a clear reason.

## What cannot be verified locally

- Whether current hosted agent clients accept stateless streamable HTTP,
  require dynamic registration, honor RFC 8707 resource indicators and
  refresh correctly. Needs real clients against a public test instance.
- The cost of `NewServerWithOptions` per request at the current tool
  count. Not measured; slice 1 measures it.
- That the SDK's stateless mode behaves as its doc comment says in v1.8.0
  (read, not run).
- Whether an IAM policy can be bound to a single token today, which
  decides the project narrowing design. Not verified.
- Behavior behind the built-in ingress and through a tunnel with real
  certificates.

## Testing plan

- Handler tests with `httptest`: missing token gives 401 with
  `resource_metadata`; an unnamed token is refused; a read grant lists the
  same tools as `--readonly`; a write tool call with a read token is
  refused by the API, not by the MCP layer.
- Reuse `TestMCPToolRoutesAreAudited` style: every remote tool call writes
  one `mcp.tool_call` row with the grant's client id.
- OAuth tests: PKCE `plain` refused, wrong verifier refused, code reuse
  refused, redirect mismatch refused, refresh reuse revokes the family,
  revoke takes effect on the next request.
- A budget test for the default remote grant against
  `APP_MCP_REMOTE_TOKEN_BUDGET`.
- Client IP test: an access policy that blocks the caller's IP blocks the
  remote MCP call too.
- Manual: one hosted client and one custom connector against a public QA
  instance, both read only, then write with step-up.

## Phases

1. Stateless `/api/mcp` with existing agent named API tokens, read only.
2. Authorization server with pre-registered clients, PKCE, consent,
   refresh and revoke, read scopes only.
3. Dynamic client registration behind its own flag.
4. Write and deploy scopes with step-up, per grant toolsets editing.
5. Server side project boundary for grants.

## First thin slice (under 2 days)

`/api/mcp` behind `APP_EXPERIMENTAL=remote-mcp`, stateless, accepting an
existing API token that has an agent name and holds no write, deploy or
root ability (same check as `checkReadonlyAbilities`), built per request
with an in process round tripper, `agent-core` profile, client id taken
from the token's agent name for `X-Agent-Client`, one `mcp.tool_call`
audit row per call, `APP_MCP_REMOTE_RPM` limit, and a measurement of the
per request build cost logged at debug. No OAuth yet; custom connectors
that can send a bearer header can use it, and it proves the transport,
the re-authorization path and the audit before any auth server code.

## Slices and file ownership

| Slice | Files (new unless noted) | Env |
| --- | --- | --- |
| 1 Endpoint | `internal/mcpremote/handler.go`, `internal/mcpremote/roundtrip.go`, `internal/mcpremote/handler_test.go`, `internal/api/mcp_remote.go` (mount), `internal/experimental/experimental.go` and `catalog.go` (edit, add `remote-mcp`) | `APP_EXPERIMENTAL=remote-mcp`, `APP_MCP_REMOTE_RPM`, `APP_MCP_REMOTE_SERVER_CACHE_TTL`, `APP_MCP_REMOTE_AUDIT_READS` |
| 2 Auth server | `internal/authengine/oauth_server.go`, `oauth_server_clients.go`, `oauth_server_test.go`, `internal/api/oauth_server_routes.go`, one store migration (next free number), dashboard consent page | `APP_MCP_OAUTH_CODE_TTL`, `APP_MCP_OAUTH_ACCESS_TTL`, `APP_MCP_OAUTH_REFRESH_TTL`, `APP_MCP_OAUTH_GRANT_MAX_AGE`, `APP_MCP_OAUTH_TOKEN_RPM`, `APP_MCP_REMOTE_CORS_ORIGINS` |
| 3 DCR | `internal/authengine/oauth_server_dcr.go`, test | `APP_MCP_OAUTH_DCR`, `APP_MCP_OAUTH_REGISTER_PER_HOUR`, `APP_MCP_OAUTH_MAX_CLIENTS` |
| 4 Write | `internal/authengine/oauth_server_stepup.go`, `internal/mcpremote/toolsets.go`, consent page edits | `APP_MCP_OAUTH_STEPUP_MAX_AGE`, `APP_MCP_REMOTE_TOKEN_BUDGET` |
| 5 Project bound | auth middleware edit in `internal/api` (decided in slice 5), migration | none new |
| Docs | `docs/mcp-remote.md`, a link from `docs/mcp.md` | none |

Slices 1 and 2 share nothing but the mount file; slice 2 owns
`internal/api/oauth_server_routes.go` and slice 1 owns
`internal/api/mcp_remote.go`. The experimental catalog edit lands in
slice 1 only.

## Consequences

- People can connect a hosted agent client with a sign in and a consent
  click, and see and revoke the grant in the dashboard.
- The control plane becomes an OAuth authorization server, with the
  maintenance and review load that brings.
- Remote listing is static per grant, so the progressive toolset flow
  stays a stdio feature.
- `levelrail-mcp` stays the answer for local use, full mode and confirm
  gated tools.

## Open questions

1. Project boundary: bind an IAM policy to the minted token, or add a
   grant column the auth middleware enforces?
2. Should dynamic registration be offered at all in phase 3, or only
   pre-registered clients plus a one click registration in the dashboard?
3. Should confirm gated tools ever be remote, for example with a
   dashboard approval per call?
4. Default for auditing read calls: on (current proposal) or sampled?
5. Is a per request server build fast enough, or is the token keyed cache
   needed from day one?
