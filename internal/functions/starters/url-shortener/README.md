# URL shortener

Short links stored in the platform key-value store. `GET /<slug>` redirects, `POST /api/links` creates a link.

## Environment

| Variable | Required | Meaning |
| --- | --- | --- |
| `KV_URL`, `KV_TOKEN` | yes | Injected when you enable the key-value store for the function. |
| `ADMIN_TOKEN` | yes | Bearer token needed to create links. Without it nobody can create one. |
| `MAX_TTL_SECONDS` | no | Cap on link lifetime; also applied when a caller sends none. Default: links never expire. |

## Use

```
curl -X POST https://<function>/api/links \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{"url":"https://example.com/long","slug":"docs","ttl_seconds":86400}'
curl -i https://<function>/docs
```

Slugs are 3 to 32 letters, digits, `-` or `_`. Without a `slug` a random one is generated.
The key-value store has no compare-and-set, so two simultaneous creates of one slug can both succeed; the later write wins.

## Test

```
node --test
```
