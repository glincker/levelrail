---
description: Authentication, authorization, roles, IAM policies, and audit logging for users and API tokens.
---

# Identity and access: users, roles, and IAM policies

Create a teammate with a curated role, scope a CI token down to one app, turn on two-factor auth, or review who changed what: this page covers who can sign in, what they can do once they're in, and a record of what they actually did.

::: details For contributors: where this lives in the source
- Backend: `internal/api/{auth,users,roles,abilities,iam,iam_handlers,invites,tokens,twofactor,oauth,oauth_settings,device_auth,audit,audit_retention}.go`
- CLI: `cmd/levelrail-cli/{users,iam,invites,tokens,auth}*.go`
- Dashboard: `web/src/routes/settings/{users,iam-policies,security,tokens,oauth,cli-access,audit-log}.tsx`
- OAuth sign-in decision logic: `completeOAuthSignin` in `internal/api/oauth.go`
:::

<InlineToc default-open />

## Why two permission models instead of one

Most self-hosted platforms pick one of two shapes and live with the downside.

- **Flat roles** (admin/member/viewer): Easy to reason about but cannot express "this CI token may deploy exactly one app and nothing else."
- **Full IAM policy engine**: Can express that but is overkill for "give Priya everything, give the contractor read-only."

This platform keeps both, layered:

**Abilities**

A flat, six-string vocabulary that every user and every API token carries directly: `read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`.

**Roles**

Named presets over the same ability list: `admin`, `operator`, `viewer`. These are a convenience for the common case, never a second storage location.

**IAM policies**

Optional, additive documents of Allow/Deny statements scoped to one resource (`app:web`, `database:main`, or `*`). Attach them to a user or token to grant or revoke access to a specific app without changing the principal's ability list.

**Evaluation**

An explicit Deny in an attached policy always wins, even over `root`. Everything else falls back to the flat abilities exactly as if no policy existed.

## How it actually works

### Principals: a session, or a token

Every gated route resolves the request to exactly one principal before deciding anything: a session cookie (mapped to a `store.User` row) or a bearer token (with its own stored `Abilities`).

**Sessions**

Sessions are an in-memory map keyed by an opaque token (`sessionStore`), with a TTL of 24h by default (overridable with `APP_SESSION_TTL`, a Go duration string). A restart clears every live session (an accepted tradeoff for a single-node control plane).

**Session cookie security**

The session cookie's `Secure` flag follows the request: it is set when the request arrived over TLS, or through the embedded Caddy ingress (a loopback peer sending `X-Forwarded-Proto: https`). `X-Forwarded-Proto` from any other peer is ignored, so a remote client can't spoof it. A fresh install reached at `http://<server-ip>:8080` therefore works out of the box, and the dashboard shows a persistent "connection is not encrypted" banner until you move to HTTPS.

Once you set an `https://` **dashboard URL** (Domains page, `levelrail-cli settings dashboard-url set --url https://...`, or `PUT /api/v1/settings/dashboard-url`), sign-in over plain HTTP is refused with a `403` pointing at that URL. This covers password login, 2FA verification, and invite acceptance. An `https://` URL can only be saved from a request that already arrived over HTTPS (or from the server itself), so saving it can't lock you out. If the https URL breaks later, set `APP_ALLOW_INSECURE_LOGIN=true` on the control plane and restart it to recover.

### Abilities and roles

```
read            view apps, logs, metrics, deploy history
read:sensitive  view secrets and other private config
write           change config, restart, manage domains and env vars
write:sensitive rotate secrets
deploy          trigger a deploy
root            everything, exclusive: combining root with anything
                else is rejected at validation time
```

A curated role just sets a principal's `Abilities` to one of three fixed
sets in one action:

| Role | Abilities |
| --- | --- |
| `admin` | `root` |
| `operator` | `read`, `read:sensitive`, `write`, `deploy` |
| `viewer` | `read` |

`GET /api/v1/roles` is the only place these live; nothing about a user
or token row remembers "I was created via the operator role." The
dashboard and CLI both compute a `role` field for display by exact-match
diffing a principal's abilities against the three presets
(`roleForAbilities`); anything that doesn't match one exactly shows as
"custom," which is expected and fine, not an error state.

### IAM policies: Allow/Deny over a resource

A policy document uses AWS IAM's shape, hand-typed or generated:

```json
{
  "Statement": [
    { "Effect": "Allow", "Action": ["deploy"], "Resource": ["app:checkout"] },
    { "Effect": "Deny", "Action": ["write:sensitive"], "Resource": ["*"] }
  ]
}
```

**Fields**

- `Action`: Ability strings or `*`
- `Resource`: `app:<name>`, `database:<name>`, or `*`. A trailing `*` matches as a prefix (e.g., `app:*` matches every app).

**Evaluation order** (`authorizeResource`, `internal/api/iam.go`)

Checked on every mutating or sensitive route scoped to a specific app or database (get, update, delete, deploy, rollback, restart, clone, exec, secrets, tags, domain config, backups, and so on):

```mermaid
flowchart TD
  A[Request arrives] --> B{Explicit Deny<br/>matching action + resource?}
  B -->|Yes| C[403 Forbidden]
  B -->|No| D{Principal's flat<br/>ability allows?}
  D -->|Yes| E[200 Allowed]
  D -->|No| F{Explicit Allow<br/>in policy?}
  F -->|Yes| G[200 Allowed<br/>scoped to resource]
  F -->|No| H[403 Forbidden]
```

Details:

1. An explicit **Deny** matching the ability and resource, in any attached policy, always wins. Full stop, even for a `root` principal.
2. Otherwise, the principal's own flat abilities decide, unchanged from having no policies attached.
3. Otherwise, an explicit **Allow** in an attached policy can still grant access the principal's flat abilities don't, scoped to that one resource.

**Attachment**

A policy attaches to a `user` or a `token` (`principal_type`), by that principal's id. A malformed stored document can never grant or deny anything; it's treated as if it simply doesn't mention the pair being checked.

Proven end to end, not just at the handler level: `test/e2e/iam_policy_enforcement_test.go` drives a real admin session through the real HTTP API to mint a second user and a token, attach Deny and Allow policies through the real IAM endpoints, and confirm the resulting 403/200s correspond to real state changes (or their absence).

### Bootstrap: exactly one path to the first admin

On startup, `BootstrapAdmin` creates a user from `APP_ADMIN_USERNAME` and `APP_ADMIN_PASSWORD` only if zero users exist. It is a no-op on later restarts, so a password change doesn't get silently reverted.

Without those env vars and with no user yet, the control plane generates a one-time **setup token**, writes it to `<data dir>/setup-token` (mode `0600`), and logs it once. `GET /api/v1/auth/setup-status` (public) returns `{"needs_setup": true}` until the first admin exists, and the login page switches to its setup form on its own. `POST /api/v1/auth/register` requires the token in `setup_token` (constant-time compare) and deletes the file on success, so whoever reaches the dashboard first can't claim the instance without shell access to the server. `install.sh` prints the token and a `http://<ip>:8080/login?setup=<token>` link; `sudo levelrail setup-token` prints it again.

`POST /api/v1/auth/register` is gated at the database layer to succeed exactly once (a unique constraint, not an application check). A race between concurrent first-registration attempts cannot create two "first" admins. Either path grants `AbilityRoot` because no one else exists yet.

::: tip Development only
`APP_DEV_MODE=1` seeds a fixed `dev`/`dev` admin for local development. Never use this in a real deployment.
:::

**Subsequent users**

Every user after the first is created by an existing root user, either directly (`POST /api/v1/auth/users`) or via an accepted invite.

**Lockout prevention**

A root user cannot edit their own abilities or delete their own account. The last remaining user can never be deleted. Both are hard 400s, not soft warnings. This is the rule standing between the platform and a permanent lockout.

### Two-factor authentication (TOTP)

Requires a master key configured on the control plane (`internal/secrets`). Without one, every 2FA route returns `501`.

**Setup flow**

```mermaid
flowchart TD
  A[POST /2fa/setup] -->|Mint secret| B[TOTP secret unconfirmed]
  B -->|Can call setup again<br/>overwrites pending| B
  B -->|User scans provisioning URI<br/>into authenticator app| C[GET /2fa or app shows secret]
  C -->|User enters live code| D[POST /2fa/confirm]
  D -->|Validate code| E{Code valid?}
  E -->|Yes| F[TOTPEnabled = true<br/>Return 10 recovery codes<br/>shown once only]
  E -->|No| G[Confirm fails]
  F --> H[2FA active]
  H -->|On next login| I[POST /auth/login returns<br/>mfa_required true + mfa_token]
  I -->|Exchange token + live code<br/>or recovery code| J[POST /2fa/verify]
  J -->|Valid| K[Session cookie granted]
```

Details:

1. `POST /api/v1/auth/2fa/setup`: Mint a fresh TOTP secret and store it unconfirmed. Calling it again before confirming overwrites the pending secret.

2. `POST /api/v1/auth/2fa/confirm`: Validate one live code against that secret, flip `TOTPEnabled`, and return 10 recovery codes in plaintext. This is the one and only time recovery codes are shown.

3. On login: `POST /api/v1/auth/login` returns `mfa_required: true` plus a short-lived `mfa_token` (5 minutes) instead of a session. Exchange that token plus a live code (or a recovery code) via `POST /api/v1/auth/2fa/verify` for the real session cookie.

**Disabling and recovery codes**

`POST /api/v1/auth/2fa/disable` re-verifies the second factor itself, not the account password. A password-only re-check would undermine 2FA's purpose.

Regenerating recovery codes invalidates the entire previous set.

**Single-use codes.** A TOTP code is accepted once. The control plane remembers the newest time step it accepted for each user (`users.totp_last_step`, migration `0350`), so a code that was just used, shoulder-surfed or replayed inside its roughly 90 second validity window is refused with `invalid code`. The check is atomic, so two simultaneous logins presenting the same code cannot both pass. The step is claimed at login (`/2fa/verify`), not at `/2fa/confirm`, so the code you enrol with can still be used once to sign in. Recovery codes were already single-use.

**Rate limiting**

Both the login-time verify step and every setup/confirm/disable call are rate limited (exponential backoff with a handful of free failures). This is separate from the password rate limiter.

### Passkeys (WebAuthn)

Sign in with Touch ID, Windows Hello, or a security key instead of a password. No master key required: a credential's public key is ordinary key material, not a secret, so it never goes through `internal/secrets`.

**Registering a passkey** (`POST /api/v1/auth/passkeys/register/begin` then `.../register/finish`) requires an existing session: it adds a credential to the account you're already signed in as. `GET /api/v1/auth/passkeys` lists an account's own credentials; `DELETE /api/v1/auth/passkeys/{id}` revokes one.

**Signing in with a passkey** (`POST /api/v1/auth/passkey-login/begin` then `.../finish`) is username-first, not usernameless: the operator types their username, the server looks up that account's own credentials, and the browser's passkey prompt proves possession. A successful passkey sign-in completes the session the same way a password does, without an additional TOTP prompt even if the account has 2FA enabled, the same shape OAuth sign-in already has.

Every registration and login challenge is single-use and expires in 5 minutes; the relying party ID and origin are derived from the request's own `Host` header, since there is no single fixed domain to configure on a self-hosted platform.

### OAuth sign-in

Four providers are supported: `google`, `github`, `microsoft` (Azure AD, common multi-tenant endpoint), `oidc` (generic OpenID Connect, requires an issuer URL).

Settings are per-provider rows (`GET`/`PUT /api/v1/settings/oauth[/{provider}]`), gated at `AbilityRoot` to change. Enabling a provider requires a client ID and a client secret (OIDC also requires an issuer URL). The secret is write-only over the API; `GET` only reveals `has_client_secret`.

**Sign-in behavior**

```mermaid
flowchart TD
  A[User initiates OAuth<br/>with provider] --> B[GET /oauth/provider/start]
  B -->|Redirect to provider| C[User authenticates]
  C -->|Callback with code| D[GET /oauth/provider/callback]
  D -->|Exchange code for identity| E{Identity lookup}
  E -->|Already linked| F[Sign in as existing user]
  E -->|Brand-new email| G{Domain allowed?}
  G -->|Yes| H[Auto-provision new user<br/>AbilityRead only]
  G -->|No| I[403 Forbidden]
  E -->|Email belongs to<br/>different user| J[403 Forbidden]
  F --> K[Session cookie granted]
  H --> K
  L[During session:<br/>GET /oauth/provider/link/start] -->|Link new provider| M[Add provider to<br/>existing account]
  M --> K
```

Details:

- **Already-linked identity**: Signs in as its existing owner.
- **Brand-new email**: Auto-provisions a new user with `AbilityRead` only (least-privilege default, never `root`, since a fresh OAuth signup is never the platform's first user). An existing root user can grant more later via `PUT /api/v1/users/{id}/abilities`.
- **Email already belonging to another account**: Refused outright. Auto-linking here would let an anonymous sign-in silently take over an unrelated account.
- **Linking a new provider to an existing account**: Requires an existing live session. Use `GET /api/v1/auth/oauth/{provider}/link/start` to do this.
- **Allowed email domain**: If `AllowedEmailDomain` is set on a provider, a new signup outside that domain is refused.

### Team invites

`POST /api/v1/invites` mints a token for one named email, specifying either `--role` or `--abilities`.

**Privilege check**

The caller cannot invite someone with an ability they don't hold themselves. `AbilityWrite` gates the route, but that per-ability cap is the actual boundary.

**Accept link**

The response always includes the plaintext accept link, whether or not SMTP is configured. A control plane with no email capability is still fully usable by copy/pasting the link.

The link is absolute when the control plane knows its own origin: the primary domain if one is set, otherwise the dashboard URL (the Domains page, or `levelrail-cli settings dashboard-url set`). With neither set, the link and the emailed link are a bare path (`/accept-invite?token=...`) that only works if pasted after the dashboard's address, so set the dashboard URL before inviting people by email. The request's `Host` header is never used to build the link. The password reset email follows the same rule.

**Invitation acceptance**

Default TTL is 7 days (`APP_INVITE_TTL`). `POST /api/v1/invites/accept` creates a normal local-password user through the exact same path `POST /api/v1/auth/users` uses. An invited account is never distinguishable from an admin-created one.

**Visibility**

A non-root caller only sees invites they created themselves. A root caller sees every pending invite.

`GET /api/v1/users` follows the same idea: a `root` caller lists every account, any other caller (a `viewer`, an `operator`, a scoped token) gets only their own record, or an empty list for a token. Emails and ability sets of colleagues are not visible to read-tier callers.

**IAM Deny and lists.** A Deny policy on `app:<name>` hides that app from `GET /api/v1/apps`, `apps-summary`, `apps-metrics`, deployments, certificates (including the auto-generated `sslip.io` hostname), `GET /api/v1/network/topology` (apps and databases) and the container names in `GET /api/v1/system/containers`, as well as returning `403` on every `/apps/{name}/...` route.

### API tokens

Scoped, revocable bearer credentials for the CLI, CI, and MCP integrations. Minted with the same six-string ability vocabulary users carry, plus an optional expiry (`expires_in_days`, 0 means never). The plaintext is returned exactly once at creation; every later read (`GET /api/v1/auth/tokens`) shows only metadata.

**Session-only token management**

Token management (`create`/`list`/`revoke`) is deliberately session-only, never bearer-token authenticated. A token can never mint or revoke another token on its own behalf. This is why the CLI's `tokens create`/`list`/`revoke` and `auth login` prompt for username and password instead of accepting `--token`. No bearer token can ever call those routes, however broadly scoped.

**Device-code flow**

The device-code flow (`POST /api/v1/auth/device/start` and `/token`, `levelrail-cli auth login --device`) sidesteps the plain-HTTP session-cookie gap.

- The CLI polls with a random device code.
- An operator approves it from the dashboard's CLI Access page (`/settings/cli-access`) using their already-established session.
- The resulting token inherits exactly that operator's abilities.
- The CLI never picks the permissions.

### Session links

A session link is a one-time login URL rather than a credential you type. Mint one from an already-authenticated context and hand the URL to whatever needs to sign in next: CI, an AI agent driving the dashboard through browser automation, or a fresh incognito window you don't want to type a password into. Opening it signs that browser in immediately, no username or password prompt.

`POST /api/v1/auth/session-links` mints one. Gated `root`, because the link it produces is root-equivalent: anyone who gets hold of the token before it's used can sign in as whoever minted it.

```bash
curl -s -X POST https://your-control-plane/api/v1/auth/session-links \
  -H "Authorization: Bearer $TOKEN"
```

```json
{ "token": "st_...", "url": "https://your-control-plane/login?session_link=st_..." }
```

Open the `url` in a browser. `/login?session_link=<token>` consumes the token automatically on page load (`GET /api/v1/auth/session-links/{token}/consume`, public, gated by possession of the token itself) and establishes a session, the same way a normal login does. No form, no click-through.

**CLI shortcut**

```bash
levelrail-cli auth session-link
# prints the ready-to-open URL and nothing else
```

**Security model**

- **Short-lived.** The token expires 2 minutes after minting, whether or not it's used.
- **Single-use.** Consuming it is an atomic claim; if two requests race on the same token, at most one succeeds and the other gets the same "invalid or expired" error a stale token would.
- **Can't escalate.** The resulting session carries exactly the minting identity's own abilities, snapshotted at mint time, never more. Minting one already requires `root`; it's a convenience for an identity that could already do anything, not a way to grant new access.
- **Minting a link on behalf of an API token** (rather than a logged-in user) produces a session pinned to that token's ability snapshot, since there's no user row to attach an ordinary session to. Revoking the underlying token afterward doesn't retroactively end a session already established from it, the same way revoking a token never ends sessions a user already has open.

### Audit log

Every request gated above `AbilityRead` (write, deploy, root-tier, and `read:sensitive`) gets one row:

- Actor type and id
- Resolved display name
- Ability checked
- HTTP method and path
- Status code
- Remote address
- Timestamp
- Caller surface (`cli`, `dashboard`, `mcp`, or `api`, sniffed from `User-Agent`)

Recording is best-effort and runs after the real request completes. A failed audit write is logged and dropped, never turned into a failed request.

**Query**

`GET /api/v1/audit-log` is cursor-paginated (`?before`, an RFC3339 timestamp) and filterable by `?path`, `?method`, `?client_kind`, and `?agent` (the agent label of the token, see [AI assistant](ai-assistant.md#agent-identity)). Use `?format=csv` to return rows as a downloadable attachment instead of JSON, for compliance export. Cells that start with `=`, `+`, `-`, `@` or a tab get a leading `'` in the CSV, so a token named `=HYPERLINK(...)` is not evaluated as a formula when the export is opened in a spreadsheet.

**Retention**

Defaults to 90 days (`APP_AUDIT_LOG_RETENTION_DAYS`). The system sweeps automatically on an interval (`APP_AUDIT_LOG_SWEEP_INTERVAL`) and is purgeable on demand via `POST /api/v1/audit-log/purge`.

## Integration walkthrough

<Steps>
<Step title="Bootstrap the first admin">

Do this once, before anyone can sign in.

::: code-group
```bash [Environment]
export APP_ADMIN_USERNAME=admin@example.com
export APP_ADMIN_PASSWORD='a-real-password'
# restart the control plane
```
```bash [Verify with curl]
curl -s -c cookies.txt -X POST https://your-control-plane/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin@example.com","password":"a-real-password"}'
```
:::

</Step>
<Step title="Create a teammate with a curated role">

No hand-picked abilities needed.

```bash
levelrail-cli users create --email ops@example.com --password 'temporary-pw' --role operator
```

```json
{ "id": "user_abc123", "email": "ops@example.com", "role": "operator", "abilities": ["read", "read:sensitive", "write", "deploy"], ... }
```

</Step>
<Step title="Scope a CI token down to one app">

Use an IAM policy, which is tighter than any flat role can express.

::: code-group
```bash [Create token]
levelrail-cli tokens create --name "ci-checkout" --abilities deploy
# → token tok_xyz, plaintext shown once
```
```bash [Create policy]
levelrail-cli iam policies create --name "checkout-only" \
  --document '{"Statement":[{"Effect":"Allow","Action":["deploy"],"Resource":["app:checkout"]}]}'
# → policy pol_abc
```
```bash [Attach policy to token]
levelrail-cli iam policies attach pol_abc --principal-type token --principal-id tok_xyz
```
:::

That token can now deploy `checkout` even without a global `deploy` ability. To block a resource a broader role would otherwise allow, write an `Effect: "Deny"` statement instead.

</Step>
<Step title="Turn on two-factor auth">

Run this as the signed-in account.

::: code-group
```bash [Setup]
levelrail-cli auth 2fa setup
# → secret + otpauth:// provisioning URI, scan it into an authenticator app
```
```bash [Confirm with TOTP code]
levelrail-cli auth 2fa enable --code 123456
# → 10 recovery codes, shown once, store them somewhere safe
```
:::

</Step>
<Step title="Invite a teammate by email">

The invitee sets their own password.

```bash
levelrail-cli invites create --email priya@example.com --role viewer
```

```json
{ "id": "inv_1", "email": "priya@example.com", "role": "viewer", "link": "https://your-control-plane/accept-invite?token=...", "expires_at": "..." }
```

</Step>
<Step title="Log the CLI in with the device-code flow">

Use this from a machine with no direct HTTPS access to the control plane. It works over plain HTTP.

```bash
levelrail-cli auth login --device
# → prints a user_code and a URL; approve it from
#   /settings/cli-access on a browser that already has a session
```

</Step>
<Step title="Review who changed what">

```bash
levelrail-cli audit-log --client-kind cli --method POST
levelrail-cli audit-log --format csv --output-file audit-export.csv
```

</Step>
</Steps>

## API reference

**Sessions and account**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | public (first user only, setup token required) |
| `GET` | `/api/v1/auth/setup-status` | public |
| `POST` | `/api/v1/auth/login` | public |
| `POST` | `/api/v1/auth/logout` | session |
| `GET` | `/api/v1/auth/whoami` | session or bearer token |
| `GET` | `/api/v1/auth/session` | session |
| `PUT` | `/api/v1/auth/password` | session |
| `POST` | `/api/v1/auth/sessions/revoke-others` | session |

**Two-factor authentication**

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/auth/2fa` | session |
| `POST` | `/api/v1/auth/2fa/setup` | session |
| `POST` | `/api/v1/auth/2fa/confirm` | session |
| `POST` | `/api/v1/auth/2fa/disable` | session |
| `POST` | `/api/v1/auth/2fa/recovery-codes/regenerate` | session |
| `POST` | `/api/v1/auth/2fa/verify` | public (mfa_token required) |

**Passkeys**

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/auth/passkeys` | session |
| `POST` | `/api/v1/auth/passkeys/register/begin` | session |
| `POST` | `/api/v1/auth/passkeys/register/finish` | session |
| `DELETE` | `/api/v1/auth/passkeys/{id}` | session |
| `POST` | `/api/v1/auth/passkey-login/begin` | public |
| `POST` | `/api/v1/auth/passkey-login/finish` | public |

**OAuth sign-in**

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/auth/oauth/providers` | public |
| `GET` | `/api/v1/auth/oauth/{provider}/start` | public |
| `GET` | `/api/v1/auth/oauth/{provider}/callback` | public |
| `GET` | `/api/v1/auth/oauth/{provider}/link/start` | session |
| `GET` | `/api/v1/settings/oauth` | `read` |
| `PUT` | `/api/v1/settings/oauth/{provider}` | `root` |

**Device login (CLI)**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/device/start` | public |
| `POST` | `/api/v1/auth/device/token` | public |
| `GET` | `/api/v1/auth/device/requests` | session |
| `POST` | `/api/v1/auth/device/{user_code}/approve` | session |
| `POST` | `/api/v1/auth/device/{user_code}/deny` | session |

**Users and roles**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/users` | `root` |
| `GET` | `/api/v1/users` | `read` |
| `PUT` | `/api/v1/users/{id}/abilities` | `root` |
| `DELETE` | `/api/v1/users/{id}` | `root` |
| `GET` | `/api/v1/roles` | `read` |

**API tokens** (all session-only, no ability tier applies)

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/tokens` | session |
| `GET` | `/api/v1/auth/tokens` | session |
| `DELETE` | `/api/v1/auth/tokens/{id}` | session |

**Session links**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/session-links` | `root` |
| `GET` | `/api/v1/auth/session-links/{token}/consume` | public (token required) |

**Invites**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/invites` | `write` |
| `GET` | `/api/v1/invites` | `read` |
| `DELETE` | `/api/v1/invites/{id}` | `write` |
| `POST` | `/api/v1/invites/accept` | public (invite token required) |

**IAM policies**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/iam/policies` | `root` |
| `GET` | `/api/v1/iam/policies` | `read` |
| `GET` | `/api/v1/iam/policies/{id}` | `read` |
| `PUT` | `/api/v1/iam/policies/{id}` | `root` |
| `DELETE` | `/api/v1/iam/policies/{id}` | `root` |
| `GET` | `/api/v1/iam/policies/{id}/attachments` | `read` |
| `POST` | `/api/v1/iam/policies/{id}/attachments` | `root` |
| `DELETE` | `/api/v1/iam/policies/{id}/attachments/{principal_type}/{principal_id}` | `root` |

**Audit log**

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/audit-log?limit=&before=&path=&method=&client_kind=&format=` | `root` |
| `POST` | `/api/v1/audit-log/purge` | `root` |

## CLI

```bash
# Users and roles
levelrail-cli users list
levelrail-cli users create --email EMAIL --password PASSWORD (--role admin|operator|viewer | --abilities LIST)
levelrail-cli users set-abilities <id> (--role ROLE | --abilities LIST)
levelrail-cli users delete <id>
levelrail-cli users roles

# IAM policies
levelrail-cli iam policies create --name NAME --document DOC   # DOC: inline JSON or file://path
levelrail-cli iam policies list
levelrail-cli iam policies get <id>
levelrail-cli iam policies update <id> --name NAME --document DOC
levelrail-cli iam policies delete <id>
levelrail-cli iam policies attach <id> --principal-type user|token --principal-id ID
levelrail-cli iam policies detach <id> --principal-type user|token --principal-id ID
levelrail-cli iam policies attachments <id>

# Invites
levelrail-cli invites create --email EMAIL (--role ROLE | --abilities LIST)
levelrail-cli invites list
levelrail-cli invites revoke <id>

# API tokens (session-only: prompts for --username/--password)
levelrail-cli tokens create --name NAME (--abilities LIST | --preset observer|deployer|operator) [--expires-in-days N] [--agent NAME]
levelrail-cli tokens list
levelrail-cli tokens revoke <id>

# Auth: login, identity check, two-factor
levelrail-cli auth login [--device] [--token-name NAME] [--abilities LIST] [--expires-in-days N]
levelrail-cli auth whoami
levelrail-cli auth session-link
levelrail-cli auth 2fa status
levelrail-cli auth 2fa setup
levelrail-cli auth 2fa enable --code CODE
levelrail-cli auth 2fa disable (--code CODE | --recovery-code CODE)
levelrail-cli auth 2fa recovery-codes --code CODE

# OAuth sign-in configuration
levelrail-cli settings oauth list
levelrail-cli settings oauth set <google|github|oidc> --client-id ID --client-secret SECRET [--enabled=false] [--issuer-url URL] [--allowed-email-domain DOMAIN]

# Audit log
levelrail-cli audit-log [--limit N] [--before TIME] [--path PATH] [--method METHOD] [--client-kind cli|dashboard|mcp|api] [--format csv] [--output-file FILE]
levelrail-cli audit-purge
```

::: details Known limits

- **No per-team or per-project access boundary**
  IAM policies scope to individual resources (`app:name`, `database:name`) or a wildcard. There is no organization- or project-level grouping in the permission model. The Organizations settings page groups projects for display and navigation only; it is unrelated to access control.

- **No SSO/SAML and no SCIM provisioning**
  OAuth covers Google, GitHub, Microsoft, and generic OIDC. Nothing beyond that today.

- **No policy dry-run or simulation**
  A newly attached Deny statement takes effect on the very next request. The only way to check its effect is to make that request and see what happens.

:::

## Next steps

<CardGroup :cols="2">
<Card title="Security overview" href="/security">

How secrets, sessions, TLS and access control fit together.

</Card>
<Card title="AI assistant integration" href="/ai-assistant">

Scope an API token for an MCP client or an AI agent.

</Card>
<Card title="CLI reference" href="/cli-reference">

API tokens and device-code authentication workflows.

</Card>
<Card title="Managing databases" href="/managing-databases">

Backup target credentials use envelope encryption, gated at `write:sensitive`.

</Card>
</CardGroup>
