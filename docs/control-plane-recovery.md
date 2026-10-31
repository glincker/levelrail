---
title: "Control plane recovery"
description: What happens when the single control plane machine is lost, what survives, and how to recover onto a new host.
"og:url": "https://docs.levelrail.com/control-plane-recovery"
---

## One control plane, no HA

A Levelrail install has one control plane machine. There is no high availability today. [ADR 037](https://github.com/glincker/levelrail/blob/main/adr/037-control-plane-ha-replication.md) (accepted as a spike, opt-in) and [ADR 047](https://github.com/glincker/levelrail/blob/main/adr/047-warm-standby-control-plane-promote-drill.md) (proposed) describe replication and a warm standby. The roadmap lists high availability as "design and drill, not a release goal yet" ([Roadmap](/roadmap)).

## Warm standby status

Proposed only. The ADR describes a second machine and a manual `levelrail standby promote`. No `standby` command exists in `cmd/` in this release, so do not plan around it.

## What is lost if it dies

Running workloads keep running. The route table keeps answering from the last sealed snapshot, and agents keep their containers up ([Resilience](/resilience)).

Stopped until recovery:

- Deploys, rollbacks, scaling and builds.
- The dashboard, API and CLI.
- Ingress changes, certificate issuance and renewal.
- Alerts and notifications, and scheduled backups.
- Wake of sleeping apps and the maintenance responses held by the control plane.

Known gap: containers use restart policy `no`, so a host reboot during an outage leaves apps down ([Feature status](/feature-status)).

## What a backup holds

The encrypted off-box backup holds `levelrail.db`. It does not hold `alerting.db`, volume data or the age identity. Details and the file you must copy yourself are in [Control plane backup](/control-plane-backup). The master key and agent CA come from the escrow bundle ([Disaster recovery](/disaster-recovery)).

## Recover onto a new host

1. Provision a new machine and install the same or a newer release.
2. Recover the master key from the escrow bundle or `APP_MASTER_KEY`.
3. Run a restore dry run, then the real restore from the newest off-box backup. Both are offline commands.
4. Copy back `alerting.db` if you saved it.
5. Start the control plane, update DNS to the new address and let agents reconnect.

Step by step with commands: [the runbook](/disaster-recovery#runbook-the-control-plane-machine-is-gone). Restoring on a new machine: [Control plane backup](/control-plane-backup#restoring-on-a-new-machine).

## RPO and RTO

No numbers are promised. The documents say:

- **RPO** is the interval between off-box backups. A restore loses changes since the newest backup.
- **RTO** is however long a person takes to provision, restore and repoint DNS. ADR 047 states this directly.
- The runbook was run end to end on one machine against a local S3 server. It was not timed and not run across real regions ([Disaster recovery](/disaster-recovery)).

To get a real RPO, set the backup schedule and retention yourself and run the drill regularly. Measure your own RTO.
