---
title: "Sleep when idle"
description: Stop an app that has had no requests for a while so it uses no memory or CPU, and wake it on the next request.
---

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

For API callers that cannot retry an error page, turn on **hold requests while
waking** (`apps sleep enable --hold`): the request waits for the start and is
replayed. See [Functions](/functions).

## Running many apps on a small server

Memory is the limit on a small VPS, and most side projects are idle most of
the time. With sleep on, an app that nobody visits holds no memory, so the
box only needs room for the apps in use at once, plus the platform itself.

- Give each app a **memory limit** (App, Settings). The dashboard counts a
  sleeping app's limit as memory it is not using.
- Start with 30 minutes. Shorter saves more memory but makes more visitors
  wait for a cold start.
- Turn on **hold requests while waking** for apps that other services call.
- The dashboard's Usage details shows how many apps are asleep and how much
  memory that frees. The apps list marks each sleeping app with an Asleep
  badge.

![The Sleeping apps card on the dashboard, with the apps that are asleep and Wake now](assets/screenshots/dashboard-sleeping-apps.png)

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
levelrail-cli apps sleep list
levelrail-cli apps sleep wake web
levelrail-cli apps sleep disable web
```

## Dashboard

![The Sleep when idle card on an app that is asleep, with Wake now](assets/screenshots/app-sleep-card.png)

App, Deploys: the Sleep when idle card has the switch, the idle minutes, and a
Wake now button while the app is asleep. Sleeping apps carry an Asleep badge in
the apps list, and the dashboard's Usage details has a Sleeping apps card.

## API

`GET` and `PUT /api/v1/apps/{name}/sleep` and `POST /api/v1/apps/{name}/sleep/wake`.
`PUT` and the wake route need the `deploy` ability, `GET` needs `read`. See the
[API reference](/api-reference).

## Troubleshooting

### Turning sleep on returns an error

**Likely cause:** The app has a pinned host port. Traffic on that port bypasses the ingress, so nothing could wake the app and its activity could not be seen. The API answers 400.

**How to confirm:** `levelrail-cli apps network <name>` shows the host port. The Sleep when idle card in the app's Deploys tab shows a note and its switch is off.

**Fix:** Unpin the host port and route the app through a domain instead, or leave sleep off for this app.

### Sleep now says to turn sleep on first, or that the app is already stopped

**Likely cause:** `apps sleep now` and the Sleep now button need sleep to be enabled for the app, and an app you stopped by hand cannot be put to sleep. Both answer 409.

**How to confirm:** `levelrail-cli apps sleep status <name>` shows whether sleep is on and whether the app is asleep.

**Fix:** Run `levelrail-cli apps sleep enable <name> --after 30m` first, or start the app and try again.

### A request to a sleeping app returns 503

**Likely cause:** This is the waking-up page, by design. The first request starts the app and the page retries on its own. Scripts and API clients do not retry an HTML page.

**How to confirm:** `levelrail-cli apps sleep status <name>` reports the app as asleep before the request, and awake a few seconds to a minute later.

**Fix:** For callers that cannot retry, turn on hold mode: `levelrail-cli apps sleep enable <name> --after 30m --hold`. The request then waits for the app and gets one `307` redirect back to the same URL once it is ready, so the client must follow redirects (`curl -L`, and most HTTP libraries do for GET).

### The app sleeps while people are using it

**Likely cause:** Only requests through the built-in ingress count as activity. TCP streams, a pinned host port, traffic from other apps and background jobs do not.

**How to confirm:** Open the app's Metrics tab and look at the request rate. A flat zero while the app is busy means the traffic is not coming through the ingress.

**Fix:** Turn sleep off for that app (`levelrail-cli apps sleep disable <name>`), or send its traffic through a domain.

### The app wakes but requests still fail

**Likely cause:** The app did not become ready in time. Hold mode waits up to 30 seconds, after which the visitor gets the waking-up page again while the start continues.

**How to confirm:** `levelrail-cli apps logs <name>` shows the app's startup output. The Deploys tab shows its readiness condition.

**Fix:** Fix whatever slows or breaks startup (a migration on boot, a missing variable). Use a longer idle time for apps with slow starts so they sleep less often.

### The app stays asleep after you started it by hand

**Likely cause:** It should not: starting the app clears the sleeping state on the next check, which runs every minute.

**How to confirm:** Run `levelrail-cli apps sleep status <name>` again after a minute.

**Fix:** If it still reports asleep, run `levelrail-cli apps sleep wake <name>`.

### Still stuck

Collect the output of `levelrail-cli apps sleep status <name>`, `levelrail-cli apps logs <name>` and `levelrail-cli doctor`, then see [Troubleshooting](troubleshooting.md).
