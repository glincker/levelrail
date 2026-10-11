# Runbook: upgrades

## Control plane

1. get_upgrade_readiness: release signature, Docker version, free disk, recent backup, breaking changes needing acknowledgement.
2. If the backup is stale, an operator takes one first: `levelrail-cli upgrade backup-now`.
3. Read the breaking changes to the user. Ones marked ack need an explicit acknowledgement id.
4. The operator applies it: `levelrail-cli upgrade --apply --version <v> --ack-breaking <id>` or `sudo levelrail self-upgrade --to <v>`. A failed start rolls back automatically and is recorded.

History: list_upgrade_history, `levelrail-cli upgrade --attempts`.

## Databases

A minor image change is `levelrail-cli databases set-version`. A Postgres major upgrade is `levelrail-cli databases major-upgrade <name> --version <v>`, which keeps a rollback snapshot (`major-upgrade-rollback`). Confirm a fresh verified backup first.

## Apps

Image updates are deploys: see the rollback runbook for the way back.
