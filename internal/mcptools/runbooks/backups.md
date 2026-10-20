# Runbook: backups

A backup that was never restored is a hope, not a backup.

## Check protection

1. backups_health: per database and volume state, last backup, last verified restore, encryption and target warnings. Unhealthy first.
2. list_backup_targets and test_backup_target_connection: is the destination reachable and writable.
3. get_control_plane_dr_status: control plane backup freshness and off box copy.

## Prove it

Restore drills restore the newest backup into a scratch target and verify it.

CLI: `levelrail-cli backups health`, `levelrail-cli backups drill run --backup <id>`, `levelrail-cli backups drill list`.

## Schedule

`levelrail-cli backups schedule set <database> ...` for databases, `levelrail-cli app-volume-backups schedule set <app> <volume> ...` for volumes. Retention is per volume policy.

## Restore

Restores overwrite data. Always name the exact backup id and the target, and get explicit confirmation. Restoring into a different app (`--target-app`) is the safe first step.
