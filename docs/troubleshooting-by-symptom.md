---
title: "Troubleshooting by symptom"
sidebarTitle: "Troubleshooting by symptom"
description: Start from what the dashboard shows. Each symptom lists what you see, why it happens, and the fix in the dashboard, the CLI and the API.
"og:url": "https://docs.levelrail.com/troubleshooting-by-symptom"
---

Find the heading that matches what you see. Each one gives the likely cause and the shortest fix. Deeper pages are linked at the end of each section. For the full list of problems and what each doctor finding means, see [Troubleshooting](/troubleshooting).

Two commands help with almost every symptom:

```bash
levelrail-cli attention
levelrail-cli doctor
```

## Crashloop

**What you see.** The app shows CrashLoopBackOff, or the container keeps restarting.

**Why.** The container exits right after it starts. Levelrail waits longer between restarts each time (5s, 10s, 20s, up to 2 minutes) so a bad container does not load the host.

**Fix.**

- Dashboard: open the app, then Logs. The last 200 log lines of a crashloop are shown for you.
- CLI: `levelrail-cli apps logs <name> --tail 50`, then `levelrail-cli apps status <name>` for the countdown to the next restart. Fix the image or its settings and redeploy. To go back to the last good image: `levelrail-cli apps rollback <name>`.
- API: `GET /api/v1/apps/{name}/status` returns the reconcile conditions, and `GET /api/v1/apps/{name}/deploys` lists the deploys.

Deeper: [Troubleshooting](/troubleshooting), [Observability](/observability#alert-rules).

## Deploy failed

**What you see.** A deploy ends as failed or blocked.

**Why.** Levelrail classifies the cause and stores a suggested fix with the deploy: a build error, a failed readiness probe, a missing secret, a full disk, and so on.

**Fix.**

- Dashboard: open the deploy and read the failure card. Fixes it can apply have a button.
- CLI: `levelrail-cli apps deploys show <name>` prints the failure code and fix. `levelrail-cli apps deploys logs <name> <deploy-id>` prints the full build log. `levelrail-cli apps diagnose <name>` explains a failed deploy.
- API: `GET /api/v1/apps/{name}/deploys/{deployId}`, read the `failure` object and its `suggested_fix`.

Deeper: [Deploy failures](/deploy-failures), [Fix-it engine](/fix-it).

## Domain not live

**What you see.** The domain opens an error page, never resolves, or the DNS check stays red.

**Why.** The A record does not point at this server yet, it has not propagated, or you edited a zone your registrar does not use.

**Fix.**

- Dashboard: the app's Domains tab shows the DNS check for each domain.
- CLI: `levelrail-cli domains check <app> <domain>` runs a real DNS lookup and reports whether the domain reaches this control plane. From your own machine, `dig +short <domain>` shows what the world sees.
- API: `GET /api/v1/certificates` shows what was issued for each domain.

Until DNS is ready, the sslip.io address shown in the dashboard works without any DNS setup.

Deeper: [Domains and ingress](/domains-and-ingress#setting-up-dns), [DNS zones](/dns-zones).

## Certificate pending

**What you see.** The domain shows Internal certificate or No certificate, the browser warns about the certificate, or a Renewal stalled badge appears.

**Why.** Let's Encrypt cannot verify the domain yet. Usual causes are DNS not pointing at this server, ports 80 and 443 not reachable from the internet, or a rate limit.

**Fix.**

- Dashboard: Infrastructure, Domains shows a badge per domain with the last renewal attempt and the next retry. The Doctor page offers the fix Renew now for a stalled certificate.
- CLI: `levelrail-cli domains certificates` lists every certificate with its `RENEWAL` state (`ok` or `stalled`).
- API: `GET /api/v1/certificates` returns `kind`, `days_left`, `last_renewal` and `next_retry_at`.

Deeper: [ACME verification runbook](/acme-verification-runbook), [Domains and ingress](/domains-and-ingress#where-to-see-certificate-status).

## App unreachable

**What you see.** The app is running but the browser cannot reach it, or the dashboard shows the red "Can't reach the control plane" banner.

**Why.** For an app: no domain or port is attached, a firewall blocks ports 80 and 443, or the readiness probe fails. For the banner: the control plane restarted or your reverse proxy lost its upstream.

**Fix.**

- Dashboard: the app's Overview page shows the reconcile conditions with a reason for each.
- CLI: `levelrail-cli apps status <name>` prints the same conditions. `levelrail-cli apps restart <name>` recreates the container.
- API: `GET /api/v1/apps/{name}/status` returns the conditions. `GET /healthz` answers without a session. If it answers on the server but not through your proxy, the proxy is the problem.

Deeper: [Troubleshooting](/troubleshooting), [Health checks](/app-spec-reference#health-checks).

## Disk full

**What you see.** Builds fail with "no space left on device", writes are slow, or the doctor reports low disk space.

**Why.** Images, build cache, stopped containers and old deploy attempts pile up on the server.

**Fix.**

- Dashboard: Settings, Advanced, Automatic cleanup. It starts in preview only; turn off "Preview only" to let it delete.
- CLI: `levelrail-cli janitor run --dry-run` shows what would be removed. `levelrail-cli janitor run` does one cleanup pass now.
- API: `GET /api/v1/system/janitor` returns the cleanup state, and `GET /api/v1/system/janitor/runs` lists past passes.

Deeper: [Automatic cleanup](/janitor), [Cleanup center](/cleanup-center).

## Update stuck

**What you see.** Settings, Updates shows an upgrade that does not finish, or the version did not change.

**Why.** Only one upgrade runs at a time. A failed preflight check (backup, disk, signature) also blocks it.

**Fix.**

- Dashboard: Settings, Updates shows the running attempt and each blocked check with a fix button.
- CLI: `levelrail-cli upgrade status` shows the running attempt and the last finished one. `levelrail-cli upgrade --attempts` lists every attempt with its steps. To undo a failed attempt, `levelrail-cli upgrade rollback <attempt>`.
- API: `GET /api/v1/updates/self-upgrade/attempts` lists the attempts, and `POST /api/v1/updates/self-upgrade/attempts/{id}/retry` re-runs a failed one.

Deeper: [Upgrade safely](/upgrade-safely), [Rolling back the control plane](/rollback).

## Backup failed

**What you see.** A backup shows failed, the doctor warns that the control plane backup is stale, or the Backups page shows failing backups.

**Why.** The target credentials are wrong or expired, the bucket is unreachable, or the server disk is full.

**Fix.**

- Dashboard: Backups shows the error on the failed attempt.
- CLI: `levelrail-cli backups health` shows the health view, and `levelrail-cli backups list-all` shows the attempt history. For the control plane itself, `levelrail-cli control-plane-backups create` takes a fresh snapshot.
- API: `GET /api/v1/backups/health` returns the last backup and last verified restore per resource, and `GET /api/v1/backup-targets` lists the targets.

Deeper: [Backups and storage](/backups-and-storage), [Control plane backup](/control-plane-backup).
