---
title: "Git sync for stacks"
tag: "Experimental"
description: Keep a stack in step with a git branch. Set the sync, rotate a webhook secret, add the GitHub or GitLab webhook, and choose what a push may change.
---

A stack can read its documents from a git branch. Levelrail fetches the branch, plans it against what runs now, and applies only when you turn that on. A push webhook makes this happen within seconds of a commit instead of on the next poll.

## 1. Create the stack and set the sync

Create the stack as usual, then point it at a repository:

```bash
curl -X PUT https://cp.example.com/api/v1/stacks/infra/sync \
  -H "Authorization: Bearer $LEVELRAIL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/acme/infra.git",
    "branch": "main",
    "path": "stacks/infra",
    "credential_ref": "app:web",
    "interval_seconds": 300,
    "webhook_only": false,
    "enabled": true,
    "apply": false,
    "prune": false,
    "deploy": false
  }'
```

| Field | Meaning |
| --- | --- |
| `branch` | The only branch that triggers a sync. Pushes to any other branch are ignored. |
| `path` | Folder inside the repository that holds the stack documents. It must stay inside the checkout. |
| `credential_ref` | A connected app (`app:<name>`) whose git credential reads a private repository. Never a token value. |
| `webhook_only` | `true` stops the interval poll for this stack: only the webhook and a manual refresh trigger it. |
| `apply` | Off: a sync only plans and reports pending changes. On: a sync hands a pending plan to the normal apply path, so `require_approval` still asks a human. |
| `prune` | Needs `apply`. Lets the apply remove resources that left the documents. |
| `deploy` | Needs `apply`. Lets the apply roll out apps whose spec changed. A webhook never deploys on its own: this flag decides. |

## 2. Rotate the webhook secret

```bash
curl -X POST https://cp.example.com/api/v1/stacks/infra/sync/webhook-secret/rotate \
  -H "Authorization: Bearer $LEVELRAIL_TOKEN"
```

The answer holds `secret`, `sync_url` and `refresh_url`. The secret is shown this once, stored sealed, and no GET returns it. Rotating again replaces it, and deliveries signed with the old one stop working. Root or a stack admin can rotate, and each rotation is in the audit log.

## 3. Add the webhook

Two URLs exist per stack:

| URL | What a push does |
| --- | --- |
| `https://cp.example.com/api/v1/hooks/stacks/infra/sync` | Plans, and applies when the stack sets `apply`. |
| `https://cp.example.com/api/v1/hooks/stacks/infra/refresh` | Plans only, whatever `apply` says. |

**GitHub**: repository Settings, Webhooks, Add webhook. Payload URL is one of the URLs above, content type `application/json`, secret is the rotated secret, events: just the push event. GitHub signs each delivery with `X-Hub-Signature-256`; a delivery signed only with the older SHA-1 header is refused.

**GitLab**: project Settings, Webhooks. URL is one of the URLs above, secret token is the rotated secret, trigger: push events. GitLab sends the token in `X-Gitlab-Token`.

## What the webhook answers

| Answer | When |
| --- | --- |
| `202` with `status: queued` | A push to the configured branch started a sync. |
| `202` with `status: coalesced` | A sync is already running; this push folded into one pending run. |
| `202` with `ignored: branch`, `tag`, `deleted`, `ping` or `event` | The delivery was signed but is not a push to the configured branch. |
| `404` | Unknown stack, sync turned off, no secret yet, or a wrong signature. All look the same on purpose. |
| `413` | The body is larger than `stacks.sync_webhook_max_body_kb` (64 KB by default). Raise it in runtime settings for very large pushes. |
| `415` | The content type is not `application/json`. |
| `429` | The per address or per stack webhook rate limit was hit. |

The webhook answers at once and never waits for the fetch. Only the ref and the head commit are read from the payload. Repeated deliveries are harmless: a stack never runs two syncs at once, and a burst becomes at most one running and one pending sync. Each trigger emits a `stacks.sync_triggered` event with the verb, branch, short commit and provider, never the payload.

## Safety checks on every sync

- A document that holds a plaintext secret is refused with `sync_secret_in_repo`. Reference secrets instead of writing them into git.
- A fetch is refused with `sync_disk_low` while free disk is below `stacks.sync_min_free_mb`.
- Repository size, file count and file size are capped by the `stacks.sync_*` runtime settings.

See the [error codes](/iac/errors) for every code a sync can report, and `GET /api/v1/stacks/{name}/sync` for the latest commit, plan hash and counts.
