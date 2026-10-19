---
title: "Your account"
description: The Account page, your own profile, sign-in security, active sessions, API tokens and a plain-language view of what you can manage.
---

Settings, Account is the personal half of access: everything about your own sign-in, in one place.

## Sections

- **Profile**: your email and a display name you can edit inline.
- **Password**: change it; this signs out your other sessions.
- **Two-factor** and **Passkeys**: enrol or remove second factors. Passkeys show their provider, synced or device-bound status and last use, and can be renamed, tested and removed (see [Security](/security#passkeys)).
- **Active sessions and trusted devices**: every browser signed in as you. End one, or sign out all others. Trusted devices are browsers that skip the new-device check; remove any you no longer use.
- **Linked sign-in providers**: link an enabled identity provider to your account. Linking needs your live session, so a provider can never be attached by email alone.
- **My API tokens**: how many active tokens you hold, with a link to manage them.
- **My access**: your access level, teams, attached IAM policies and, for each area you cannot manage, the permission that is missing.

## From the API

| Route | What it does |
| --- | --- |
| `PATCH /api/v1/auth/profile` | Change your own display name (1 to 80 characters). Session only. |
| `GET /api/v1/auth/my-access` | Your tier, abilities, teams, linked providers, attached policies and per-area capabilities. |
| `GET /api/v1/security/sessions` | Your live sessions, trusted devices and tokens. Never returns token values. |
| `DELETE /api/v1/security/sessions/{id}` | End one of your sessions. |
| `POST /api/v1/auth/sessions/revoke-others` | End every session except this one. |

Every route here is scoped to the caller. Another user's sessions read as not found unless you are an admin using `?user_id=`.

## From the CLI

```bash
levelrail-cli auth whoami --access
```

Prints your access level, abilities, teams and attached policies, then one line per area: `can` or `cannot`, with the missing ability for the latter. Add `--json` for the raw response.
