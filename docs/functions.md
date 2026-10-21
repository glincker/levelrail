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

The dashboard has the same flow under Functions: the Deploy function button, and
copyable starter handlers for each runtime.

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
