---
description: How sign-in, API tokens, two-factor, passkeys and OAuth are served, and the settings that tune them.
---

# Sign-in engine

Sign-in, sessions, API tokens, device login, two-factor codes, passkeys and OAuth sign-in all run on one auth library embedded in the control plane. There is nothing to enable or migrate on a new install. The first account is created from the setup token, and it lands directly in the library.

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
| `APP_AUTH_ENGINE_SESSION_IDLE_TIMEOUT` | `0` (off) | End a session unused for this long |
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
| `APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL` | `30s` | How long a resolved OAuth provider is cached. Saving provider settings drops it at once |

## Things to know

- API tokens carry the `tk_` style prefix of the product. A token's effective abilities never exceed its owner's current abilities.
- A TOTP code can be used once. Repeated wrong codes lock the user out, counted across devices, and answer 429 with `Retry-After`.
- Recovery codes look like `XXXX-XXXX-XXXX-XXXX` and are accepted with or without hyphens and in any case.
- Sign-in is case-insensitive on the username. Usernames that are not an email address (for example `admin`) are still accepted at first-run and by invites.
- OAuth callback URL: register `https://<your-host>/api/v1/auth-lib/providers/<provider>/callback` at each identity provider. Linking a provider to your account from your profile still uses `/api/v1/auth/oauth/<provider>/callback`, so register that one too.
- An existing account is linked on OAuth sign-in only when the provider reports the email as verified. A provider's allowed email domain gates new accounts only.
- `recover-admin` sets the password through the library, which also ends that account's sessions and clears its lockout. No restart is needed.
