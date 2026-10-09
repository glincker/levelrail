---
description: Stop an app that has had no requests for a while so it uses no memory or CPU, and wake it on the next request.
---

# Sleep when idle

Opt an app into stopping after it has had no requests for a set time. A
sleeping app uses no memory or CPU. The next request to its domain starts it
again. Off by default, per app.

## How it works

- A check runs every minute. An app sleeps when its last request, or the
  moment you turned sleeping on or last woke it, is older than the idle time
  you chose (5 minutes to 7 days). Request activity comes from the same
  per-app request metrics the dashboard charts use.
- Sleeping stops the app the same way **Stop** does. Its settings, domains,
  secrets and volumes stay.
- While asleep, the app's domains answer a short "Waking up" page that retries
  on its own. The first request starts the app, so the first visitor waits for
  the cold start (the container start plus its readiness time, usually a few
  seconds to a minute).
- Starting the app by hand clears the sleeping state. Turning sleep off wakes
  the app first.

## Limits

- Needs request traffic through the built-in ingress. An app that is only
  reached over a pinned host port, a TCP stream, or from other apps is not
  seen as active and would sleep while in use, so leave it off there.
- Background work and scheduled tasks do not count as activity.
- Apps with a deploy freeze or in a protected environment sleep and wake like
  any other app, since neither changes what runs.

## CLI

```
levelrail-cli apps sleep enable web --after 30m
levelrail-cli apps sleep status web
levelrail-cli apps sleep wake web
levelrail-cli apps sleep disable web
```

## Dashboard

App, Deploys: the Sleep when idle card has the switch, the idle minutes, and a
Wake now button while the app is asleep.

## API

`GET` and `PUT /api/v1/apps/{name}/sleep` and `POST /api/v1/apps/{name}/sleep/wake`.
`PUT` and the wake route need the `deploy` ability, `GET` needs `read`. See the
[API reference](api-reference.md).
