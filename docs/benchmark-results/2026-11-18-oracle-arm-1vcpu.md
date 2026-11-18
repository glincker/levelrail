# Oracle ARM 1 vCPU, v1.5.3, 2026-11-18

Hardware: Oracle Cloud Ampere A1, aarch64, 1 vCPU, 5.8 GB RAM, Ubuntu 24.04. Two VMs on one private network: the server under test, and a second VM as load generator. Server upgraded from v0.9.5 to v1.5.3 during the run. Raw figures: [2026-11-18-oracle-arm-1vcpu.json](2026-11-18-oracle-arm-1vcpu.json). Addresses, hostnames, user names, tokens, keys, restore point ids and bucket names are removed. No feature status row changes because of this run.

## Results

| Check | Result | Figure |
| --- | --- | --- |
| `levelrail-cli upgrade --apply --version v1.5.3` | FAIL | Refused at the guard step: no release repository configured, and this path has no `--repo` flag |
| `levelrail self-upgrade --to v1.5.3 --repo glincker/levelrail --yes` | Pass, slower than hoped | About 50 s, 10 of 10 steps ok, signature verified, data intact (table md5 match), agent reconnected. Ingress and `/readyz` were down about 8 s |
| Idle footprint, v1.5.3 | Pass | RSS 212 to 223 MB, about 0.4 percent of one vCPU, 6 samples 30 s apart starting 10 min after the upgrade |
| Deploy to first HTTP 200 | Pass | 12.5 s image cached, 15.4 s cold pull |
| Redeploy under 200 rps, 90 s, open loop | Pass | 18000 of 18000 ok. p50 2.04, p95 3.01, p99 7.94 ms (baseline without deploy: p50 1.99, p95 2.58, p99 5.57). Redeploy 4.6 s |
| Bad deploy | Contained, with a flaw | Old release kept serving (654 of 654 probes ok). Status read "ready (live)" for about 60 s before flipping to failed |
| Rollback | Pass | Command 0.29 s, previous release serving 11.8 s after |
| Postgres backup and restore, local S3 | Pass | Backup about 3 s (38276 bytes, sealed), restore 2.6 s, 1000 rows, md5 match |
| Reboot | Local recovery pass, external FAIL | Local recovery about 56 s from the command (about 30 s from kernel boot), data intact. External ports unreachable: the host firewall returned to the provider default (host configuration, not a product defect) |

## Failures and flaws

- The documented upgrade path `levelrail-cli upgrade --apply` did not work on this install.
- The working path (`levelrail self-upgrade`) caused an ingress gap of about 8 s, with one early failed boot attempt logged.
- A bad deploy looked ready and live for about 60 s before it was marked failed.
- The v1.5.3 `release-manifest.json` lists amd64 only for a multi-arch release. The arm64 upgrade still passed.
- The upgrade plan said 0 migrations would run for a schema move that then ran one.

## Not run

Upgrade from older than v0.9.5, Compose database backups, MySQL, point in time recovery, rates above 200 rps, replica kill, node drain, control plane restart under load, public ACME, hard power off, automatic rollback after a failed verification. Numbers here say nothing about capacity: one vCPU generator, 200 rps, one app.
