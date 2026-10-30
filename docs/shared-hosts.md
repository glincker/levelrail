---
title: "Shared hosts"
description: What Levelrail touches and does not touch on a server that already runs other containers or a control panel.
"og:url": "https://docs.levelrail.com/shared-hosts"
---

Levelrail is designed to own its server. This page states what the code does when other containers, a panel or a firewall are already there, and marks what is not guaranteed. Every claim names the code behind it.

## What it manages

- **Containers it creates** carry the label `platform-reserved.instance` with this install's id (`internal/spec/labels.go`). Container names are the service name, a dash and a content hash.
- **Networks** are named `<prefix>-app-<appID>` with the default prefix `platform` (`internal/reconcile/application/controller.go`).
- **Ingress** is an embedded Caddy that needs ports 80 and 443 on the host, and 8080 for the dashboard by default ([Installing](/installing)). If another proxy owns 80 and 443, use [Behind an existing proxy](/behind-an-existing-proxy).

## What it does not touch

- **Foreign containers are not adopted.** Levelrail does not import, restart or rename a container it did not create. Reconcile lists containers by service name prefix, then only treats a name as its own when it matches the service name plus a hex hash (`ownsContainer`).
- **Stopped container cleanup is scoped.** The janitor lists only exited or created containers that carry this install's instance label (`internal/docker/prune.go`, `instanceLabelFilter`), and skips any container desired state still wants. See [Janitor](/janitor).
- **Network removal is scoped.** A network with no instance label is excluded from removal, on purpose (`internal/reconcile/application/network_cleanup.go`). Only networks of apps that no longer exist are removed.
- **Volumes**: orphaned volumes are quarantined first (14 days by default) and database volumes (`db-*`) are never removed by the janitor ([Janitor](/janitor)).

## Firewall

Allow-lists use the `DOCKER-USER` chain through `iptables` (`internal/exposure/manager.go`). Rules carry a tag with the infix `exposure:` and the code deletes by position within that chain. Egress control uses an ipset named `levelrail_egress`. Where Docker uses nftables and the chain does not exist, the feature reports that it cannot apply rules instead of creating them.

## Not guaranteed

These could not be confirmed from code and need a test on a real shared host:

1. **Same-name containers without a label.** A container with no instance label is treated as this install's own when its name matches a service name and hash (`ownsInstance`). A foreign container named like that could be removed as stale. The hash makes an accidental match very unlikely.
2. **Dangling image pruning.** The janitor removes untagged images. This review did not confirm that it is limited to images Levelrail built, so another tool's untagged images may be removed.
3. **Orphaned volumes.** A named volume that no Levelrail app uses is a candidate, even if another tool created it. Keep "Delete volumes after quarantine" off on a shared host, and keep preview-only mode on until you have read the preview.
4. **Rule position changes.** Another tool that also edits `DOCKER-USER` can shift rule positions between a read and a delete. No test covers that.
5. **A panel that manages the same Docker daemon** may recreate or stop Levelrail containers. No interop run exists.
6. **A second Levelrail install on the same daemon** is the only shared case with a recorded incident and a fix (the instance label). Use a different data directory and different ports.

## Recommended setup on a shared host

- Leave the janitor in preview mode and read the preview before acting.
- Put Levelrail behind your existing proxy and give it a non-default dashboard port (`LEVELRAIL_DASHBOARD_PORT`).
- Run `levelrail-cli doctor` after install ([Installing](/installing)).
- Prefer a dedicated host for production.
