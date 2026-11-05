---
title: "Image auto-update"
description: Redeploy an image-based app automatically when its tag points at a new digest, with the same freeze and protection gates as a manual deploy.
"og:url": "https://docs.levelrail.com/image-auto-update"
---

An app can redeploy itself when its image tag moves to a new digest (the
hash of the image's contents). It suits apps that track a floating tag such
as `nginx:1` or `myorg/api:stable`. It is off by default, per app.

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
treat it like a password. It must be reachable by your registry. Rotating
also mints a GitHub signing secret (shown once) when the control plane has a
secrets store, and records who rotated, which matters for approvals below.
A rotation stages the new secret and records the rotator before the new URL is
saved, so a failure before the save leaves the working hook as it was. After
the save you always get the new URL and secret, and the staged secret is
accepted until the next signed delivery or rotation finishes promoting it.

A webhook for an app that has not opted in, or with a wrong token, answers
`404`. A plain POST with no recognized push body just runs the interval
check now (same skip rules, repeat deliveries within the debounce are
acknowledged and skipped) and is not gated by the signing secret.

### Docker Hub and GitHub (ghcr.io) pushes

A recognized push body is handled as a push event and deploys a digest, not
the tag. For both providers the control plane resolves the pushed tag from
the registry with the app's registry credentials and deploys what the
registry serves. A GHCR payload digest is only a claim: if it differs from
the registry's current digest, nothing deploys and the audit row records both.

- **Docker Hub:** add the URL under the repository's Webhooks. Only
  `docker.io` repositories match. Docker Hub sends no digest and no
  signature, so the URL is the only secret.
- **GitHub Packages (ghcr.io):** add the URL under Settings, Webhooks, with
  content type `application/json`, event Packages, and the secret printed
  with the URL. A delivery without a valid `X-Hub-Signature-256` answers
  `404` when a secret exists.

The pushed repository must be the app's own image repository (otherwise
`404`) and the pushed tag must be the tag the app follows (otherwise the
delivery is acknowledged and ignored). Built-from-source and stopped apps
ignore pushes.

What a push does not skip, unlike the interval check:

- **Protected environments** file a deploy approval for the digest instead of
  being skipped. The person who last rotated the webhook cannot approve it.
  A webhook rotated before this feature has no recorded person, so it files
  no approval until you rotate it once (the card shows "rotate the registry
  webhook once"). A pending approval for the same digest suppresses a second
  one, and a newer push supersedes older pending approvals the hook filed.
- **The update window** still applies outside protected environments: a push
  outside it is recorded as waiting.
- **Deploy freezes and policy** apply as for a manual deploy.

One push per app is processed at a time. A delivery that arrives during one
is queued and runs right after it, so the latest push is not lost; the queue
keeps only the newest delivery and lives in memory, so a restart drops it, and
a deploy that outlives `deploys.registry_push_timeout` keeps the slot busy so
later pushes only queue. A queued run re-checks the app first: if the tag it
follows, its running state or its update window changed meanwhile, the push is
not deployed. Unsigned deliveries run at least `deploys.registry_push_debounce`
apart (each costs a registry lookup) instead of being dropped, and a signed
repeat of the digest just handled is skipped.

The outcome shows as the card's last result and as a `registry_push.deploy`
audit row with provider, repository, tag and digest. Runtime settings:
`deploys.registry_push_max_body_bytes`, `deploys.registry_push_timeout` and
`deploys.registry_push_debounce`. See ADR 073.

A tag pinned by digest (`image@sha256:...`) is followed by tag: both the
interval check and a push resolve the tag and deploy its newest digest.
