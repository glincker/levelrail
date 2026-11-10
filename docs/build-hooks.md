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
levelrail-cli apps build-hooks create my-site --name ci --allow-cidrs 203.0.113.0/24,198.51.100.7
levelrail-cli apps build-hooks allow my-site bh_AbC123 --allow-cidrs 203.0.113.0/24
levelrail-cli apps build-hooks allow my-site bh_AbC123 --allow-any
levelrail-cli apps build-hooks list my-site
levelrail-cli apps build-hooks rotate my-site bh_AbC123
levelrail-cli apps build-hooks revoke my-site bh_AbC123
```

API: `GET|POST /api/v1/apps/{name}/build-hooks`, `PATCH .../{id}` (body `{"allowed_cidrs": [...]}`, or `{"allow_any": true}` to clear the list; a missing or empty list and unknown fields answer `400`), `POST .../{id}/rotate`, `DELETE .../{id}`. Creating, editing the allowlist, rotating and revoking need the write-sensitive ability on the app.

## Source allowlist

A hook can take an optional list of source ranges, for example your CI runners' egress addresses. A bare address means that one host. A range with host bits set (such as `10.1.2.3/8`) and a `/0` range are refused with `400`; use `allow_any` to allow every source. Entries are stored in canonical form and deduplicated; the most entries per hook is the `deploy.build_hook_max_cidrs` runtime setting (default 32, seeded by `APP_BUILD_HOOK_MAX_CIDRS`). An empty list allows any source.

A call with the right secret from an address outside the list answers `403` and builds nothing. Refused calls have their own per-hook budget, so a blocked caller never uses up the allowed callers' limit. The first refusal per hook per minute is written to the audit log (secret redacted) and shown as the hook's last refusal, apart from the last build result; repeats in the same minute are not recorded. Each allowlist change is audited with the list before and after.

The client address is read the same way as for every other rate limit: `X-Forwarded-For` counts only when the direct peer is a trusted proxy. Never put your proxy's own range in a hook's allowlist: when the proxy is not configured as trusted, every call appears to come from the proxy and the allowlist would let anyone through.

In the dashboard, choose Allowlist on a hook to edit its list, or fill Allowed sources when creating one.

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
- Each hook shows its last triggered time, run count and last result, plus its last call time, which also counts refused calls.

## Security

- The secret is random, stored only as a hash, compared in constant time, and shown once. Rotate to replace it; the old URL stops working immediately. Revoke to delete the hook.
- Unknown hook and wrong secret answer the same `404`.
- Each trigger, and each call refused by the allowlist or the per-hook limit, is written to the audit log with the secret redacted from the path.
- Rate limits answer `429` with `Retry-After`. The per-hook limit is the live runtime setting `deploy.build_hook_triggers_per_minute` (1 to 600): change it in the runtime settings and the next call uses it. Calls refused by the allowlist are limited by a separate bucket of the same size. The environment variable below only seeds its default. Environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BUILD_HOOK_TRIGGERS_PER_MINUTE` | 6 | Seeds the per-hook calls per minute setting |
| `APP_BUILD_HOOK_MAX_CIDRS` | 32 | Seeds the allowlist size setting |
| `APP_BUILD_HOOK_AUTH_FAILURES_PER_MINUTE` | 30 | Requests per client address per minute, bounds secret guessing |
| `APP_BUILD_HOOKS_MAX_PER_APP` | 20 | Hooks per app |
| `APP_BUILD_HOOK_FIELD_MAX_BYTES` | 1024 | Longest title or body passed to the build |
| `APP_BUILD_HOOK_BODY_MAX_BYTES` | 65536 | Most of the request body that is read |
| `APP_BUILD_HOOK_TIMEOUT_SECONDS` | 1800 | Longest a triggered build may run |

## Limits

Hooks build from the app's git source. Image-only apps use [image auto-update](/image-auto-update) instead.
