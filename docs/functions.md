---
description: Run an image as a function that sleeps when idle and answers the first request after a cold start, like a serverless function on your own server.
---

# Functions

A function is an image app that costs nothing while idle. It sleeps after a
quiet period and starts on the next request. Unlike a plain
[sleeping app](sleep-when-idle.md), a request that arrives while it is asleep
waits through the cold start and is then answered, so API callers see one slower
response instead of an error page. It runs on your own server next to your other
apps, with the same logs, metrics, secrets, domains and rollbacks.

## Deploy and call one

```
levelrail-cli functions deploy hello --image traefik/whoami:v1.10.1 --port 80 --idle 10m
levelrail-cli functions invoke hello --path /api
levelrail-cli functions list
levelrail-cli functions delete hello
```

`functions deploy` creates the app and turns on sleeping with function mode. The
container must serve HTTP on `--port`. Add a domain the usual way, or use the
automatic URL it prints. `--idle` is 5 minutes to 7 days.

To make an existing app behave this way:

```
levelrail-cli apps sleep enable web --after 10m --hold
```

The dashboard has the same switch: App, Deploys, Sleep when idle, "Hold
requests while waking".

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

- A function waits for one container start. There is no per-request concurrency
  control, queueing across many replicas, or autoscaling: it is one container,
  like any other app.
- The cold start is a container start, not a millisecond isolate start. Keep
  images small and the process quick to listen.
- Callers must follow redirects. Streaming request bodies larger than memory
  are not replayed by every client.
- It needs requests through the built-in ingress to count as activity, the same
  as [sleep when idle](sleep-when-idle.md#limits).

## Verified

Run on a real control plane with Docker and embedded Caddy: with the function
asleep, a GET and a POST with a body (query string kept) both returned 200 in
about 0.7 seconds through `curl -L`, with the POST body arriving intact.
