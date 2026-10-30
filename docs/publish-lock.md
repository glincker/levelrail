---
title: "Publish lock"
description: "Lock publishing on an app so what is live stays exactly as it is. New builds still run and finish, but wait as held builds until you publish them."
"og:url": "https://docs.levelrail.com/publish-lock"
---

Locking publishing on an app keeps what is live exactly as it is. New builds still run and finish, but they wait as held builds until you publish them.

## What it covers

- Git pushes and webhooks (container apps).
- Static sites, including ones with a build command. The files are built and staged, but the site keeps serving the previous version.
- Automatic image deploys: image auto-update, pipeline deploy steps and releases after a freeze window.

## What it does not cover

- Manual deploys and rollbacks you start yourself. They are an operator decision, so they go live as usual.
- Automatic rollback after a failed release. Safety rollbacks still run.

If Levelrail cannot read the lock state, it holds the build instead of publishing it.

## Using it

- Lock: `PUT /api/v1/apps/{name}/publish-lock`, or the lock control on the app page.
- Held builds show as "held" in the app's deploy history.
- Publish one: `POST /api/v1/apps/{name}/deploys/{deployId}/publish`, or Publish on the held deploy.
- Unlock: `DELETE /api/v1/apps/{name}/publish-lock`. Builds already held stay publishable; only later builds go live on their own.

Publishing is safe to retry. If the control plane stops partway, publish the same build again.
