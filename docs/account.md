---
title: "Your account"
description: The Account page, your own profile, sign-in security, active sessions, API tokens and a plain-language view of what you can manage.
---

Settings, Account is the personal half of access: everything about your own sign-in, in one place.

## Sections

- **Profile**: your email and a display name you can edit inline.
- **Password**: change it; this signs out your other sessions.
- **Two-factor** and **Passkeys**: enrol or remove second factors.
- **Active sessions and trusted devices**: every browser signed in as you. End one, or sign out all others. Trusted devices are browsers that skip the new-device check; remove any you no longer use.
- **Preferences**: your timezone, language, theme and the page the dashboard opens after sign-in. They follow your account to every browser. The avatar is your initials on a color derived from your account; a Gravatar is used only if you switch it on (off by default, it shares a hash of your email with Gravatar). No uploads.
- **Email address**: change the address you sign in with. A link goes to the new address first, you confirm it while signed in, and the old address gets a notice. It needs a password and a configured email sender; without one the card links to Email settings.
- **Linked sign-in providers**: link an enabled identity provider to your account, or unlink one. Linking needs your live session, so a provider can never be attached by email alone. You cannot unlink your last way in: the dialog offers Set a password and Add a passkey instead.
- **My API tokens**: how many active tokens you hold, with a link to manage them.
- **My access**: your access level, teams, attached IAM policies and, for each area you cannot manage, the permission that is missing.
- **My activity**: what your own account did, newest first: sign-ins, token use, session changes, passkey and two-factor changes, preferences and exports. Filter by category. Only your own rows are ever listed.
- **Export my data**: a zip of your profile, preferences, sessions, tokens (never their secrets), linked providers, team roles and activity. Built in the background, kept for 15 minutes.
- **Danger zone**: delete your own account after re-confirming your password (or passkey) and typing your email. It lists what blocks it, each with a button to fix it: being the last administrator, or the only owner of a team that still has members or projects. Your tokens are revoked and you are signed out everywhere.

## From the API

| Route | What it does |
| --- | --- |
| `PATCH /api/v1/auth/profile` | Change your own display name (1 to 80 characters). Session only. |
| `GET /api/v1/auth/my-access` | Your tier, abilities, teams, linked providers, attached policies and per-area capabilities. |
| `GET /api/v1/security/sessions` | Your live sessions, trusted devices and tokens. Never returns token values. |
| `DELETE /api/v1/security/sessions/{id}` | End one of your sessions. |
| `POST /api/v1/auth/sessions/revoke-others` | End every session except this one. |
| `GET /api/v1/auth/my-activity` | Your own activity feed. `?kind=` (`sign_in`, `security`, `sessions`, `tokens`, `token_use`, `account`, `failed`), `?limit=` (1 to 100) and `?cursor=` from `next_cursor`. |
| `GET /api/v1/auth/my-credentials` | Your live sessions and active tokens, read only, never a token secret. |
| `GET` / `PUT /api/v1/auth/preferences` | Your timezone, language, theme, landing page and Gravatar choice. |
| `POST /api/v1/auth/export`, `GET /api/v1/auth/export/{id}`, `GET /api/v1/auth/export/{id}/download` | Request, poll and download your data export. |
| `GET /api/v1/auth/delete-account/check`, `DELETE /api/v1/auth/account` | See what blocks deleting your account, then delete it (typed email plus a password or step-up proof). |
| `DELETE /api/v1/auth/link/{provider}` | Unlink a sign-in provider. Refused with `last_sign_in_method` when it is your only way in. |
| `GET` / `POST /api/v1/auth/email-change`, `POST /api/v1/auth/email-change/confirm` | Whether email can be sent, request an address change, confirm it with the mailed token. Session only. |

Every route here is scoped to the caller, and the account always comes from your credential, never from the request. Another user's sessions read as not found unless you are an admin using `?user_id=`. A token can use the read routes when it belongs to a user and holds `read:sensitive` (activity, sessions, export) and the write routes with `write:sensitive`.

## From the CLI

```bash
levelrail-cli auth whoami --access
```

Prints your access level, abilities, teams and attached policies, then one line per area: `can` or `cannot`, with the missing ability for the latter. Add `--json` for the raw response.

```bash
levelrail-cli auth activity [--kind security] [--limit 25]
levelrail-cli auth preferences get
levelrail-cli auth preferences set --timezone Europe/Berlin --theme dark
levelrail-cli auth export --out my-data.zip
levelrail-cli auth unlink github
levelrail-cli auth delete-account --confirm you@example.com
```

`delete-account` logs in with `--username` and `--password` (prompted if omitted), and takes `--code` when two-factor is on. All of them accept `--json`.

## From MCP

Read-only tools: `whoami`, `get_my_access`, `list_my_sessions`, `list_my_tokens` (never a secret) and `list_my_activity`. None of them changes the account.
