---
description: Send a share of an app's traffic to a new image before promoting it, with one-command promote and abort.
---

# Canary deploys

Run a new image next to the current release and send it a percentage of
requests. If it looks healthy, promote it. If not, abort and every request
goes back to the release that was already serving. Nothing changes for the
stable release until you promote.

## How it works

- Starting a canary creates a hidden clone of the app (`<app>--canary`) with
  the same environment variables and secrets, running the new image. It does
  not appear in the apps list and has no domains of its own.
- Ingress splits each request between the app and the canary by weight (1 to
  99 percent). The split is per request, not per user, so one visitor can see
  both releases.
- A canary that is not running or not ready receives no traffic, so a bad
  image cannot take the app down. Weight 0 pauses the canary without removing it.
- **Promote** deploys the canary image to the app through the normal deploy
  path (readiness gate, rollback target, freeze and protection checks), then
  removes the clone.
- **Abort**, or deleting the app, removes the clone.

## Limits

- One canary per app.
- Image apps only, with a single replica and no pinned host port. Apps built
  from source, stopped apps, and apps under a deploy freeze are refused.
- The canary has its own container and its own data connections. If the app
  writes to a database, the canary writes to it too.

## CLI

```
levelrail-cli apps canary start web --image myorg/web:next --weight 10
levelrail-cli apps canary weight web --weight 50
levelrail-cli apps canary status web
levelrail-cli apps canary promote web
levelrail-cli apps canary abort web
```

## Dashboard

App, Deploys: the Canary release card has the image and traffic share form,
and Promote and Abort buttons while a canary is running.

## API

`GET`, `POST`, `PUT` and `DELETE /api/v1/apps/{name}/canary`, and
`POST /api/v1/apps/{name}/canary/promote`. All need the `deploy` ability, and
`GET` needs `read`. See the [API reference](api-reference.md).
