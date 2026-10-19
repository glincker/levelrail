---
title: "Sign-in engine"
description: How sign-in, API tokens, two-factor, passkeys and OAuth are served, and the settings that tune them.
---

Sign-in, sessions, API tokens, device login, two-factor codes, passkeys and OAuth all run on one auth library built into the control plane. On a new install there is nothing to enable or migrate. The first account is created from the setup token and goes straight into the library.

Check what is available with `levelrail-cli auth-engine status` (root only) or on Settings > Security. It reports the library version and whether two-factor, passkeys and OAuth are available.

## Requirements

| Feature | Needs |
| --- | --- |
| Two-factor codes | The control plane master key |
| Passkeys | A dashboard URL set to a domain (or `APP_AUTH_ENGINE_WEBAUTHN_RP_ID`). An IP address cannot be a relying party. Changing the URL needs a restart |
| OAuth and OIDC sign-in | The master key, and `APP_AUTH_ENGINE_BASE_URL` set to the public dashboard URL |

## Settings

All optional.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_AUTH_ENGINE_BASE_URL` | derived from the listen address | Public URL used in generated links and OAuth callbacks |
| `APP_AUTH_ENGINE_PATH_PREFIX` | `/api/v1/auth-lib` | Route prefix of the library's own routes |
| `APP_AUTH_ENGINE_PASSWORD_MIN_LENGTH` | `8` | Minimum password length |
| `APP_AUTH_ENGINE_DEVICE_POLL_INTERVAL` | `5s` | Device login poll interval |
| `APP_DEVICE_TOKEN_TTL_DAYS` | `30` | Lifetime of a device login token |
| `APP_DEVICE_TOKEN_ALLOW_ROOT` | `false` | Let a root approver mint a root device token |
| `APP_SESSION_TTL` | `24h` | Absolute session lifetime |
| `APP_AUTH_ENGINE_SESSION_IDLE_TIMEOUT` | `8h` (`0` turns it off) | End a session unused for this long |
| `APP_AUTH_ENGINE_SESSION_LINK_TTL` | `2m` | How long a session link can be exchanged |
| `APP_AUTH_ENGINE_LOGIN_GRACE_FAILURES` | `3` | Free failed logins per IP and username before backoff |
| `APP_AUTH_ENGINE_LOGIN_BASE_DELAY` | `1s` | First backoff delay, doubled per further failure |
| `APP_AUTH_ENGINE_LOGIN_MAX_DELAY` | `15m` | Backoff cap |
| `APP_AUTH_ENGINE_LOGIN_RESET_AFTER` | `15m` | Forget failures after this idle time |
| `APP_AUTH_ENGINE_LOGIN_USER_MAX_FAILURES` | `10` | Consecutive failures on one account that lock it |
| `APP_AUTH_ENGINE_LOGIN_USER_LOCKOUT` | `15m` | How long that lock lasts |
| `APP_AUTH_ENGINE_RATE_LIMIT_PER_IP` | `60` | Credential requests per minute per IP |
| `APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL` | `60` | Credential requests per minute per username |
| `APP_AUTH_ENGINE_STREAM_WATCH_INTERVAL` | `5s` | How often a live stream re-checks its session |
| `APP_AUTH_ENGINE_WEBAUTHN_RP_ID` | host of the dashboard URL | Passkey relying party ID |
| `APP_AUTH_ENGINE_WEBAUTHN_ORIGINS` | the dashboard URL origin | Extra allowed passkey origins, comma separated |
| `APP_AUTH_ENGINE_WEBAUTHN_REQUIRE_UV` | `false` | Demand PIN or biometric on every passkey use |
| `APP_AUTH_ENGINE_WEBAUTHN_CLONE_WARNING` | `reject` | `reject` refuses a sign-in whose counter went backwards, `flag` allows it and audits |
| `APP_AUTH_ENGINE_MFA_MAX_FAILURES` | `5` | Wrong codes per user before a lockout |
| `APP_AUTH_ENGINE_MFA_LOCKOUT` | `15m` | How long that lockout lasts |
| `APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS` | empty | Extra hosts allowed in an OAuth redirect URI once a dashboard URL is set |
| `APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL` | `30s` | How long a resolved OAuth provider is cached. Saving provider settings drops it at once |

## Upgrading from a release before the auth library

The auth library replaced the old built-in sign-in, and that change breaks existing installs (the platform had no production users when it landed). After upgrading from `v0.2.0-beta.18` or earlier:

- Everyone is signed out. Sign in again with your existing username and password: accounts and password hashes carry over.
- API tokens created before the upgrade stop working (they answer 401). Create new ones in **Settings, CLI Access**, or run `levelrail-cli auth login --device`, and update CI secrets.
- Two-factor codes, passkeys and recovery codes are not carried over. Re-enroll them after signing in.

Take a backup of the data directory before upgrading, as the installer's upgrade notes recommend. Each start also writes a pre-migration database snapshot you can restore with `levelrail restore-db`.

## Things to know

- API tokens carry the `tk_` style prefix of the product. A token's effective abilities never exceed its owner's current abilities.
- A TOTP code can be used once. Repeated wrong codes lock the user out, counted across devices, and answer 429 with `Retry-After`.
- Recovery codes look like `XXXX-XXXX-XXXX-XXXX` and are accepted with or without hyphens and in any case.
- Sign-in is case-insensitive on the username. Usernames that are not an email address (for example `admin`) are still accepted at first-run and by invites.
- OAuth callback URL: register `https://<your-host>/api/v1/auth/oauth/<provider>/callback` at each identity provider. Sign-in and profile linking share it, and Levelrail tells them apart by the state value. Once a dashboard URL is set, redirect hosts are limited to it and the base URL host, plus any in `APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS`. The redirect URI is always built from the dashboard URL (or `APP_DASHBOARD_URL`). With neither set, the request's `Host` and `X-Forwarded-Proto` are used only from a loopback peer or one listed in `APP_INGRESS_TRUSTED_PROXIES`; any other caller gets a `400` until a dashboard URL is set.
- An existing account is linked on OAuth sign-in only when the provider reports the email as verified. A provider's allowed email domain gates new accounts only.
- `recover-admin` sets the password through the library, which also ends that account's sessions and clears its lockout. No restart is needed. Locked out another way? See [Emergency access](/emergency-access).

## Troubleshooting

### Passkeys not available, getting "Passkey registration is not available"

Likely cause: The control plane is running on an IP address instead of a domain name, or `APP_AUTH_ENGINE_WEBAUTHN_RP_ID` is not set.

How to confirm: Run `levelrail-cli auth-engine status` and check the "Passkeys" line. If it says "not available", passkeys are disabled. Check what URL is set: Dashboard Settings > Security or run `curl https://<control-plane-url>/api/v1/brand`.

Fix: Set your dashboard to a domain name (not an IP), or set `APP_AUTH_ENGINE_WEBAUTHN_RP_ID` to your domain and restart the control plane.

Still stuck: See [Configuration](/environment-variables) for how to set environment variables and restart.

### Two-factor codes not working or "Enrollment unavailable"

Likely cause: The master key is not configured. Two-factor, passkeys and OAuth all need the control plane's encryption master key to function.

How to confirm: Run `levelrail-cli auth-engine status` and check whether "2FA" and "OAuth" are available. Both should show "available (requires master key)". If the master key is missing, they will show "not available".

Fix: Set `APP_MASTER_KEY` to the master key value and restart the control plane. See [Master key rotation](/master-key-rotation) for how to set or recover the master key.

Still stuck: Read [Master key](/master-key-rotation) if you lost the master key.

### OAuth callback failing with "redirect_uri_mismatch"

Likely cause: The OAuth redirect URI registered at the provider does not match what the control plane is sending. The platform builds the URI from the dashboard URL, and the host must match exactly (including `www.`).

How to confirm: Check your OAuth provider's console for which URIs are registered. On the control plane, run `curl https://<control-plane-url>/api/v1/brand` and note the exact domain in "domain". Register `https://<that-domain>/api/v1/auth/oauth/<provider>/callback` at the provider.

Fix: Update the registered redirect URI at your OAuth provider to exactly match `https://<dashboard-domain>/api/v1/auth/oauth/<provider>/callback`. If you need to use a different hostname, set `APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS` to comma-separated allowed hosts.

Still stuck: See [SSO and SAML](/sso-saml) for identity provider setup.

### API token returns 401 "Unauthorized" after an upgrade

Likely cause: You upgraded from v0.2.0-beta.18 or earlier, and API tokens created before the upgrade are no longer valid (the auth library replaced the old sign-in system).

How to confirm: Check when the token was created. If it was before the upgrade, it is obsolete. Run `levelrail-cli auth login --device` to mint a new token.

Fix: Create a new API token in Settings > CLI Access (dashboard) or by running `levelrail-cli auth login --device`, then update your CI or scripts to use the new token.

Still stuck: See [API reference](/api-reference) for the API surface.
