---
description: Per-team idle timeout, maximum session age, IP allowlist and required two-factor sign-in, enforced on every request, with a host-side recovery path for lockouts.
---

# Session policy

A session policy belongs to a team. Every authenticated request is checked against the policies of all the teams the caller belongs to, before any handler runs. Someone in several teams gets the strictest combination: the shortest timeouts, every allowlist, and MFA if any team requires it.

<InlineToc default-open />

## Rules

| Rule | Effect | Applies to |
| --- | --- | --- |
| Idle timeout | A session unused for this long is ended (at least 5 minutes) | Sessions |
| Maximum session age | A session older than this is ended, however active (at least 15 minutes) | Sessions |
| Allowed networks | Requests from any other address are refused with `ip_not_allowed`. One IP or CIDR per entry, IPv4 or IPv6, up to 100 | Sessions and API tokens |
| Require two-factor sign-in | A user without an authenticator app or passkey can only reach the enrollment and sign-out routes until they add one (`mfa_enrollment_required`). SAML sign-ins count as MFA, since the IdP handles it | Sessions |

An ended session answers 401 with `session_idle_timeout` or `session_max_lifetime` and is revoked, so the dashboard returns to the sign-in page. These rules sit on top of `APP_SESSION_TTL` and `APP_AUTH_ENGINE_SESSION_IDLE_TIMEOUT` from [Sign-in engine](auth-engine.md); whichever is shorter wins.

If the policy cannot be read, requests are refused with 503 rather than let through.

## Configure

Open **Settings, Session policy**, choose **Edit policy** on a team, and save. Or:

```bash
levelrail-cli session-policy set org_default --idle 30m --max-age 12h --allow 203.0.113.0/24 --allow 2001:db8::/32 --require-mfa
levelrail-cli session-policy list
levelrail-cli session-policy effective
levelrail-cli session-policy clear org_default
```

`set` replaces the whole policy: a rule you leave out is turned off. Every user is a member of the default team (`org_default`), so its policy is effectively instance-wide.

### Lockout protection

Saving is refused when the new policy would block the admin saving it: your own current address must be allowed, and you must have a second factor before requiring one. **Your address** at the top of the page is the address the server sees.

Behind a reverse proxy, the address comes from `X-Forwarded-For` only when the TCP peer is loopback or listed in `APP_INGRESS_TRUSTED_PROXIES`. Otherwise the peer address is used and forwarded headers are ignored, so a client cannot spoof its way into an allowlist.

## Recovering from a lockout

If an allowlist or MFA requirement locks everyone out, remove every session policy from the host. This opens the data directory directly, so it works without network access to the API, and it is written to the audit log:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail access-policy-reset --yes
```

Then sign in again. `recover-admin` still resets an admin password, and `emergency-token` still mints a root token, but that token is subject to the allowlist like any other, so run `access-policy-reset` first if your address is blocked. See [Emergency access](emergency-access.md).

## Things to know

- Existing API tokens keep working without MFA; the MFA rule applies to dashboard sessions. Tokens are still bound by the allowlist.
- Idle time is tracked to about 30 seconds of precision.
- Removing a user from a team removes that team's rules for them at their next request.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/security/session-policies` | `root` |
| `PUT` | `/api/v1/security/session-policies/{team_id}` | `root` |
| `DELETE` | `/api/v1/security/session-policies/{team_id}` | `root` |
| `GET` | `/api/v1/security/session-policy/effective` | any signed-in user |

The body of a `PUT` is `{"idle_timeout": "30m", "max_lifetime": "12h", "ip_allowlist": ["203.0.113.0/24"], "require_mfa": true}`.
