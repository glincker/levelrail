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
providers, such as Bitbucket and Gitea, the gate cannot read checks, logs a
warning and the push deploys as before. The missed-webhook poller and release
tag deploys are not gated.

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
`GET /api/v4/projects/:id/repository/commits/:sha/statuses`, up to 10 pages of
100. The status name is the job name, so `ci_required_checks` entries are the
names of your GitLab CI jobs, for example `build` and `test`.

| GitLab status | Gate result |
| --- | --- |
| `success`, `skipped` | Passes |
| `failed` | Fails, unless the job has `allow_failure`, then it passes |
| `canceled` | Fails |
| `manual` | Passes only if the job has `allow_failure`, otherwise pending |
| `created`, `pending`, `running` | Pending |
| Any other value | Pending |

Pending is never treated as a pass. If the API returns an error, the deploy does
not start on that read. If the commit has more than 10 pages of statuses, the
read fails instead of deciding on a partial list. A rate limit response (429) is
retried until the timeout. As with GitHub, the deploy never starts on an
unknown state, and if the wait times out it does not start.

The request does not follow redirects, so the token is only sent to the
configured GitLab host.

Troubleshooting:

- Every check stays pending or the read fails: the connected token needs read
  access to the project and its pipelines.
- A job fails but the deploy still starts: the job has `allow_failure: true`.
  Remove it, or leave the job out of the required checks.
- The deploy waits and then times out: a `manual` job without `allow_failure`
  stays pending until someone runs it. Run the job, or set `allow_failure: true`
  on it.
- A required check never appears: the name must match the GitLab job name
  exactly.

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
