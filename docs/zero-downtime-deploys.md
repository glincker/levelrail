---
title: "Zero-downtime deploys with Docker: readiness-gated cutover and rollback"
description: "How Levelrail ships without downtime: blue-green, rolling or recreate strategies gated on real readiness probes and digest-pinned images."
"og:url": "https://docs.levelrail.com/zero-downtime-deploys"
---

**A deploy is live only when it is actually ready**

Traffic cuts over after the new container passes a real readiness probe, the previous release stays available, and four guarantees keep the wrong build from serving.

[Read deploy safety](/deploy-safety) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## Four guarantees between a deploy and serving traffic

<CardGroup cols={3}>
  <Card title="Digest-truthful deploys">
    A deploy is pinned to the image digest it resolved, so a moved tag cannot change what serves. Rollbacks deploy the recorded pinned reference.
  </Card>
  <Card title="Stale-deploy guard">
    Automated deploys carry a per-app sequence number and commit order, so an older build still in flight cannot overwrite a newer one. Manual deploys and rollbacks are never rejected.
  </Card>
  <Card title="Freeze windows">
    Block automated deploys on a cron schedule, for example every Friday evening, and release them afterwards.
  </Card>
  <Card title="Instant rollback window">
    The previous release is held briefly after cutover, and prior images stay pinned so garbage collection cannot remove a rollback target.
  </Card>
  <Card title="Three strategies">
    Blue-green, rolling or recreate, all gated on readiness and liveness probes.
  </Card>
  <Card title="A reason for every state">
    The reconciler records a status condition with a reason after each pass, so a stuck deploy explains itself.
  </Card>
</CardGroup>

## The order of a cutover

Deploys, restarts and rollbacks follow the same order, and every step waits for the one before it:

1. The new release starts and passes its readiness probe (and the stable window, when one is set).
2. The route moves to it. Until then the release that was serving keeps every request.
3. The previous release keeps answering in-flight requests for the drain timeout (`APP_DEPLOY_DRAIN_TIMEOUT`), then stays held for a rollback (`APP_DEPLOY_PREVIOUS_RELEASE_HOLD`) and is stopped after that.

Two orders lose requests, and the ingress tests check both stay out of the cutover path: stopping the serving release before the route has moved, and moving the route to a release that is running but not listening yet. In both cases the ingress retries the dial for `APP_INGRESS_RETRY_WINDOW`, so a client with a shorter timeout sees a failed request.

## What stays running after a cutover

Two different things can leave an older release on disk, and neither is a standing replica.

- **The held previous release.** Exactly one: the newest release before the current one keeps running for `APP_DEPLOY_PREVIOUS_RELEASE_HOLD` (at least the drain timeout, and the watch window when auto-rollback is on), then it is stopped and removed. Older releases are removed at cutover.
- **Deploy URL copies.** Opening an old deploy at its own URL uses a one-replica copy of that deploy's pinned image. Copies are created asleep, so nothing runs until the URL is visited, and they sleep again after `APP_DEPLOY_URL_IDLE_MINUTES`. At most `APP_DEPLOY_URL_MAX_COPIES` (default `1`) exist per app, within the newest `APP_DEPLOY_URL_KEEP` deploys (default `3`, `0` turns the feature off). Copies are removed when the app is deleted or falls out of the window.

A copy has no volumes and no host ports, so apps that own data or a host claim get none: any app with a named volume, a writable bind mount, a published host port or a fixed host port. Such an app keeps only the held previous release, and its rollback uses the same volumes through the normal cutover. No copy is created when the host has less than `APP_DEPLOY_URL_MIN_FREE_MEMORY_MB` available (default `1024`), or when the app's memory limit is above `APP_DEPLOY_URL_MAX_MEMORY_PERCENT` percent of host RAM (default `25`); `0` turns either check off.

A stopped app stays stopped: no revive path starts an older release while the app is suspended.

A rollback to a release that is still held reuses it only when its container is the same one that served before. A rollback that has to start a fresh container goes through the full readiness gate, so traffic stays on the current release until the restored one is ready. That takes the readiness time plus the stable window.

Rollbacks follow the same three steps:

- A held container that was stopped and is started again for the rollback counts as a new process. It passes readiness before it takes traffic, even though the container itself is older than the rollback.
- When the restored release gets a new container (for example a rollback by digest), a held container that runs the same image serves while the new one warms up.
- When the rollback lands on a held release that is still running, the release it replaces keeps answering in-flight requests for the drain timeout, as after a deploy.

## Frequently asked questions

<AccordionGroup>
  <Accordion title="What does zero downtime require from my app?">
    A readiness endpoint that returns success only when the app can serve. Cutover waits for it.
  </Accordion>
  <Accordion title="What if the new version never becomes ready?">
    Traffic stays on the current release and the deploy is reported as failed, with logs and a reason.
  </Accordion>
  <Accordion title="Can a stale build overwrite a newer one?">
    Not for automated deploys: the stale-deploy guard skips them. Manual deploys and explicit rollbacks always run.
  </Accordion>
  <Accordion title="Does it work on one server?">
    Yes. The same cutover logic runs on a single node.
  </Accordion>
</AccordionGroup>

## Ship without holding your breath

Read the deploy safety guide or try it on a spare server.

[Read deploy safety](/deploy-safety) | [View on GitHub](https://github.com/glincker/levelrail)

## Probe a redeploy under load

`scripts/zero-downtime-load/run.sh` checks the cutover order above with real traffic. It starts a throwaway dev-mode control plane, creates one app from `traefik/whoami`, sends requests to it at a steady rate through the HTTPS ingress, and redeploys the app while the requests continue. It then reports how many requests failed.

It needs a checkout of this repository (it builds `./cmd/levelrail` from the repo root), Go, a Docker daemon reachable through the current Docker context, and `curl`, `awk` and `sort`. It uses its own ports and never touches the dev app on port 8080. Dev mode and the fixed dev token are for local use only.

```bash
scripts/zero-downtime-load/run.sh -r 20 -s 8
```

Flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-r` | `20` | Requests per second. Each request runs in the background, so a slow one does not delay the schedule. |
| `-d` | `5` | Number of redeploys. The script defines four image tags, so use `3` or less. The default asks for a tag that does not exist, so pass `-d` explicitly. |
| `-s` | `8` | Seconds to wait after each redeploy before the next one. |

The app starts on tag `v1.10.3`. Redeploy 1 uses `v1.10.2`, redeploy 2 uses `v1.10.1` and redeploy 3 uses `v1.10.0`.

Environment variables:

| Variable | Default | Meaning |
| --- | --- | --- |
| `ZDL_DIR` | a directory under `/private/tmp` | Work directory for the binary, data, load log and control plane log. |
| `ZDL_API_PORT` | `18480` | Port of the throwaway control plane API. |
| `ZDL_INGRESS_PORT` | `18482` | HTTPS port of the throwaway ingress, host `zdl.localhost`. |
| `ZDL_ENV` | empty | Extra `KEY=VAL` pairs for the control plane, separated by spaces. |
| `ZDL_IMAGE` | `traefik/whoami` | Image repository to deploy. |

The script removes the app and any containers named `zdl-` when it exits.

### Reading the output

The script prints these lines, in order:

1. `create: <status>`, the HTTP status of the create call.
2. For each redeploy, `deploy N: <status>` and then either `deploy N: new release answering` or `deploy N: NEVER CUT OVER`. The second means the route did not move to the new release within the wait limit of 360 polls, about three minutes.
3. `requests N`, the number of requests sent, failed ones included.
4. `failures N`, the number of requests that did not return a 2xx status. A request that got no response is counted with status `000`.
5. `latency p50 ... p95 ... p99 ...`, measured over 2xx responses only.
6. `longest gap between 2xx`, in seconds. At the default rate, requests are 0.05 s apart, so a gap of a second or more means no 2xx response arrived for that long.
7. Only when there are failures: `failing window`, the first and last failing timestamps as epoch seconds, and a count per status code.
8. `control plane log:`, the path to the server log.

The exit status is `0` when every request returned 2xx and every redeploy cut over. It is `1` when any request failed, when any redeploy printed `NEVER CUT OVER`, when the build fails, or when the app never answers. `-d` defaults to 3, the number of redeploys the script has image tags for, and values above that are refused.

A deploy that says `NEVER CUT OVER` fails the run even with zero failed requests, because the requests only reached the old release.

### Recorded result

One recorded run exists. Two of three redeploys cut over without failed requests. The third is unexplained and still open.

Everything else is `pending run`:

- Rates other than 20 requests per second.
- Settle times other than 8 seconds.
- Images other than `traefik/whoami`.
- Rollbacks while requests are running.
- A real VPS, a public domain or more than one node.

This section does not show zero downtime on a host. It shows what the probe measures and what it has measured once.

## Troubleshooting

For issues with zero-downtime deploys, readiness gates, or rollbacks, see [Deploy safety](deploy-safety.md) for safety mechanisms and [Deploying apps](deploying-apps.md) for deploy troubleshooting. Check [Deploy failures](deploy-failures.md) for structured failure codes and what each one means.
