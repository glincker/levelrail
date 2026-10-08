---
title: Zero-downtime deploys without Kubernetes
description: "How to ship containers to your own servers with no downtime and a fast rollback, using readiness probes, digest-pinned images, and freeze windows. No Kubernetes."
---

# Zero-downtime deploys without Kubernetes

You do not need a cluster scheduler to deploy without dropping requests. You need three things: a check that the new version is ready before it gets traffic, the old version still available if it is not, and a way to know the running content is the content you shipped. This guide shows how Levelrail does each on plain Docker.

## 1. Gate traffic on readiness

Blue-green and rolling deploys start the new container, wait for its readiness probe, then move traffic and retire the old one. If readiness never passes, the old release keeps serving and the deploy fails with a reason (`ReadinessFailed`, `ExitedDuringReadiness` or `OOMKilledDuringReadiness`). The `recreate` strategy is the exception: it stops the old release first.

Declare the probe in `app.yaml`:

```yaml
health:
  readiness: { path: /healthz, interval: 5s, timeout: 2s }
  readyTimeout: 90s
```

`readyTimeout` defaults to 60 seconds. Keep the endpoint cheap: a probe that calls other services fails for reasons unrelated to your release. Every field is in the [app spec reference](/app-spec-reference).

## 2. Deploy and watch it

```bash
levelrail-cli apps deploy web --image registry.example.com/org/web:1.4.0
levelrail-cli apps deploys list web
```

`apps deploys wait` blocks until a deploy settles, which suits CI. It exits 0 when healthy, 7 when failed, canceled, superseded or blocked, and 6 on timeout.

## 3. Trust the content, not the tag

A tag like `latest` can point at different content over time. Levelrail resolves the tag to its digest at deploy time and pins the deploy to it, so a moved tag cannot change what serves. The deploy history records how the digest was found. If the registry is unreachable, a cached image may deploy and the dashboard flags it. To refuse that fallback:

```bash
levelrail-cli apps deploy web --image registry.example.com/org/web:1.4.0 --pull
```

## 4. Roll back to exactly what ran

```bash
levelrail-cli apps deploys rollback-to web DEPLOY_ID
```

This redeploys the image a past successful deploy ran, pinned by digest. After a cutover the previous release stays running briefly (`APP_DEPLOY_PREVIOUS_RELEASE_HOLD`), so an immediate rollback needs no pull. Rollback answers 410 if the old image was garbage collected and 409 if its tag now points at different content. Pinned rollback tags are never pruned as dangling images.

## 5. Stop old builds from winning

Automatic deploys carry a per-app sequence number and commit order, so a slow build that finishes late cannot overwrite a newer one; it is marked superseded. Manual deploys and rollbacks are never rejected.

## 6. Freeze risky windows

```bash
levelrail-cli apps freeze set web --cron "0 17 * * 5" --duration 64h --timezone Europe/Berlin --reason "weekend freeze"
```

While a window is active, webhook and pipeline deploys are held and run when it ends. A manual deploy needs an override:

```bash
levelrail-cli apps deploy web --image registry.example.com/org/web:1.4.1 --override-freeze --override-reason "security fix"
```

## Limits to know

- Remote nodes do not yet report the running image ID, so a digest mismatch is not detected there.
- Zero downtime is per deploy on running hosts. It does not replace redundancy: with one replica on one node, a node failure is still an outage.
- Levelrail is beta. These guarantees have tests behind them but less production time than older tools.

## Next steps

- [Deploy safety](/deploy-safety)
- [Deploy failures](/deploy-failures)
- [Zero-downtime tutorial](/tutorials/zero-downtime-deploys-with-health-checks)
- [Deploying apps](/deploying-apps)
