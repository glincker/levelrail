---
description: Point any Sentry SDK at your control plane, group errors into issues, and see which deploy produced them.
---

# Error tracking

Apps report errors to the control plane with the Sentry SDK they already use. Events are grouped into issues, each tied to the deploy that was current when it arrived, and shown on the app's **Observe, Errors** page. Nothing leaves the node, and nothing is stored until you turn error tracking on for an app.

## Turn it on

In the dashboard, open the app, then **Observe, Errors**, and choose **Turn on error tracking**. Or:

```bash
levelrail-cli apps error-tracking enable web
```

You get a DSN. It is shown once, and enabling again rotates it (the old DSN gets 401s):

```
SENTRY_DSN=https://<key>@cp.example.com/api/v1/sentry/1
```

Set it on the app and redeploy. The project number in the DSN is always `1`; the key alone identifies the app.

## SDK setup

JavaScript:

```js
import * as Sentry from '@sentry/node'
Sentry.init({ dsn: process.env.SENTRY_DSN, release: process.env.GIT_SHA })
```

Python:

```python
import os, sentry_sdk
sentry_sdk.init(dsn=os.environ["SENTRY_DSN"], release=os.environ.get("GIT_SHA"))
```

Go:

```go
sentry.Init(sentry.ClientOptions{Dsn: os.Getenv("SENTRY_DSN"), Release: os.Getenv("GIT_SHA")})
defer sentry.Flush(2 * time.Second)
```

Any other Sentry SDK works the same way. Browser SDKs are supported (the receiver answers CORS preflights).

## How errors are grouped

An event joins an issue by fingerprint, in this order:

1. The SDK's own `fingerprint`, when it is more than `["{{ default }}"]`.
2. The exception type plus the innermost in-app stack frame (module or file, and function; line numbers are ignored so a code edit does not split an issue).
3. The message with digits, hex ids and UUIDs collapsed, for events with no exception.

An issue is `open`, `resolved` or `ignored`. A resolved issue that receives a new event reopens and is flagged as a regression. Each issue records the SDK `release` and the deploy that was newest when its last event arrived.

## CLI

```bash
levelrail-cli apps errors list web --status open --since 24h
levelrail-cli apps errors get web <fingerprint>
levelrail-cli apps errors resolve web <fingerprint>
levelrail-cli apps errors ignore web <fingerprint>
```

## Retention and limits

- Events are scrubbed of values under keys such as `authorization`, `cookie`, `password`, `secret` and `token` before they are stored.
- Each issue keeps its newest 20 events (`APP_ERROR_EVENTS_PER_ISSUE`), each at most 64 KiB.
- Issues and events age out with the logs retention window (`APP_LOGS_RETENTION`).
- A request body is capped at 1 MiB after decompression; gzip is accepted.
- Each app may send 600 requests a minute (`APP_ERROR_INGEST_PER_MIN`). Past that the receiver answers 429 with `Retry-After`, which SDKs honor.
- Sessions, transactions, attachments and other envelope item types are accepted and ignored.
