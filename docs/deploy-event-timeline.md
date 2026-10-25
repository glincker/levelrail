---
title: "Deploy event timeline"
description: "One screen per deploy that tells the whole story in order: trigger, build, push, hooks, readiness, cutover, post-deploy watch, rollback, and traffic before and after cutover, live while it runs and replayable after."
"og:url": "https://docs.levelrail.com/deploy-event-timeline"
---

The event timeline answers "what happened in this deploy, and in what order" without stitching together the build log, the step list, the gate view and the traffic page. It reads the sources that already exist and lays them out as one ordered list. The [fleet deploy timeline](deploy-timeline.md) shows many deploys on one axis. This page is about one deploy in depth.

## What is on it

Events appear oldest first:

- **Trigger**: the git push or webhook that started it, a manual or API request, a promotion or an automatic rollback, with the branch, commit, author and, for API and CLI requests, the token or user that made the call.
- **Build**: detecting, building and the build output, folded into a collapsed group you open when you want it.
- **Image push**, any **pre-deploy hook** and the pipeline steps the reconciler records (pull, create, start).
- **Readiness**: each probe with its target, status and latency, then the stability gate.
- **Cutover**: the moment traffic moved.
- **Post-deploy watch**: crashloop, out-of-memory kill, container exit, a rise in 5xx rate and a latency regression, each with the reason the watch recorded.
- **Rollback or success**: the automatic rollback, or the final outcome.
- **Traffic impact**: request rate, 5xx rate and p95 latency in a window before cutover next to the same window after it.

A summary beside the list shows how long each phase took.

## Live and replay

While a deploy runs, the page follows it over server-sent events and adds events as they happen. When the deploy and its watch window have settled, the server freezes the timeline. Opening that deploy later replays the frozen copy, so the page does not change even after the traffic samples or hook runs it was built from age out.

A frozen timeline is kept for `APP_DEPLOY_TIMELINE_RETENTION` and then dropped. After that the page falls back to assembling the story from whatever sources remain.

## Compare two deploys

Pick **Compare with** on the page, or run the CLI with `--compare`. You get a table of time per phase for both deploys with the difference, the signals only one of them produced (a crashloop, a rollback), and the two event lists next to each other. Build output is left out of the comparison.

## Where to find it

- Dashboard: **Timeline** in the details drawer of a deployment on the [Deployments page](deployments-page.md), or `/deploy-events/<deploy-id>`.
- CLI:

```sh
levelrail-cli deployments timeline <deploy-id>
levelrail-cli deployments timeline <deploy-id> --follow
levelrail-cli deployments timeline <deploy-id> --compare <other-deploy-id>
```

`--follow` streams until the deploy settles and exits non-zero when it failed or was rolled back. Build output is hidden unless you pass `--logs`.

- API: `GET /api/v1/deployments/{id}/timeline`, `GET /api/v1/deployments/{id}/timeline/stream` and `GET /api/v1/deployments/{id}/timeline/compare?with={id}`. They need the read ability and only return deploys of apps you can read.
- MCP: the read tool `get_deploy_timeline` returns the ordered events, the time per phase and a one line traffic summary. It counts build output lines instead of returning them.

## Settings

| Variable | Default | Effect |
| --- | --- | --- |
| `APP_DEPLOY_TIMELINE_RETENTION` | `720h` | How long a frozen timeline is kept. |
| `APP_DEPLOY_TIMELINE_MAX_LOG_LINES` | `500` | Build output lines kept per timeline. The newest are kept and the page says how many were left out. |
| `APP_DEPLOY_TIMELINE_IMPACT_WINDOW` | `5m` | Length of the window before and after cutover used for the traffic impact. |
| `APP_DEPLOY_TIMELINE_ACTOR_WINDOW` | `1m` | How far from the deploy start the audit log is searched for who made the request. |
| `APP_DEPLOY_TIMELINE_SETTLE_IDLE` | `5m` | How long after a deploy finishes, with no closing summary, before it counts as settled. |

## Limits

- The actor comes from the audit log by time and request path, because audit entries carry no deploy id. If two requests for the same app land within the window it can name the wrong one, and it stays empty when nothing matches. Git push and webhook deploys show the commit author instead.
- Traffic impact needs request metrics for the app. Apps with no traffic in either window show no impact card.
- Only the latest run of each hook is stored, so a hook shows up only if it ran during this deploy and has not been overwritten by a later one.
- Build output for a running deploy has no per-line timestamps, so its lines sit at the start of the build step until the finished log replaces them.
