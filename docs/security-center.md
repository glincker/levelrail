---
description: The security center scores your instance from its real state, lists what to fix first with a one-click fix or a link, manages every session and device, and sets sign-in approval and API token policy.
---

# Security center

The security center is one page, **Security center** in the sidebar (`/security`), that answers three questions: how exposed is this instance right now, what should I fix first, and who is signed in. It reads the control plane's own state (accounts, tokens, sessions, settings, certificates, the exposure audit) and never calls anything outside your servers.

<InlineToc default-open />

## Security score and checklist

`GET /api/v1/security/posture` returns a score from 0 to 100, a grade, and counts by severity. Every check that fails costs points: 25 for critical, 15 for high, 8 for medium and 3 for low. A check that could not be read shows as unknown and costs nothing, so a missing data source never makes the score look worse or better than it is.

Anyone with `read` gets the score and counts, plus a short checklist for their own account (two-factor or passkey, recovery codes, a reset flag). Admins (`root`) also get the full platform checklist with the names of the accounts, tokens, domains and nodes behind each failing check. The result is cached for `APP_SECURITY_POSTURE_CACHE_TTL` (default 30s); the exposure audit inside it, which may ask remote agents for their containers, refreshes every `APP_SECURITY_POSTURE_EXPOSURE_TTL` (default 5m). Changing the policy, revoking sessions or a "this wasn't me" report refreshes it at once.

| Check | Severity | Data source | Fix |
| --- | --- | --- | --- |
| Admins without two-factor or a passkey | critical | users with `root`, TOTP status, registered passkeys | Settings > Security |
| Accounts flagged for a new password | critical | "this wasn't me" reports | change the password |
| The Docker API is reachable from the internet | critical | exposure audit findings for ports 2375 and 2376 | Firewall, `firewall exposure` |
| API tokens that hold root | high | live tokens | Settings > API tokens |
| New browser approval is off | high | `APP_AUTH_NEW_DEVICE_APPROVAL` | set it back to `true` |
| Container ports open to the internet | high | exposure audit | Firewall, `firewall exposure` |
| Off-box control plane backups are off | high | disaster recovery status | Control plane backup |
| Domains without a publicly trusted certificate | high | routed domains, stored certificates, ACME setting | Domains |
| Bursts of failed sign-ins | high | failed sign-in counter, see below | Audit log |
| Accounts with no recovery codes left | medium | TOTP status | Settings > Security |
| API tokens that never expire | medium | live tokens | one click: limit new tokens to 90 days |
| API tokens that can approve sign-ins | medium | live tokens holding `signin:approve` | Settings > API tokens |
| Admins can sign in with a code | medium | `auth.code_login` | one click: turn it off |
| HSTS is off while real certificates are in place | medium | HSTS setting and certificates | Domains |
| Node agents older than the minimum version | medium | each node's reported agent version | Nodes |
| No admin has a passkey | low | registered passkeys | Settings > Security |
| API tokens unused for a long time | low | last use, `warn_unused_days` | Settings > API tokens |
| Old sessions still signed in | low | sessions older than `APP_SECURITY_SESSION_MAX_AGE_DAYS` (default 7) | Sessions tab |
| Only password sign-ins need new browser approval | low | `auth.approval_scope` | one click: every method |
| The master key is missing or overdue for rotation | low | rotation history, `APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS` | `secrets rotate-master-key` |
| Secrets older than 90 days | low | secret update times | rotate the secret |

A domain counts as served without public TLS when ACME is off, or when its only certificate is from the control plane's internal issuer or has expired. A proxy in front that terminates TLS (`tls_terminated_upstream`) passes the check.

From the CLI:

```bash
levelrail-cli security posture
levelrail-cli security posture --json
```

## Sessions and devices

The **Sessions and devices** tab, `GET /api/v1/security/sessions`, and `levelrail-cli security sessions list` show:

- every live session of the account: browser (a short label like "Firefox on Linux" built from a fixed list, never the raw User-Agent), the network it signed in from (the /24 or /48 of its address, which is the only location this platform knows: no GeoIP database is used), when it signed in, when it was last active, and which one is this browser;
- trusted browsers, which skip new browser approval;
- the account's API tokens with when each was last used.

**Sign out** ends one session (`DELETE /api/v1/security/sessions/{id}`, `security sessions revoke <id>`). **Sign out all other sessions** (`POST /api/v1/security/sessions/revoke-others`, `security sessions revoke-others`) keeps the browser you are using and also resets trusted browsers, waiting sign-ins and `signin:approve` tokens, like changing the password does.

An admin can pick another account in the tab, or pass `--user <user id>` (`?user_id=` on the API), to see and end its sessions. Every revoke is audited (`security.session_revoked`, `security.sessions_revoked`), with the target account when it is not the caller's own. Anyone else asking for a session that is not theirs gets a not found answer, so session ids cannot be probed. A token used for these routes must belong to a user and hold `write:sensitive` or `signin:approve`.

## Sign-in protection

### Approval scope

New browser approval (see [Identity and access](identity-and-access.md#new-browser-approval)) always applies to password sign-ins. `auth.approval_scope` decides whether it also applies to passkey and OAuth sign-ins:

| Value | Password | Passkey | OAuth / OIDC |
| --- | --- | --- | --- |
| `password_only` (default) | held | not held | not held |
| `all_methods` | held | held | held |

A sign-in is only held when the account already has another live session, so the last way in is never blocked. A held OAuth sign-in returns to the sign-in page, which shows the number to type in the approving session. Set it in the **Policy** tab, with `levelrail-cli security policy set --approval-scope all_methods`, or `PUT /api/v1/security/policy` (root). `APP_AUTH_APPROVAL_SCOPE` sets the default before anyone saves one. If the policy cannot be read, the sign-in is held, never waved through.

Each account can also turn on **Require approval for new browsers on my account** in the Policy tab (`PUT /api/v1/security/account`). That applies approval to every method for that account whatever the platform scope is. Turning it off needs a signed-in dashboard session; a token can only turn it on.

**Break-glass.** `APP_AUTH_NEW_DEVICE_APPROVAL=false` on the control plane, then a restart, turns approval off for every method and every account, whatever the scope and account switches say. Use it only when nobody can approve; set it back afterwards. `recover-admin` on the server still works.

### New sign-in alerts

When an account signs in from a browser and network it has not used before, its owner gets an email with the browser, network and time, and channels opted in to sign-in notices (the same switch as CLI login notices) get a short message. The first browser an account ever uses raises nothing, and a sign-in you approved yourself, or a session link, does not alert. At most `APP_SIGNIN_ALERTS_PER_ACCOUNT_PER_HOUR` (default 5) emails go out per account per hour. Known browsers are remembered as a hash of the browser label and network for `APP_KNOWN_BROWSER_RETENTION` (default 180 days).

The email carries a **this wasn't me** link. Opening it changes nothing; the page has one button. Pressing it (`POST /api/v1/auth/sign-in-alert/disown`) signs that session out, removes every trusted browser of the account, cancels its waiting sign-ins and flags the account for a new password. The flag shows in the checklist and the attention center until the password is changed or reset. The link:

- is an HMAC over a random id with a key derived from the control plane's encryption key, so it cannot be forged or altered;
- works once, recorded in the database before anything is revoked;
- expires after `APP_SIGNIN_ALERT_LINK_TTL` (default 6 hours);
- is rate limited per address and only accepted as a same-origin JSON request;
- is sent only to the account's own email, never to a channel, since anyone reading a channel could use it.

### Failed sign-in signals

Failed password sign-ins are counted per typed account name and per address over `APP_SECURITY_FAILED_LOGIN_WINDOW` (default 15 minutes). Reaching `APP_SECURITY_FAILED_LOGIN_ACCOUNT_THRESHOLD` (default 10) for one account or `APP_SECURITY_FAILED_LOGIN_IP_THRESHOLD` (default 20) from one address writes one audit row (`security.failed_logins`), posts one notice to opted-in channels, and adds an attention item (to admins, and to the account owner for their own account). A burst raises one alert per window, not one per attempt. The counts live in memory, so a restart starts them again. This is a signal only: the login throttle and account lockout in the auth engine are what slow guessing down. There is no "new country" signal, since the control plane has no GeoIP data.

## API token hygiene

Three policy settings, each in the Policy tab, `security policy set`, or `PUT /api/v1/security/policy`:

| Setting | Env default | What it does |
| --- | --- | --- |
| `max_token_lifetime_days` | `APP_TOKEN_MAX_LIFETIME_DAYS` (none) | new tokens must set `expires_in_days` from 1 to this; existing tokens are only flagged in the checklist |
| `warn_unused_days` | `APP_TOKEN_WARN_UNUSED_DAYS` (30) | tokens unused this long are listed in the checklist |
| `disable_unused_days` | `APP_TOKEN_DISABLE_UNUSED_DAYS` (off) | tokens unused this long are disabled after a notice |

The sweep runs every `APP_TOKEN_HYGIENE_SWEEP_INTERVAL` (default 1h). A token unused for `disable_unused_days` first gets a notice: an audit row (`security.token_unused_notice`), a channel message and an attention item for its owner and admins. If it is still unused `APP_TOKEN_HYGIENE_NOTICE_GRACE` (default 7 days) later, it is revoked (`security.token_unused_disabled`). Using the token at any point clears the notice. Tokens that belong to no user (minted by the platform itself) are never swept. A value of 0 turns a setting off.

```bash
levelrail-cli security policy get
levelrail-cli security policy set --max-token-lifetime-days 90 --disable-unused-days 120
```

## Audit events

`security.session_revoked`, `security.sessions_revoked`, `security.policy_updated`, `security.device_approval_changed`, `security.new_browser_sign_in`, `security.sign_in_disowned`, `security.token_unused_notice`, `security.token_unused_disabled` and `security.failed_logins`.

## Next steps

<CardGroup :cols="2">
<Card title="Identity and access" href="/identity-and-access">

Users, roles, tokens, sign in with a code and new browser approval.

</Card>
<Card title="Exposure audit" href="/exposure-audit">

Find and restrict container ports the internet can reach.

</Card>
</CardGroup>
