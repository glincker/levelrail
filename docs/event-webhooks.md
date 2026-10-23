---
title: "Event webhooks"
description: Push signed events to your own HTTPS endpoint when something changes, with retries, a dead letter state, a delivery log and one click replay.
"og:url": "https://docs.levelrail.com/event-webhooks"
---

Event webhooks tell your own systems when something changed on the control plane: an app was deployed, a user was added, a token was revoked. Each event is a signed JSON POST to an HTTPS endpoint you choose. Delivery is queued and runs in the background, so a slow or dead endpoint never slows an API request or a deploy.

For a continuous copy of the whole audit log to a SIEM, syslog server or bucket, use [Data export](/data-export). Event webhooks are for reacting to individual changes, and they keep a per-delivery log you can replay from.

<InlineToc default-open />

## What triggers an event

Every successful change made through the API becomes one event, named `<resource>.<verb>`, for example `apps.create`, `apps.delete`, `apps.deploy`, `users.update`. Failed requests and read requests produce no event. A subscription can list exact types, a family such as `apps.*`, or nothing to receive every event.

## Add a webhook

From **Settings, Event webhooks** in the dashboard, or:

```bash
levelrail-cli event-webhooks create --name siem --url https://hooks.example.com/levelrail --event 'apps.*,users.*'
```

The signing secret is printed once. It is stored encrypted and never returned by the API again. Lost it? Run `levelrail-cli event-webhooks rotate-secret <id>` to issue a new one.

The URL must be a public http(s) address. Internal addresses are refused unless `APP_EXPORT_ALLOW_PRIVATE_NETWORKS` is set. Creating webhooks needs a master key, because signing secrets are stored with envelope encryption.

## Payload

```json
{
  "id": "evt_9f2c1a...",
  "type": "apps.deploy",
  "created_at": "2026-10-14T09:30:00Z",
  "data": {
    "actor_type": "session",
    "actor_name": "admin",
    "method": "POST",
    "path": "/api/v1/apps/web/deploy",
    "status": 202,
    "audit_id": "aud_..."
  }
}
```

## Deploy notifications with release notes

A generic notification channel attached to an app (see [Notification channels](/alerting)) receives its own deploy outcome JSON, separate from the signed events above. For a successful deploy into a production environment, or a completed promotion, it carries an optional `release_notes` field: the markdown generated for the commits since the previous successful deploy that recorded a commit.

```json
{ "app_name": "web", "image": "web:8b7d0e4", "succeeded": true, "attempt_id": "dep_9f2", "release_notes": "## web: 3f2a9c1 to 8b7d0e4\n\n### Features\n\n- add promotion preview (8b7d0e4)\n" }
```

The field is left out when there are no notes, the app has no connected git source, the range cannot be listed, or generation fails or times out. Generation never fails or blocks the deploy or the notification. `RELEASE_NOTES_MAX_BYTES` (default 4000) caps the field; longer notes end with `...(truncated)`. `RELEASE_NOTES_TIMEOUT_SECONDS` (default 5) bounds generation.

## Verify the signature

Every request carries these headers, with the brand short name as the prefix (shown here as `X-Brand`):

| Header | Value |
| --- | --- |
| `X-Brand-Event` | event type |
| `X-Brand-Event-Id` | event id, stable across retries and replays |
| `X-Brand-Signature` | `t=<unix>,v1=<hex>` |

The signature is an HMAC-SHA256, keyed with the signing secret, over `"<t>.<raw body>"`. Recompute it over the raw bytes, compare in constant time, and reject timestamps older than a few minutes to stop replayed captures.

```python
import hmac, hashlib, time

def verify(secret: str, header: str, body: bytes, tolerance=300) -> bool:
    parts = dict(p.split("=", 1) for p in header.split(","))
    mac = hmac.new(secret.encode(), parts["t"].encode() + b"." + body, hashlib.sha256).hexdigest()
    fresh = abs(time.time() - int(parts["t"])) <= tolerance
    return fresh and hmac.compare_digest(mac, parts["v1"])
```

## Delivery, retries and the dead letter state

A delivery succeeds on any 2xx answer. Everything else is retried with exponential backoff, doubling from 30 seconds up to one hour, for up to 8 attempts. A 4xx answer other than 408 and 429 is treated as permanent and goes straight to `dead`, since retrying cannot fix it.

Delivery is at least once. If your endpoint accepts an event but the control plane fails to record that, the event is sent again with the same `X-Brand-Event-Id`, so deduplicate on it.

| State | Meaning |
| --- | --- |
| `pending` | waiting for the next attempt |
| `delivered` | the endpoint answered 2xx |
| `dead` | out of attempts or permanently rejected, replay it once the endpoint is fixed |

## Test, inspect and replay

- **Send test** (`levelrail-cli event-webhooks test <id>`) sends a signed `webhook.test` event immediately and shows the HTTP status or the error.
- **Delivery log** (`levelrail-cli event-webhooks deliveries <id>`) lists each event with its state, attempts, last HTTP status and last error.
- **Replay** (`levelrail-cli event-webhooks replay <delivery-id>`) puts a delivery back in the queue with a fresh attempt budget.

Pause a webhook with `disable`. Disabled webhooks do not queue new events.

## Tuning

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_EVENT_WEBHOOK_MAX_ATTEMPTS` | `8` | attempts before a delivery is dead lettered |
| `APP_EVENT_WEBHOOK_BACKOFF_BASE` | `30s` | wait before the first retry |
| `APP_EVENT_WEBHOOK_BACKOFF_MAX` | `1h` | cap on the retry wait |
| `APP_EVENT_WEBHOOK_TIMEOUT` | `10s` | per request timeout |
| `APP_EVENT_WEBHOOK_RETENTION` | `720h` | how long finished deliveries are kept |
| `APP_EVENT_WEBHOOK_QUEUE_SIZE` | `1024` | in memory buffer, events beyond it are dropped, never waited on |

## Limits

Events are generated from audited API changes. Changes made by the control plane itself, such as an automatic rollback, are not events yet.
