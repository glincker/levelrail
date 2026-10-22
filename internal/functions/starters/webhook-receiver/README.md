# Webhook receiver

Verifies an HMAC signature, then appends the event to the platform key-value store. `GET /events` reads the latest events back.

## Environment

| Variable | Required | Meaning |
| --- | --- | --- |
| `KV_URL`, `KV_TOKEN` | yes | Injected when you enable the key-value store for the function. |
| `WEBHOOK_SECRET` | yes | Shared secret the sender signs with. Every call is refused while it is unset. |
| `ADMIN_TOKEN` | no | Bearer token for `GET /events`. The route is closed while it is unset. |
| `EVENT_TTL_SECONDS` | no | How long events are kept. Default 604800 (7 days). |
| `SIGNATURE_WINDOW_SECONDS` | no | Replay window for timestamped signatures. Default 300. |

## Signature

Send `X-Signature: sha256=<hex>` where the hex is `HMAC-SHA256(secret, body)`. To get replay protection, also send `X-Signature-Timestamp: <unix seconds>` and sign `timestamp + "." + body` (the same scheme as the platform's signed webhook trigger).

```
BODY='{"type":"order.paid"}'
SIG=$(printf '%s' "$BODY" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | sed 's/^.* //')
curl -X POST https://<function>/ -H "X-Signature: sha256=$SIG" -d "$BODY"
```

## Test

```
node --test
```
