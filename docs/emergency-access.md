---
title: "Emergency access"
description: "Get back into a control plane you cannot sign in to: reset the admin password, mint a root token from the host, or use a break-glass token made earlier."
"og:url": "https://docs.levelrail.com/emergency-access"
---

If sign-in is broken (a lost password, passkeys that cannot work without HTTPS, a lockout after failed attempts, a stale session), you do not need to reinstall. Every route below needs root on the host that runs the control plane, the same trust boundary as the data directory itself. None of them is reachable over the network, so none of them widens the attack surface.

## Reset the admin password

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail recover-admin --username admin
```

This sets a new random password and prints it once. It also ends that account's sessions and clears its login lockout. Use it when you can reach the sign-in page but cannot get past it.

## Mint a short-lived root token

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail emergency-token --ttl 1h
```

This prints a root API token once and never stores it in plaintext. Use it when the dashboard is unusable but you still need to fix something, for example enabling HTTPS or creating a user:

```bash
APP_API_URL=http://<server>:8080 APP_API_TOKEN=<token> levelrail-cli settings ingress get
```

What keeps it safe:

- Host access only: it opens the data directory directly, so it cannot be called over HTTP.
- Short lived: the default is one hour. `--ttl` accepts 1 minute up to 24 hours, and `APP_EMERGENCY_TOKEN_MAX_TTL` raises or lowers that cap on the control plane.
- Visible and revocable: it appears as `emergency (host)` under **Settings, CLI Access** with its expiry, and you can revoke it there before it runs out.
- Audited: every mint is written to the audit log and the control plane log.
- Owned by an admin: it acts as the named admin (`--username`) or the first admin, never as an anonymous credential.

## Make a break-glass token in advance

While you can still sign in, create a token in **Settings, CLI Access** named `break-glass`, with the abilities you would need and an expiry (30 days is a reasonable rotation). Store it in your password manager, outside the server. It works from anywhere and does not need host access, so treat it like a root password: scope it, expire it, and revoke it once used.

## Other recovery routes

| Situation | Route |
| --- | --- |
| First run, no admin yet | `sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token` (or `docker compose exec levelrail levelrail setup-token`) prints the one-time setup token |
| Bad upgrade | Restore the pre-migration snapshot with `levelrail restore-db` |
| Session policy blocks your address | `levelrail access-policy-reset --yes` removes every session policy |
| CLI already logged in | `levelrail-cli auth login --device`, approved from any signed-in session |

## Locked out by a session policy

An IP allowlist or a required second factor in a [session policy](session-policy.md) can lock everyone out, including holders of an emergency token. Remove every session policy from the host, then sign in again:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail access-policy-reset --yes
```

The reset is written to the audit log.

## Single sign-on is broken

When [SAML single sign-on](sso-saml.md) is required, root users keep password sign-in on purpose: it is the break-glass path when the identity provider is down or misconfigured. Sign in as root with the password, fix or disable the connection, or turn off **Require SSO for non-root users**. `recover-admin` above resets that password if it is lost.

## Passkeys and HTTPS

Passkeys only work on an HTTPS dashboard URL. On a fresh server without HTTPS, sign-in falls back to the password automatically. Turn on HTTPS under **Domains** and passkeys become available.

## See also

Roles and access control are covered in [Access control](/access-control).
