---
description: Add a readiness health check so a bad release fails before it takes over, read the structured failure, then fix it or roll back.
---

# Zero-downtime deploys with health checks

A deploy that starts a container is not a deploy that works. In this tutorial you will add a readiness check, ship a release that fails it on purpose, read exactly why it failed, and recover. The point is that a broken release never replaces a working one.

## Before you start

- A running Levelrail instance and the CLI logged in to it ([installing](../installing.md)).
- An app that is already running. This tutorial reuses `hello` from [Deploy a Docker app](deploy-a-docker-app.md), which serves the stock `nginx` page.

## 1. Add a readiness check

A readiness probe gates a deploy: the new release only takes over once the probe passes. A liveness probe restarts a container that hangs later. Start with readiness, pointing at a path that does not exist yet:

```bash
levelrail-cli apps health set hello --probe readiness --path /healthz
```

```text
readiness: GET http://:port/healthz (expect 200-299)
liveness: not configured
```

The probe is an HTTP request that must return a status from 200 to 299. You can also run a command inside the container with `--exec` instead of `--path`.

## 2. Ship a release that fails it

The stock nginx page has no `/healthz`, so the probe will get a 404. Deploy a new version:

```bash
levelrail-cli apps deploy hello --image nginx:1.27-alpine
levelrail-cli apps wait hello --timeout 150s
```

```text
waiting for "hello" to converge... (pending)
waiting for "hello" to converge... (pending)
waiting for "hello" to converge... (failed)
"hello" failed to converge ()
```

`apps wait` exits non-zero here, so a script or a CI job fails instead of carrying on. The deploy history shows it:

```bash
levelrail-cli apps deploys list hello
```

```text
ID                IMAGE              STATUS     ROLLOUT  STARTED
dep_NVoflejpBmz-  nginx:1.27-alpine  failed     failed   2026-10-05T02:30:13Z
dep_JJyaXkXlvjmO  nginx:alpine       succeeded  serving  2026-10-05T02:15:57Z
```

## 3. Read the failure

Every failed or blocked deploy carries a structured reason. Open it:

```bash
levelrail-cli apps deploys show hello dep_NVoflejpBmz-
```

```text
status:   failed
image:    nginx:1.27-alpine

failure:  health_check_failed (retryable: false)
  cause:  The container started but never passed its readiness health check.
  fix:    Confirm health.readiness.path returns a success status quickly, or raise
          health.readyTimeout if the app just starts slowly.
  docs:   /deploy-failures#health_check_failed
  log excerpt:
    readiness probe: never became ready (context deadline exceeded), last attempt:
      GET http://127.0.0.1:60290/healthz returned 404, expected 200-299
```

You get the failure code, the plain-language cause, a suggested fix, and the last lines the container logged. Read this before changing anything, and change one thing per attempt. The app's own status agrees:

```bash
levelrail-cli apps status hello
```

```text
Ready  False  RunningNotReady  readiness recheck for "hello-44338b1e": GET .../healthz returned 404, expected 200-299
```

## 4. Fix it, or roll back

Here the probe path was wrong, not the release. Point it at a path that exists and deploy again:

```bash
levelrail-cli apps health set hello --probe readiness --path /
levelrail-cli apps deploy hello --image nginx:1.27-alpine --pull
levelrail-cli apps wait hello
```

```text
waiting for "hello" to converge... (succeeded)
```

`--pull` re-resolves the tag so the deploy runs even when the image is unchanged. If the release itself were broken, you would go back instead:

```bash
levelrail-cli apps deploys rollback-to hello dep_JJyaXkXlvjmO
```

## What the platform guarantees

Levelrail keeps the previous release serving while a new one fails its readiness check, and holds the last good release for a short window after a successful cutover so rollback is instant. The full list, including digest-truthful deploys and the stale-deploy guard, is in [Deploy safety](../deploy-safety.md). Failure codes are in [Deploy failures](../deploy-failures.md).

## Clean up

```bash
levelrail-cli apps health clear hello
```

## Where to go next

- [Deploy from GitHub Actions](deploy-from-github-actions.md): fail the pipeline when a rollout fails.
- [Deploy safety](../deploy-safety.md): freeze windows and the post-cutover hold.
- [Load balancing](../load-balancing.md): health checks across replicas.
