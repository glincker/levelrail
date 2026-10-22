---
title: "Functions"
description: Deploy a ten line handler in Node, Python or Go as a function that sleeps when idle and answers the first request after a cold start, like a serverless function on your own server.
"og:url": "https://docs.levelrail.com/functions"
---

A function is an app that costs nothing while idle. It sleeps after a
quiet period and starts on the next request. Unlike a plain
[sleeping app](/sleep-when-idle), a request that arrives while it is asleep
waits through the cold start and is then answered, so API callers see one slower
response instead of an error page. It runs on your own server next to your other
apps, with the same logs, metrics, secrets, domains and rollbacks.

## From a handler to a live URL

Start from a starter handler (Node, Python or Go, about ten lines each):

```
levelrail-cli functions init node ./hello
cd hello && git init && git add . && git commit -m "handler" && git push
levelrail-cli functions deploy hello --repo https://example.com/you/hello.git
```

`functions deploy --repo` creates the app, builds the handler with Railpack (no
Dockerfile needed), turns on sleeping with function mode, waits for the build to
go live and prints the URL. `--dir` points at a handler inside a monorepo, `--ref`
picks the branch, `--wait 0` returns without waiting. The handler listens on the
`PORT` variable, which is set to `--port` (default 8080). The first request after
it sleeps wakes it.

The dashboard has the same flow under Functions, as a three step guide behind the
Deploy function button:

1. **What to run.** Pick a starter (Go, Node.js, Python), an idea that prefills
   the runtime, sleep period and a suggested name (webhook receiver, scheduled
   job runner, form to email, URL shortener, chat bot), your own repository, or a
   container image. Ideas show copyable starter code to push first.
2. **Where the code is.** Choose from the repositories of your connected git
   providers (search, most recently updated first) or paste a URL, then pick a
   branch. The build is checked and the detected framework is shown. For an image,
   enter the reference with a tag.
3. **Details.** The name is suggested from the repository or image and checked for
   availability. "Sleep after" is a select (5 minutes, 10 minutes, 1 hour, 1 day,
   custom). Serving at a path uses a dropdown of domains already attached to an
   app, with a live preview of the final URL. The port sits under Advanced
   settings. A summary line states how it builds, when it sleeps and where it is
   reachable before you press Deploy.

After deploying, the dialog follows the build and shows the function URL with
Copy and Open function buttons. The starter handlers also stay on the page for
each runtime.

## Image functions, calling and statistics

```
levelrail-cli functions deploy hello --image traefik/whoami:v1.10.1 --port 80 --idle 10m
levelrail-cli functions invoke hello --path /api
levelrail-cli functions list --window 24h
levelrail-cli functions delete hello
```

`functions list` and the Functions page show each function's state, number of
calls, p95 duration and server error rate over a 1 hour, 24 hour or 7 day window.
The numbers come from the ingress request metrics, so they cover calls through
the built-in ingress. Logs are the app's own logs (`apps logs <name>`). `--idle`
is 5 minutes to 7 days.

To make an existing app behave this way:

```
levelrail-cli apps sleep enable web --after 10m --hold
```

The dashboard has the same switch: App, Deploys, Sleep when idle, "Hold
requests while waking".

## Serve a function at a path

A function can answer at `<domain>/<path>` as well as on its own host. Pick a
domain that is already attached to an app, then give the path:

```bash
levelrail-cli functions deploy hello --repo https://github.com/you/hello --domain example.com --path /hello
```

The Deploy function dialog has the same two fields. The route is a path
forwarder on the domain (see domain traffic controls), so `/hello` and
`/hello/...` go to the function, the prefix is stripped before the handler sees
the request, and every other path still goes to the domain's own app. A longer
prefix wins over a shorter one. A sleeping function is woken by a request to
its path. `functions delete` removes its routes.

## Runtime controls and versions

Open a function from the Functions page to get its own view with Overview,
Settings, Environment and secrets, and Versions tabs. Settings reuses the app
cards for the idle period, memory and CPU limits and the port, and adds two
limits that apply while a request waits for a sleeping function:

- Request timeout: the longest a request is held during a cold start. It
  replaces the adaptive wait (the 30 second default in "What a caller sees").
- Max concurrency: requests held at once. The next one gets `429` with
  `Retry-After`.

`0` means the server default. Defaults and ceilings come from
`APP_FUNCTION_REQUEST_TIMEOUT`, `APP_FUNCTION_REQUEST_TIMEOUT_MAX`,
`APP_FUNCTION_MAX_CONCURRENCY` and `APP_FUNCTION_MAX_CONCURRENCY_CEILING`.
Both limits apply to requests held for a wake, not to traffic proxied to an
awake function. Environment variables and secrets are the app's own editor.

Versions lists the deploy history. Roll back uses the app rollback. Pin this
version rolls back and locks publishing, so new builds stay held until you
unlock them.

```
levelrail-cli functions settings hello --timeout 45s --concurrency 20 --idle 10m
levelrail-cli functions versions hello
levelrail-cli functions rollback hello --to <deploy id> --pin
```

The API is `GET|PUT /api/v1/functions/{name}/settings`,
`GET /api/v1/functions/{name}/versions` and
`POST /api/v1/functions/{name}/rollback?to=<id>&pin=true`. Without `--to`,
rollback goes to the newest earlier successful version.

## Triggers

A function wakes on a request. Three more things can call it for you, and all of
them wake it first:

| Trigger | How |
| --- | --- |
| Schedule | An http job on the function's app, see [scheduled functions](/scheduled-functions). It is listed under the function's triggers. |
| Signed webhook | `POST /api/v1/webhooks/functions/<name>`, signed with a per-function secret. |
| Async call | `POST /api/v1/functions/<name>/async` returns `202` at once; the call is delivered from a durable queue. |

```
levelrail-cli functions triggers list hello
levelrail-cli functions triggers webhook hello --path /hooks/stripe
levelrail-cli functions triggers enqueue hello --path /jobs/resize --data '{"id":7}'
levelrail-cli functions triggers dead-letters
levelrail-cli functions triggers retry inv_0123abcd
```

The Functions page shows the same under Triggers: enable, rotate or turn off the
webhook, see schedules and queue depth, and retry or discard dead letters.

### Signed webhook

Enabling the webhook prints a secret once (rotate to get a new one). It needs the
control plane master key. A caller sends the body with two headers:

- `X-Signature-Timestamp`: unix seconds.
- `X-Signature`: `sha256=` and the hex of `HMAC-SHA256(secret, timestamp + "." + body)`.

A timestamp outside the replay window, a bad signature or a repeated signature is
refused (`401`, or `200` with `duplicate: true` for a repeat). A good call is
queued and forwarded as a `POST` to the path chosen at setup, with the original
`Content-Type` and body. The request is answered with `202` before the function
has run.

### Queue, retries and dead letters

Each queued call is delivered by a small worker pool that wakes the function,
calls it and treats a `2xx` as done. Transport errors, `408`, `429` and `5xx`
are retried with a doubling delay up to a cap; any other status goes straight to
the dead letters, as does a call that used all its attempts. Delivery is
at least once: the function gets `X-Function-Invocation` (stable id) and
`X-Function-Attempt` headers so it can deduplicate. A full queue answers `429`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_FUNCTION_WEBHOOK_REPLAY_WINDOW` | 5m | Allowed clock difference for a signed timestamp. |
| `APP_FUNCTION_WEBHOOK_RATE_LIMIT_RPM` | 600 | Webhook calls per minute per client and function. |
| `APP_FUNCTION_QUEUE_MAX_BODY_BYTES` | 1048576 | Largest queued body. |
| `APP_FUNCTION_QUEUE_MAX_PER_FUNCTION` | 1000 | Waiting plus running calls per function. |
| `APP_FUNCTION_QUEUE_MAX_ATTEMPTS` | 5 | Tries before a call is dead-lettered. |
| `APP_FUNCTION_QUEUE_BACKOFF_BASE` | 5s | Delay after the first failure, doubled each try. |
| `APP_FUNCTION_QUEUE_BACKOFF_MAX` | 15m | Cap of the retry delay. |
| `APP_FUNCTION_QUEUE_ATTEMPT_TIMEOUT` | 2m | Wake plus call time of one try. |
| `APP_FUNCTION_QUEUE_POLL_INTERVAL` | 2s | How often idle workers look for due calls. |
| `APP_FUNCTION_QUEUE_WORKERS` | 2 | Calls delivered at the same time. |
| `APP_FUNCTION_QUEUE_DONE_RETENTION` | 24h | How long finished calls are kept. |

Not yet verified on a real host. Request bodies are stored in the control plane
database until delivered, so do not queue secrets you would not keep there.

## What a caller sees

When the function is asleep, the first request is held while the container
starts and the route updates (typically about a second for a cached image, longer
for a large one). The platform then answers with a `307` redirect to the same
URL. A `307` keeps the method and body, so a POST is replayed as it was sent.
Most HTTP clients follow it: `curl -L`, browsers, `fetch`, Go's client and
Python `requests` do. A client that does not follow redirects sees the `307` and
can retry.

If the app does not become ready within 30 seconds, the caller gets the
"Waking up" page (503) and can retry.

## Recent calls, test console and replay

The Functions page lists the recent calls of a function: time, method, path, status, duration, whether it was a cold start, and request and response sizes. Each call links to the app logs filtered around the time of the call. The list comes from the edge access log, which lives in the control plane's memory and is lost on restart. Request bodies are never recorded.

| Environment variable | Meaning |
| --- | --- |
| `APP_ACCESS_LOG_SIZE` | Calls kept per route, 0 turns recording off |
| `APP_ACCESS_LOG_RETENTION` | Hide calls older than this duration, empty keeps them until the ring overwrites them |
| `FUNCTION_LOG_PADDING` | Time added before and after a call when linking to logs |

The test console sends a request through the function's real public route, so a sleeping function is woken first. It shows status, headers, body, duration and the cold start flag. The same call is available as `POST /api/v1/functions/{name}/invoke`.

Replay is opt-in per function. With "Keep the input of test calls" on, calls sent from the console or the CLI keep their method, path, body and headers (without `Authorization`, `Cookie` and similar credential headers) so the Replay button, `POST /api/v1/functions/{name}/invocations/{id}/replay` or `functions invoke <name> --replay <id>` can repeat them. Traffic from other clients is never stored. Stored inputs live in memory only.

| Environment variable | Meaning |
| --- | --- |
| `FUNCTION_INPUT_MAX_ENTRIES` | Stored inputs kept in total |
| `FUNCTION_INPUT_MAX_BODY_BYTES` | Stored body size, longer bodies are cut |
| `FUNCTION_INPUT_RETENTION` | How long a stored input stays replayable |
| `FUNCTION_TEST_MAX_BODY_BYTES`, `FUNCTION_TEST_MAX_RESPONSE_BYTES`, `FUNCTION_TEST_TIMEOUT` | Caps for a test call |

A call counts as a cold start when the wake hook held it and replayed it. From the CLI:

```bash
levelrail-cli functions invocations hello --limit 20
levelrail-cli functions invoke hello --replay 3f9a1c2b7d4e5a60
```

## Run a handler locally

```bash
levelrail-cli functions dev ./hello --port 8080 --env-file .env.local
```

Runs the handler in a throwaway container for its runtime (picked from `package.json`, `requirements.txt` or `main.py`, or `go.mod`) with the directory mounted, `PORT` set and the env file passed in. Docker must be installed. `--image` overrides the image. Stop it with Ctrl-C.

## Key-value store

A function can keep small pieces of state in a store the control plane holds for
it. Each function has its own namespace; nothing else can read it.

```
levelrail-cli functions kv enable hello          # issue the token, restart with KV_URL and KV_TOKEN
levelrail-cli functions kv put hello greeting "hi" --ttl 1h
levelrail-cli functions kv get hello greeting
levelrail-cli functions kv list hello --prefix gre
levelrail-cli functions kv delete hello greeting
```

The dashboard has the same under the function's Key-value store tab: browse,
edit, delete and rotate the token. Values are shown truncated in the list and
are never written to logs.

Enabling the store injects two variables into the function: `KV_URL` and
`KV_TOKEN`. The token is stored encrypted with the function's other secrets
(it needs the control plane master key), works for that function's namespace
only, and changes when you rotate it (`kv enable` again, or Rotate token). The
function restarts to pick up a new token, and the old one stops working at once.

From the handler, send the token as a bearer header:

| Call | Meaning |
| --- | --- |
| `GET $KV_URL/entry?key=k` | The value as the response body, `404` if missing or expired. |
| `PUT $KV_URL/entry?key=k&ttl=60` | The request body becomes the value. `ttl` is seconds, optional. |
| `DELETE $KV_URL/entry?key=k` | Remove the key. |
| `GET $KV_URL/keys?prefix=p&limit=100` | `{"keys":[{"key","size","expires_at","updated_at"}]}`. |

Over a limit the store answers `413` (value too large), `507` (key count or total
bytes reached) or `400` (bad key). Expired keys disappear and free their space.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_FUNCTION_KV_MAX_KEY_BYTES` | 512 | Longest key. |
| `APP_FUNCTION_KV_MAX_VALUE_BYTES` | 262144 | Largest value. |
| `APP_FUNCTION_KV_MAX_KEYS` | 10000 | Keys per function. |
| `APP_FUNCTION_KV_MAX_TOTAL_BYTES` | 67108864 | Keys plus values per function. |
| `APP_FUNCTION_KV_MAX_TTL_SECONDS` | 31536000 | Longest expiry; longer ones are shortened. |
| `APP_FUNCTION_KV_URL` | request host | Address of the control plane as the function's container sees it, for example `http://10.0.0.1:8080`. When unset, the host of the request that enabled the store is used, which may not be reachable from a container. |

The store has no compare-and-set. Two writers racing on one key both succeed and
the later write wins.

## Utility starters

Five ready handlers back the ideas in the deploy flow. Each is a complete Node
handler with a README, the exact environment variables it needs and a unit test
file you run with `node --test` (Node 18 or newer, no install step).

| Starter | What it does | Uses the store |
| --- | --- | --- |
| `webhook-receiver` | Verifies an HMAC signature, appends each event, `GET /events` reads them back. | Events. |
| `scheduled-job-runner` | Named jobs called by an http cron job, run history and an overlap lock. | Runs and locks. |
| `form-to-email` | Accepts a contact form, can ping a notify webhook. | Submissions. |
| `url-shortener` | `POST /api/links` creates, `GET /<slug>` redirects. | Slugs. |
| `chat-bot` | Relays a webhook to Slack, Discord or Telegram. | Delivery dedupe. |

```
levelrail-cli functions init url-shortener ./links
cd links && node --test && git init && git add . && git commit -m "url shortener"
# push to a repository, then:
levelrail-cli functions deploy links --repo <git url> \
  --env ADMIN_TOKEN=change-me
levelrail-cli functions kv enable links
```

Set secrets such as `ADMIN_TOKEN` or `WEBHOOK_SECRET` in the function's
Environment and secrets tab rather than `--env`, which stores them as plain
values. The deploy flow shows the same files, the variables each one reads and
the commands to copy.

Two limits to know. The platform has no email send API for functions, so
`form-to-email` stores submissions and can notify a webhook; add an SMTP library
if you want mail. And there is no way yet to deploy a starter without a git
repository: the deploy flow gives you the files to push rather than a one-click
deploy.

## Limits

- Functions are routed by host, one URL per function. Path prefixes on a shared
  domain are not supported yet.
- A function waits for one container start. Concurrency is limited only for
  requests held during a wake. There is no queueing across many replicas or
  autoscaling: it is one container, like any other app.
- The cold start is a container start, not a millisecond isolate start. Keep
  images small and the process quick to listen.
- Callers must follow redirects. Streaming request bodies larger than memory
  are not replayed by every client.
- It needs requests through the built-in ingress to count as activity, the same
  as [sleep when idle](/sleep-when-idle#limits).

## Verified

Run on a real control plane with Docker and embedded Caddy: with the function
asleep, a GET and a POST with a body (query string kept) both returned 200 in
about 0.7 seconds through `curl -L`, with the POST body arriving intact.

## Troubleshooting

### First request hangs for a long time, then times out

**Likely cause:** The function image is large or the container startup is slow. The 30 second wake timeout was exceeded.

**How to confirm:** Make a request to a sleeping function with a tool that shows timing, such as `curl -w "@curl-format.txt" -o /dev/null -s <function-url>`.

**Fix:** Keep function images small and startup fast. If the function genuinely needs more than 30 seconds to start, it is not a good candidate for sleeping mode. Disable sleep and keep it running, or optimize the startup.

### Caller receives 503 instead of the response

**Likely cause:** The client did not follow the 307 redirect that wakes the function. Or the function did not become ready within the hold timeout.

**How to confirm:** Check the HTTP response code and headers. If it is 503 "Waking up", the client did not follow the redirect or waking timed out.

**Fix:** Use a client that follows redirects (curl -L, fetch, requests, etc.). If the function is slow to start, increase its readiness timeout. If waking consistently times out (30s), consider keeping the function running instead.

### Function crashes or becomes unhealthy after waking

**Likely cause:** The function image has a startup or initialization issue, or the readiness probe is flaky.

**How to confirm:** Check the function's logs immediately after a failed wake: `levelrail-cli apps logs <function-name> -f`.

**Fix:** Fix the startup issue in the image or fix the readiness probe configuration. Test the image locally to ensure it starts and becomes ready quickly and reliably.

### Still stuck

For help troubleshooting functions, check the logs and metrics as described above, or refer to [Sleep when idle](sleep-when-idle.md) since functions use the same sleep and hold mechanism.
