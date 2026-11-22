---
title: "Token expiry and rotation"
description: "How Levelrail reminds you before an API token expires, how to rotate it without breaking the caller, and what the CLI does for a saved login."
"og:url": "https://docs.levelrail.com/token-expiry-and-rotation"
---

A token that expires without anyone noticing breaks an integration or an agent session at the worst moment. Levelrail shows when each token expires, reminds you ahead of time, and lets you replace a token while the old one still works.

## See when a token expires

- Dashboard: Settings > Tokens shows an expiry column with a badge: fine, expires soon, or expired.
- CLI: `levelrail-cli tokens list` prints an EXPIRES column with the date and the days left.
- Status: tokens that expire soon appear as one grouped row on the [attention center](/attention-center).

## Reminders

A background check runs on `credential_expiry.reminder_interval`. For every token you created, it raises one reminder per window in `credential_expiry.reminder_windows_days` (14, 7 and 1 days by default), and once more when the token has expired. Each reminder is sent as an event to your [event webhooks](/event-webhooks) with the types `tokens.expiring` and `tokens.expired`. A reminder is never repeated for the same window, and rotating the token clears it. Platform-owned tokens, such as the short-lived CI exchange tokens, rotate themselves and never produce a reminder.

The event carries the token id, name, expiry and the rotate path. It never carries a secret.

## Rotate a token

Rotation mints a replacement with the same name, abilities, team or stack scope and attached policies, and a fresh expiry of the same length. The new secret is returned once. The old token keeps working for `credential_expiry.rotation_grace` (24 hours by default) so you can switch the caller over, then it is revoked.

```bash
levelrail-cli tokens rotate ci-deploy
levelrail-cli auth rotate
```

`tokens rotate` takes an id or an unambiguous name. `auth rotate` replaces the token behind the current profile and rewrites that profile in the credentials file atomically, with mode 0600, keeping the new expiry. A token can rotate itself only when it holds the write ability; a read-only token is rotated by its owner. Platform tokens are refused. If no master key is configured the old token is revoked at once, because the grace period could not be remembered across a restart.

A rotation follows the same rules as creating a token:

- One rotation per token. A token that was already rotated and is in its grace period cannot be rotated again (409): use its replacement. The time the old token stops working never moves.
- Abilities come from the owner as they are now. If the owner no longer holds one of the token's abilities, the rotation is refused (403) with the ability named, and you create a new token instead. `signin:approve` and `secrets:reveal` carry over only when the owner rotates from a signed-in session, never when a token rotates itself.
- Lifetime follows the security policy. When `max_token_lifetime_days` is set, the replacement never exceeds it, and a token without an expiry gets the default lifetime. A token without an expiry keeps none only while its owner holds root and no maximum is set.
- If the old token is revoked while the replacement is being minted, the replacement is revoked too and the call answers 409.
- Tokens issued to an MCP client through OAuth sign-in are refused (409). The client renews them itself; reconnect the client to get a fresh one.

Every rotation attempt, including a refused one, is written to the audit log with the real actor: the signed-in user, or the token itself when it rotated itself. If the background check cannot read the pending revocations, it logs a warning and the error shows on the token expiry reminders loop on the control plane health page, since rotated tokens stay valid until it can.

The API call is `POST /api/v1/auth/tokens/{id}/rotate`. It answers 201 with the new token (including `token`, shown once), `replaces`, and `previous_valid_until`. While the grace runs, the token list shows `revokes_at` on the old token. Each successful rotation emits a `tokens.rotated` event.

## Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| `credential_expiry.reminder_windows_days` | `14:7:1` | Days before expiry that raise a reminder |
| `credential_expiry.reminder_interval` | `6h` | How often the check runs |
| `credential_expiry.rotation_grace` | `24h` | How long the old token works after a rotation |
| `tokens.expiry_warning_days` | `7` | Window for the grouped Status row |

See [Runtime settings](/runtime-settings) for ranges and environment seeds.

Git provider credentials follow the same windows: see [credential health, auto-renew and rotation](/git-integrations#credential-health-auto-renew-and-rotation).

## DNS provider credentials

A DNS provider token can be revoked or lose its zone rights long before a certificate renewal needs it. The `dns_credential_failing` [alert](/alerts) re-runs the read-only zone and record check on every connected provider (Cloudflare, Route 53, Google Cloud DNS and the token providers), at most once per rule `for_duration` (6 hours by default). It fires for a credential the provider rejects or that lacks zone read rights, and names the missing permission. A provider that is only unreachable does not fire it. Fix it by saving a working credential under Settings > DNS.
