---
title: "Build hooks"
description: Secret URLs that build and deploy an app when POSTed to, for headless CMS publishes and cron triggers.
"og:url": "https://docs.levelrail.com/build-hooks"
---

A build hook is a secret URL for one app. A POST to it builds the app's connected git source and deploys it. Use it to rebuild a static site when a headless CMS publishes, or from a cron job.

An app can have several named hooks, each optionally pinned to a branch.

## Create a hook

Dashboard: open the app, then Deploys, then Build hooks, enter a name (and optionally a branch) and choose Create hook. Copy the URL right away: it is shown once.

CLI:

```sh
levelrail-cli apps build-hooks create my-site --name "cms publish" --branch main
levelrail-cli apps build-hooks list my-site
levelrail-cli apps build-hooks rotate my-site bh_AbC123
levelrail-cli apps build-hooks revoke my-site bh_AbC123
```

API: `GET|POST /api/v1/apps/{name}/build-hooks`, `POST .../{id}/rotate`, `DELETE .../{id}`. Creating, rotating and revoking need the write-sensitive ability on the app.

## Trigger a build

```sh
curl -X POST https://levelrail.example.com/api/v1/hooks/deploy/bh_AbC123/SECRET
```

No auth header is needed: the secret in the path is the credential. The call answers `202` at once and the build runs in the background.

Optional context, as query parameters or a JSON body:

```sh
curl -X POST "$HOOK_URL" -H 'Content-Type: application/json' \
  -d '{"title": "Post published", "body": "hello-world"}'
```

The build receives `TRIGGER_TITLE`, `TRIGGER_BODY`, `TRIGGER_BRANCH` and `TRIGGER_HOOK` as build args. Declare the ones you want in the Dockerfile with `ARG TRIGGER_TITLE`. A body that is not JSON is ignored, so any CMS webhook works.

## Behavior

- Branch: the hook's branch if set, otherwise the app's git source branch.
- The build uses the latest commit on that branch and shows in deploy history as a webhook deploy.
- A deploy freeze skips the build and records the reason as the hook's last result.
- An app with no git source accepts the call but records `failed: no git source connected`.
- Each hook shows its last triggered time, run count and last result.

## Security

- The secret is random, stored only as a hash, compared in constant time, and shown once. Rotate to replace it; the old URL stops working immediately. Revoke to delete the hook.
- Unknown hook and wrong secret answer the same `404`.
- Each trigger is written to the audit log with the secret redacted from the path.
- Rate limits answer `429` with `Retry-After`. Environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BUILD_HOOK_TRIGGERS_PER_MINUTE` | 6 | Triggers per hook per minute |
| `APP_BUILD_HOOK_AUTH_FAILURES_PER_MINUTE` | 30 | Requests per client address per minute, bounds secret guessing |
| `APP_BUILD_HOOKS_MAX_PER_APP` | 20 | Hooks per app |
| `APP_BUILD_HOOK_FIELD_MAX_BYTES` | 1024 | Longest title or body passed to the build |
| `APP_BUILD_HOOK_BODY_MAX_BYTES` | 65536 | Most of the request body that is read |
| `APP_BUILD_HOOK_TIMEOUT_SECONDS` | 1800 | Longest a triggered build may run |

## Limits

Hooks build from the app's git source. Image-only apps use [image auto-update](/image-auto-update) instead.
