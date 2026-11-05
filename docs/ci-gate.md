---
title: "Wait for CI before deploying"
description: "Hold an automatic git deploy until the commit's CI checks conclude. Works with GitHub and GitLab, and the push is answered at once while the wait runs in the background."
"og:url": "https://docs.levelrail.com/ci-gate"
---

A push to a connected repository normally deploys as soon as the webhook
arrives, even if the commit's tests are still running or have failed. Turn on
the CI gate to hold the deploy until the commit's checks conclude.

## Supported sources

| Source | Gate can read CI |
| --- | --- |
| GitHub, through the GitHub App | Yes |
| GitLab.com and self-managed GitLab, through the GitLab connection | Yes |
| Bitbucket, Gitea, SSH and plain `http://` repository URLs | No |

For a source that cannot be read, saving `wait_for_ci` is refused with a 422.
If the setting is already on and the source turns out to be unreadable, the
deploy is skipped with the reason `cannot read provider status`. The gate never
lets an unreadable source deploy. The provider token is only sent over
`https://`.

## What it does

When `wait_for_ci` is on, an automatic git deploy polls the provider for the
checks of the exact pushed commit:

- All gated checks pass: the deploy starts.
- A gated check fails: the deploy does not start. The failing check names are
  recorded.
- The wait times out: the deploy does not start.

A skipped deploy shows as a failed attempt in the deploy history with the
reason (`skipped by CI gate: ...`) and fires the app's deploy notification
channels like any failed deploy.

The commit is always read from the provider API for the stored repository. The
webhook body is never used to decide which checks count. An annotated tag push
is gated on the commit the tag points at, and that same commit is deployed.

## Which deploys are gated

Gated, when `wait_for_ci` is on:

- Webhook push.
- GitHub release event.
- The missed-webhook poller.
- Scheduled git deploys.
- Build hooks.
- PR comment redeploys.
- Retry build. It rebuilds the connected branch head through the same path as a
  scheduled deploy.

Not gated, on purpose, because a person picked the commit or it never reaches
production:

- Rollback.
- Preview deploys.
- Branch deploys.
- The built-in git push remote.

Known gap: a build hook on a protected environment parks the request for
approval before the gate is reached, so the gate does not run at that point.

A push that deletes the ref is ignored with a note, nothing is gated or
deployed.

## Request flow

The webhook answers `202` at once with `accepted: waiting for CI before
deploying ...`. The gate and the deploy then run in the background, so the
provider's short delivery timeout cannot disable the hook. The delivery row
shows `accepted: waiting for CI`. The real outcome is in the deploy history,
not on the delivery.

- One wait per app and ref. A newer push for the same ref supersedes the
  waiting one. The replaced wait is recorded as a superseded info row and does
  not notify. A push that is the same commit, or provably older than the
  waiting one, is ignored with a note.
- At most `deploys.ci_gate_max_waits_per_app` refs of one app wait at the same
  time (default 4, range 1 to 50, changes apply live, see
  [Runtime settings](/runtime-settings#deploys)). Past the cap the request is
  refused with a 429 and can be retried.
- A gate that passes after a newer push took over the app does not deploy. It
  is recorded as superseded.
- Freeze and approval checks that begin during the wait still apply when the
  gate passes.

## Check states

Check names are trimmed. The platform's own deploy check is ignored so the gate
never waits on itself. If the same name is reported more than once, the worst
state is used.

- No required checks set: every check on the commit counts. If the commit
  reports no checks at all, the gate passes after `APP_CI_GATE_NO_CHECKS_WAIT`.
- Required checks set: only those names are waited for. A required check that
  has not appeared yet counts as pending.

GitHub, check runs and commit statuses:

- `success`, `neutral` and `skipped` count as passed.
- A completed check run with any other conclusion fails.
- A run that is not completed is pending.
- A commit status of `failure` or `error` fails, `success` passes, anything
  else is pending.

GitLab, job and commit statuses for the pushed commit and ref:

| Status | State |
| --- | --- |
| `success` | Passes |
| `failed` | Fails |
| `failed` with allow failure | Soft: does not block, but fails a required check |
| `skipped` | Soft: does not block, but fails a required check |
| `canceled`, `canceling` | Fails |
| `manual` | Waits |
| `manual` with allow failure | Soft |
| Anything else | Waits |

A skipped GitLab job usually means an earlier stage failed, which is why it
cannot satisfy a required name. GitHub is different: a skipped GitHub check
counts as passed even when it is required. This difference is known and not
changed.

On GitLab, a branch push reads statuses for that branch and a tag push for
that tag, so other pipelines on the same commit are ignored. A release-mode
source with no tag reads every ref on the commit.

## Timing

The wait is tuned with environment variables on the server. There are no
runtime settings for these three.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CI_GATE_TIMEOUT` | `15m` | Longest wait before the deploy is skipped |
| `APP_CI_GATE_POLL_INTERVAL` | `15s` | Base poll interval; it backs off while checks stay pending, up to 4 times the base |
| `APP_CI_GATE_NO_CHECKS_WAIT` | `1m` | How long to wait for a first check before treating a commit with none as passing |

A value that is missing, not a duration or not above zero uses the default.
Provider rate limits are respected: a `Retry-After` or `X-RateLimit-Reset`
header delays the next poll. Read errors are retried until the timeout, so a
flaky provider never counts as a pass.

## When the gate is red

The commit is skipped and recorded in the deploy history. The poller notes the
head, so it does not queue the same commit again. A commit whose CI later turns
green is not deployed automatically. To recover, use Retry build, which
rebuilds the current head of the connected branch, or push a new commit.

## Restarts

When the server shuts down, waiting gates end and are recorded as `interrupted
by shutdown`. A webhook that arrives during shutdown gets a 503 so the provider
can redeliver it.

- For a wait started by the poller, the head is left unnoted, so the next poll
  gates it again.
- For a push to the connected branch, the poller's previous head is restored
  when the poller covers the source, so the next poll gates the push again.
  The poller does not cover sources in release mode.
- Otherwise the row says `and nothing retries it automatically`. Push again or
  use Retry build.

## Configure

Dashboard: app, Source tab, "Wait for CI before deploying".

CLI:

```
levelrail apps git-source settings my-app --wait-for-ci --ci-check build --ci-check test
levelrail apps git-source settings my-app --wait-for-ci=false
```

API: `PUT /api/v1/apps/{name}/git-source/deploy-settings` with `wait_for_ci`
(bool, default false) and `ci_required_checks` (list of names, at most 50, each
1 to 200 characters). Both are also returned on
`GET /api/v1/apps/{name}/git-source`.

## Troubleshooting

- `not deployed: cannot read provider status`: the source has no connection
  that can read CI, or its URL is not `https://`. Connect the GitHub App or a
  GitLab connection for that repository.
- `not deployed: the release tag cannot be resolved for the CI gate` or `the
  release tag could not be resolved in time`: the release tag could not be
  turned into a commit. Check the repository access and that the tag exists.
- `not deployed: commit id is not a full SHA`: the trigger gave a short or
  empty commit id, so nothing can be read.
- 422 on save: `wait_for_ci needs a GitHub App or GitLab connection that can
  read this repository's CI status`. The source can never be read.
- 503 on save: `could not confirm CI access right now`. The connection or token
  was unavailable. Try again.
- 429 on a push: `too many refs of this app are waiting for CI, retry later`.
  Raise `deploys.ci_gate_max_waits_per_app` or retry after a wait ends.
- 503 on a webhook during shutdown: `shutting down: redeliver this later`.
  Redeliver it from the provider after the restart.
- `CI checks failed: <names>` or `timed out waiting for CI checks: <names>`: a
  check failed, or the named checks were still pending at the timeout.
