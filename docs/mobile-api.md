---
title: "Mobile API"
description: "Pair a phone with the control plane: one-time pairing codes, scoped device tokens, revocation, and the single round trip overview endpoint."
"og:url": "https://docs.levelrail.com/mobile-api"
---

The mobile API is the server side of the phone app: pairing, per-device tokens, and an overview endpoint sized for a phone. It is a beta feature and off by default.

## Turn it on

Open **Settings, Beta features** and switch on **Mobile app access**, or set `APP_EXPERIMENTAL=mobile-api`. Set `APP_MOBILE_API=off` to disable it entirely; that wins over the Settings switch. While it is off, every `/api/v1/mobile/` route answers `404` with code `experimental_feature_disabled`.

## Pair a phone

1. In **Settings, Mobile devices**, click **Pair a device** and confirm your password (or passkey).
2. The page shows a one-time code, a pairing link and the server address. The code works once and expires after 10 minutes.
3. Enter the code in the app, or open the link on the phone.

From the CLI, with a root token:

```sh
levelrail-cli mobile pair
levelrail-cli mobile devices list
levelrail-cli mobile devices revoke <id>
```

`mobile pair` asks for your password (and authenticator code when enrolled), the same as `apps env get --reveal`.

### What a paired phone can do

Each phone gets its own API token named `Mobile: <device name>`. It shows up in **Settings, API tokens** and can be revoked there or from the Mobile devices page.

| Ability | Granted |
| --- | --- |
| `read` | yes |
| `deploy` (restart, deploy, rollback, start, stop) | yes |
| `root`, `write`, `write:sensitive`, `read:sensitive` | never |
| `secrets:reveal`, `signin:approve` | never |

The token expires after 30 days by default (`APP_MOBILE_TOKEN_TTL`, for example `7d` or `720h`, at most 365 days). A security policy with a shorter maximum token lifetime wins. The abilities are also capped at what the pairing user holds.

## Endpoints

| Route | Auth | Purpose |
| --- | --- | --- |
| `POST /api/v1/mobile/pairing-codes` | root plus step-up | Create a one-time pairing code |
| `POST /api/v1/mobile/pair` | none, the code is the credential | Exchange `code`, `device_name`, `platform` (`ios`, `android`, `other`) for a device token |
| `GET /api/v1/mobile/devices` | read | List paired phones (root sees all, others their own) |
| `DELETE /api/v1/mobile/devices/{id}` | read, owner or root | Revoke a phone and its token |
| `GET /api/v1/mobile/overview` | read | Server, attention items, apps, nodes, 24h deploys, open incidents |

`POST /api/v1/mobile/pair` is rate limited per IP (`APP_MOBILE_PAIR_PER_MINUTE`, default 10) and per code. Five wrong guesses burn a code. Every failure returns the same `400` with code `pairing_invalid`, so a caller cannot tell an unknown code from an expired or used one.

### Overview and caching

`GET /api/v1/mobile/overview` returns everything the home screen needs in one request. Lists are capped (50 apps, top 5 attention items); `apps_total` and `attention.count` carry the full counts. The response carries an `ETag`. Send it back as `If-None-Match` and an unchanged overview costs a `304` with no body.

## Typed client

Every route here is in the generated OpenAPI document at `docs/public/openapi.json` (see [API reference](/api-reference)), so the app can generate a typed TypeScript client from it instead of hand writing request shapes.

## Audit

Pairing code created, device paired, failed pairing and device revoked each write an audit entry (`mobile.pairing_created`, `mobile.paired`, `mobile.pair_failed`, `mobile.revoked`).
