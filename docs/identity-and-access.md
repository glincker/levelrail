# Identity and access: users, roles, and IAM policies

Who can sign in, what they can do once they're in, and a record of what
they actually did. Packages: `internal/api/{auth,users,roles,abilities,
iam,iam_handlers,invites,tokens,twofactor,oauth,oauth_settings,
device_auth,audit,audit_retention}.go`, `cmd/levelrail-cli/{users,iam,
invites,tokens,auth}*.go`, `web/src/routes/settings/{users,iam-policies,
security,tokens,oauth,cli-access,audit-log}.tsx`.

## Why two permission models instead of one

Most self-hosted platforms pick one of two shapes and live with its
downside. A flat role list (admin/member/viewer) is easy to reason about
but can't express "this CI token may deploy exactly one app and nothing
else." A full IAM-style policy engine can express that but is overkill
for the 90% case of "give Priya everything, give the contractor
read-only."

This platform keeps both, layered:

- **Abilities** are a flat, six-string vocabulary (`read`,
  `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`) that
  every user and every API token carries directly. **Roles**
  (`admin`/`operator`/`viewer`) are nothing more than named presets over
  that same list, a convenience for the common case, never a second
  storage location.
- **IAM policies** are optional, additive documents of Allow/Deny
  statements scoped to one resource (`app:web`, `database:main`, or
  `*`), attachable to a user or a token. They exist for the narrow case
  a flat ability can't express: granting (or explicitly revoking) access
  to one specific app without touching the principal's own ability list.

An explicit Deny in an attached policy always wins, even over `root`.
Everything else falls back to the flat abilities exactly as if no policy
existed.

## How it actually works

### Principals: a session, or a token

Every gated route (`requireAbility`, `internal/api/auth.go`) resolves the
request to exactly one principal before deciding anything: a session
cookie mapped to a `store.User` row, or a bearer token's own stored
`Abilities`. Session lookups are an in-memory map keyed by an opaque
token (`sessionStore`), TTL 24h by default, overridable with
`APP_SESSION_TTL` (a Go duration string). A restart clears every live
session, the same accepted tradeoff a single-node control plane makes
elsewhere.

The session cookie is set `Secure`, deliberately, even though the
control plane's own HTTP listener speaks plain HTTP: embedded Caddy is
expected to terminate TLS in front of it. Hitting the control plane
directly over `http://` (the common local-dev case) means the cookie
never round-trips back on a follow-up request, which is a real,
load-bearing gap `levelrail-cli auth login` inherits (see "Not built
yet" below).

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

A policy document is AWS IAM's own shape, hand-typed or generated:

```json
{
  "Statement": [
    { "Effect": "Allow", "Action": ["deploy"], "Resource": ["app:checkout"] },
    { "Effect": "Deny", "Action": ["write:sensitive"], "Resource": ["*"] }
  ]
}
```

`Action` entries are ability strings or `*`; `Resource` entries are
`app:<name>`, `database:<name>`, or `*`, with a trailing `*` matching as
a prefix (`app:*` matches every app). Evaluation order
(`authorizeResource`, `internal/api/iam.go`), checked only on the two
routes that are resource-scoped today (apps and databases):

1. An explicit **Deny** matching the ability and resource, in any
   attached policy, always wins. Full stop, even for a `root` principal.
2. Otherwise, the principal's own flat abilities decide, unchanged from
   having no policies attached at all.
3. Otherwise, an explicit **Allow** in an attached policy can still grant
   access the principal's flat abilities don't, scoped to that one
   resource.

A policy attaches to a `user` or a `token` (`principal_type`), by that
principal's own id. A malformed stored document can never grant or deny
anything, it's treated as if it simply doesn't mention the pair being
checked.

### Bootstrap: exactly one path to the first admin

On startup, `BootstrapAdmin` creates a user from `APP_ADMIN_USERNAME` /
`APP_ADMIN_PASSWORD` only if zero users exist yet; it's a no-op on every
later restart, so a password change doesn't get silently reverted.
Without those env vars set and no user yet, the control plane still
starts (it serves the public `/api/v1/brand` endpoint) but every
auth-required route stays inaccessible until an operator sets them and
restarts, or calls `POST /api/v1/auth/register` by hand.
`POST /api/v1/auth/register` is gated at the database layer to succeed
exactly once (a unique constraint, not just an application check), so a
race between two concurrent first-registration attempts can't create two
"first" admins. Either path grants `AbilityRoot`: there's no one else yet
to grant anything narrower. (`APP_DEV_MODE=1` seeds a fixed `dev`/`dev`
admin for local development instead; never use it in a real deployment.)

Every user after the first is created by an existing root user, either
directly (`POST /api/v1/auth/users`) or via an accepted invite.

A root user cannot edit their own abilities or delete their own account
(`handleUpdateUserAbilities`, `handleDeleteUser`), and the last remaining
user can never be deleted. Both are hard 400s, not soft warnings: the
one rule standing between the platform and a permanent lockout.

### Two-factor authentication (TOTP)

Requires a master key configured on the control plane (`internal/secrets`);
without one, every 2FA route returns `501`. The flow:

1. `POST /api/v1/auth/2fa/setup` mints a fresh TOTP secret and stores it
   unconfirmed. Calling it again before confirming just overwrites the
   pending secret.
2. `POST /api/v1/auth/2fa/confirm` validates one live code against that
   secret, flips `TOTPEnabled`, and returns 10 recovery codes in
   plaintext, the one and only time they're ever shown.
3. From then on, `POST /api/v1/auth/login` returns `mfa_required: true`
   plus a short-lived `mfa_token` (5 minutes) instead of a session;
   `POST /api/v1/auth/2fa/verify` exchanges that token plus a live code
   (or a recovery code) for the real session cookie.

Disabling 2FA (`POST /api/v1/auth/2fa/disable`) re-verifies the second
factor itself, not the account password: a password-only re-check would
undermine the exact property 2FA exists to provide. Regenerating
recovery codes invalidates the entire previous set. Both the login-time
verify step and every setup/confirm/disable call are rate limited
(exponential backoff, a handful of free failures before it bites),
separately from the password rate limiter below.

### OAuth sign-in

Three providers, `google`, `github`, `oidc` (generic OpenID Connect,
requires an issuer URL). Settings are per-provider rows
(`GET`/`PUT /api/v1/settings/oauth[/{provider}]`), `AbilityRoot` to
change, enabling one requires a client ID and a client secret (the OIDC
provider also requires an issuer URL); the secret is write-only over the
API, `has_client_secret` is all a `GET` ever reveals.

Sign-in behavior (`completeOAuthSignin`, `internal/api/oauth.go`):

- An already-linked external identity signs in as its existing owner.
- A brand-new email auto-provisions a new user with `AbilityRead` only,
  the least-privilege default (never `root`, since a fresh OAuth signup
  is never the platform's first user). An existing root user grants more
  afterward via `PUT /api/v1/users/{id}/abilities`.
- An email that already belongs to a different, existing account is
  refused outright: auto-linking here would let an anonymous sign-in
  silently take over an unrelated account. Attaching a new provider to
  an account you already control only happens through
  `GET /api/v1/auth/oauth/{provider}/link/start`, which requires an
  existing live session first.
- If `AllowedEmailDomain` is set on a provider, a new signup outside that
  domain is refused.

### Team invites

`POST /api/v1/invites` mints a token for one named email, either
`--role` or `--abilities`, capped by a real privilege check: the caller
cannot invite someone with an ability they don't hold themselves
(`AbilityWrite` gates the route, but that per-ability cap is the actual
boundary). The response always includes the plaintext accept link,
whether or not SMTP is configured, so a control plane with no email
capability is still fully usable by copy/pasting the link. Default TTL
is 7 days (`APP_INVITE_TTL`). `POST /api/v1/invites/accept` creates a
normal local-password user through the exact same insertion path
`POST /api/v1/auth/users` uses, so an invited account is never
distinguishable from an admin-created one. A non-root caller only ever
sees invites they created themselves; a root caller sees every pending
invite.

### API tokens

Scoped, revocable bearer credentials for the CLI, CI, and MCP
integrations, minted with the same six-string ability vocabulary users
carry, plus an optional expiry (`expires_in_days`, 0 means never). The
plaintext is returned exactly once, at creation; every later read
(`GET /api/v1/auth/tokens`) shows only metadata.

Token management (`create`/`list`/`revoke`) is deliberately **session-
only**, never bearer-token authenticated: a token can never mint or
revoke another token on its own behalf. That's why the CLI's
`tokens create`/`list`/`revoke` and `auth login` all prompt for a
username and password rather than accepting `--token`, there is no
bearer token that could ever call those routes, however broadly scoped.

The device-code flow (`POST /api/v1/auth/device/start` and `/token`,
`levelrail-cli auth login --device`) exists specifically to sidestep the
plain-HTTP session-cookie gap above: the CLI polls with a random device
code, an operator approves it from the dashboard's CLI Access page
(`/settings/cli-access`) using their own already-established session,
and the resulting token inherits exactly that approving operator's own
abilities. The CLI never gets to pick.

### Audit log

Every request gated above `AbilityRead` (write, deploy, root-tier, plus
`read:sensitive` since reading a private credential isn't an ordinary
read) gets one row: actor type and id, resolved display name, the
ability checked, HTTP method and path, status code, remote address,
timestamp, and which caller surface it came from (`cli`, `dashboard`,
`mcp`, or `api`, sniffed from `User-Agent`). Recording is best-effort and
runs after the real request has already completed: a failed audit write
is logged and dropped, never turned into a failed request.

`GET /api/v1/audit-log` is cursor-paginated (`?before`, an RFC3339
timestamp), filterable by `?path`, `?method`, and `?client_kind`, and
`?format=csv` returns the identical rows as a downloadable attachment
instead of JSON, for compliance export. Retention defaults to 90 days
(`APP_AUDIT_LOG_RETENTION_DAYS`), swept automatically on an interval
(`APP_AUDIT_LOG_SWEEP_INTERVAL`) and purgeable on demand via
`POST /api/v1/audit-log/purge`.

## Integration walkthrough

1. **Bootstrap the first admin** (once, before anyone can sign in):

   ```bash
   export APP_ADMIN_USERNAME=admin@example.com
   export APP_ADMIN_PASSWORD='a-real-password'
   # restart the control plane, then:
   curl -s -c cookies.txt -X POST https://your-control-plane/api/v1/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"admin@example.com","password":"a-real-password"}'
   ```

2. **Create a teammate with a curated role**, no hand-picked abilities
   needed:

   ```bash
   levelrail-cli users create --email ops@example.com --password 'temporary-pw' --role operator
   ```

   ```json
   { "id": "user_abc123", "email": "ops@example.com", "role": "operator", "abilities": ["read", "read:sensitive", "write", "deploy"], ... }
   ```

3. **Scope a CI token down to one app** with an IAM policy, tighter than
   any flat role could express:

   ```bash
   levelrail-cli tokens create --name "ci-checkout" --abilities deploy
   # → token tok_xyz, plaintext shown once

   levelrail-cli iam policies create --name "checkout-only" \
     --document '{"Statement":[{"Effect":"Allow","Action":["deploy"],"Resource":["app:checkout"]}]}'
   # → policy pol_abc

   levelrail-cli iam policies attach pol_abc --principal-type token --principal-id tok_xyz
   ```

   That token can now deploy `checkout` even without a global `deploy`
   ability, or be explicitly denied a resource a broader role would
   otherwise allow, by writing an `Effect: "Deny"` statement instead.

4. **Turn on two-factor auth** for the signed-in account:

   ```bash
   levelrail-cli auth 2fa setup
   # → secret + otpauth:// provisioning URI, scan it into an authenticator app
   levelrail-cli auth 2fa enable --code 123456
   # → 10 recovery codes, shown once, store them somewhere safe
   ```

5. **Invite a teammate by email** instead of creating their password
   yourself:

   ```bash
   levelrail-cli invites create --email priya@example.com --role viewer
   ```

   ```json
   { "id": "inv_1", "email": "priya@example.com", "role": "viewer", "link": "https://your-control-plane/accept-invite?token=...", "expires_at": "..." }
   ```

6. **Log the CLI in from a machine with no direct HTTPS access** to the
   control plane (the device-code flow, works over plain HTTP):

   ```bash
   levelrail-cli auth login --device
   # → prints a user_code and a URL; approve it from
   #   /settings/cli-access on a browser that already has a session
   ```

7. **Review who changed what**:

   ```bash
   levelrail-cli audit-log --client-kind cli --method POST
   levelrail-cli audit-log --format csv --output-file audit-export.csv
   ```

## API reference

**Sessions and account**

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/auth/register` | public (first user only) |
| `POST` | `/api/v1/auth/login` | public |
| `POST` | `/api/v1/auth/logout` | session |
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
levelrail-cli tokens create --name NAME --abilities LIST [--expires-in-days N]
levelrail-cli tokens list
levelrail-cli tokens revoke <id>

# Auth: login, identity check, two-factor
levelrail-cli auth login [--device] [--token-name NAME] [--abilities LIST] [--expires-in-days N]
levelrail-cli auth whoami
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

## Not built yet (deliberate follow-ups)

- **`auth whoami` cannot work against a bearer token.**
  `GET /api/v1/auth/session` is session-cookie-only by design
  (`handleGetSession`'s own doc comment: "a bearer token has no session
  of its own to report on"), and the CLI only ever persists a bearer
  token. Running `levelrail-cli auth whoami` the normal way returns a
  real `401` every time; there's no bearer-token-compatible identity
  endpoint yet.
- **`auth login` (username/password) needs an HTTPS front.** The session
  cookie `POST /api/v1/auth/login` sets is `Secure`, so it never round-
  trips back to the token-minting step against a plain-HTTP target (the
  common local-dev default with no TLS-terminating Caddy in front yet).
  Use `--device` to avoid this entirely; it works over plain HTTP.
- **No per-team or per-project access boundary.** IAM policies scope to
  individual resources (`app:name`, `database:name`) or a wildcard;
  there's no organization- or project-level grouping in the permission
  model itself. The Organizations settings page groups projects for
  display and navigation only, it's unrelated to who can access what.
- **No SSO/SAML and no SCIM provisioning.** OAuth covers Google, GitHub,
  and generic OIDC; nothing beyond that today.
- **No policy dry-run or simulation.** A newly attached Deny statement
  takes effect on the very next request; the only way to check its
  effect is to make that request and see what happens.
