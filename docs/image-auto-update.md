---
description: Redeploy an image-based app automatically when its tag points at a new digest, with the same freeze and protection gates as a manual deploy.
---

# Image auto-update

Opt an app into redeploying when its image tag moves to a new digest, for
apps that track a floating tag such as `nginx:1` or `myorg/api:stable`.
Off by default, per app.

## How it works

Every enabled app is checked on the control plane's interval
(`APP_IMAGE_UPDATE_INTERVAL`, default `1h`, any Go duration). A check asks
the image's registry for the tag's current digest. If it differs from the
digest the app runs, the app redeploys through the normal image deploy path,
so it shows up in deploy history and rolls out with its configured strategy.

A check is skipped, with the reason recorded, when the app:

- was built from source (redeploy it from git instead)
- is stopped
- is inside a deploy freeze window
- is in a protected environment (deploy it manually so it gets approved)
- already has a deploy running

## Turn it on

Dashboard: open the app, then **Deploys**, and switch on **Redeploy
automatically when the tag moves**. **Check now** runs one check on demand.

CLI:

```sh
levelrail-cli apps auto-update enable web
levelrail-cli apps auto-update check web
levelrail-cli apps auto-update status web
levelrail-cli apps auto-update disable web
```

API: `GET` and `PUT /api/v1/apps/{name}/auto-update`, and `POST
/api/v1/apps/{name}/auto-update/check`. Reading needs `read`, changing or
checking needs `deploy`.

## Update on push

Instead of waiting for the interval, point your registry's push webhook at a
URL the control plane mints for the app. Dashboard: **Create webhook URL** on
the same card. CLI: `levelrail-cli apps auto-update webhook web`. The URL is
shown once and a new one replaces the old; the URL itself is the secret, so
treat it like a password. It must be reachable by your registry (Docker Hub,
Harbor and most others just POST to it).

A webhook for an app that has not opted in, or with a wrong token, answers
`404`. Repeat deliveries within 30 seconds of a check are acknowledged and
skipped. The check runs in the background and follows the same skip rules.

A tag pinned by digest (`image@sha256:...`) never moves, so there is nothing
to update.
