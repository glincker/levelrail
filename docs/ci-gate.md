---
title: "Wait for CI before deploying"
description: "Hold a deploy until the commit's CI checks conclude. Without the gate, a push deploys as soon as the webhook arrives, even if tests are still running or failed."
"og:url": "https://docs.levelrail.com/ci-gate"
---

A push to a connected repository normally deploys as soon as the webhook
arrives, even if the commit's tests are still running or have failed. Turn on
the CI gate to hold the deploy until the commit's checks conclude.

## What it does

When `wait_for_ci` is on, a push deploy polls the git provider for the commit's
check runs and commit statuses:

- All gated checks pass: the deploy starts.
- A gated check fails: the deploy does not start. The failing check names are
  recorded.
- The wait times out: the deploy does not start.

A skipped deploy shows as a failed attempt in the deploy history with the
reason (`skipped by CI gate: ...`), appears as a note on the webhook delivery,
and fires the app's deploy notification channels like any failed deploy.

GitHub and GitLab (GitLab.com and self-managed) are supported. For other
providers, such as Bitbucket and Gitea, the gate cannot read checks. With wait
for CI on, the deploy is skipped with the reason "cannot read provider status".
With wait for CI off nothing changes. The missed-webhook poller is not gated.

The checks are always read from the provider API for the pushed commit. The
webhook body is never used to decide whether CI passed.

## Which checks count

- No required checks set: every check run and commit status on the commit
  counts. If the commit reports no checks at all, the gate passes after
  `APP_CI_GATE_NO_CHECKS_WAIT` so a repository without CI is not blocked.
- Required checks set: only those names are waited for. A required check that
  has not appeared yet counts as pending.
- The platform's own deploy check is ignored, so the gate never waits on itself.

## GitLab

For a GitLab project the gate reads the statuses of the pushed commit from
`GET /api/v4/projects/:id/repository/commits/:sha/statuses` with
`ref=<pushed branch>` and `per_page=100`. If the push carries no branch, the
configured branch is used. It reads up to 10 pages. If the commit has more, the
gate fails at once instead of deciding on a partial list.

The status name is the job name, so `ci_required_checks` entries are the names
of your GitLab CI jobs, for example `build` and `test`. Names are trimmed.

The project path comes from the stored repository URL, with the instance path
prefix removed if the instance lives under one. It is never taken from the
webhook body. The pushed SHA must be 40 or 64 lowercase hex characters, or the
deploy is skipped with a reason.

| GitLab status | No required checks | Name is required |
| --- | --- | --- |
| `success` | Passes | Passes |
| `skipped` | Passes | Fails |
| `failed` | Fails | Fails |
| `failed` with `allow_failure` | Passes, does not block | Fails |
| `canceled`, `canceling` | Fails | Fails |
| `manual` | Pending | Pending |
| `manual` with `allow_failure` | Passes, does not block | Fails |
| Any other value (`created`, `pending`, `running`) | Pending | Pending |

Pending is never treated as a pass. If the same job name appears in several
pipelines on the commit, the worst state wins: failed, then pending, then
skipped or allowed failure, then passed. A read error or a rate limit response
(429) is retried until the timeout. The deploy never starts on an unknown
state, and if the wait times out it does not start.

The request does not follow redirects, so the token is only sent to the
configured GitLab host.

Known limits:

- In release (tag) mode the statuses are not scoped to a ref yet, so every
  pipeline on the commit counts.
- The webhook request is held while the gate waits, up to the gate timeout.
- No run against a real GitLab instance has been recorded yet.

Troubleshooting:

- Every check stays pending or the deploy is skipped with "cannot read provider
  status": the repository must be connected over an https URL on the connected
  GitLab host, and the connection token must still be valid and have read access
  to the project and its pipelines.
- A job fails but the deploy still starts: the job has `allow_failure: true` and
  you did not list it as required. Remove `allow_failure`, or list the job as
  required, which then needs it to succeed.
- A required job is skipped and the deploy is blocked: a skipped job usually
  means an earlier stage failed. A required name must succeed, so skipped does
  not count.
- A required job with `allow_failure: true` blocks the deploy when it fails or
  is skipped: a required name must succeed, even if the job is allowed to fail.
- The deploy waits and then times out: a `manual` job stays pending until
  someone runs it. Run the job, or set `allow_failure: true` on it and leave it
  out of the required checks.
- A required check never appears: the name must match the GitLab job name
  exactly. Leading and trailing spaces are trimmed.

## Configure

Dashboard: app, Source tab, "Wait for CI before deploying".

CLI:

```
levelrail apps git-source settings my-app --wait-for-ci --ci-check build --ci-check test
levelrail apps git-source settings my-app --wait-for-ci=false
```

API: `PUT /api/v1/apps/{name}/git-source/deploy-settings` with `wait_for_ci`
(bool, default false) and `ci_required_checks` (list of names). Both are also
returned on `GET /api/v1/apps/{name}/git-source`.

## Server settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CI_GATE_TIMEOUT` | `15m` | Longest wait before the deploy is skipped |
| `APP_CI_GATE_POLL_INTERVAL` | `15s` | Base poll interval; it backs off while checks stay pending |
| `APP_CI_GATE_NO_CHECKS_WAIT` | `1m` | How long to wait for a first check before treating a commit with none as passing |

Provider rate limits are respected: a `Retry-After` or `X-RateLimit-Reset`
header delays the next poll.
