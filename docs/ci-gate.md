# Wait for CI before deploying

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

GitHub is supported. For other providers the gate cannot read checks, logs a
warning and the push deploys as before. The missed-webhook poller and release
tag deploys are not gated.

## Which checks count

- No required checks set: every check run and commit status on the commit
  counts. If the commit reports no checks at all, the gate passes after
  `APP_CI_GATE_NO_CHECKS_WAIT` so a repository without CI is not blocked.
- Required checks set: only those names are waited for. A required check that
  has not appeared yet counts as pending.
- The platform's own deploy check is ignored, so the gate never waits on itself.

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
