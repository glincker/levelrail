---
title: "Archives"
description: "Move cold audit events, deploy attempts and pipeline runs into compressed bundles instead of deleting them."
---

The control plane keeps audit events, deploy attempts and pipeline runs in its database. Archives move the old ones into zstd compressed NDJSON bundles under `<data dir>/archives`, so the live database stays small and nothing is lost. A bundle can be browsed, verified by restore, downloaded and put back.

## Retention tiers

| Tier | What it holds | Controlled by |
| --- | --- | --- |
| Hot | Rows in the live database | `audit_hot_days`, `deploy_hot_days`, `pipeline_hot_days` (default 30 each) |
| Archive | Rows in bundles | `keep_days` (default 365, 0 keeps forever) |
| Delete | Expired bundles | Removed after each scheduled run |

The audit log also has its own purge (`APP_AUDIT_LOG_RETENTION_DAYS`, default 90). Keep the audit hot days below it, or archiving never sees those rows.

## What is archived

- **Audit events**: every row older than the hot days.
- **Deploy attempts**: finished attempts older than the hot days. The two newest successful attempts of each service are never archived (they are rollback targets), and neither is an attempt a health probe still points at.
- **Pipeline runs**: succeeded, failed and cancelled runs older than the hot days, with their jobs and steps. Run logs, artifacts and approvals of an archived run are deleted with it.

Bundles hold no secret values. Deploy attempts record only the key names of secret environment variables, never their values.

## Safety

A bundle is written, fsynced, then re-read and compared (checksum and per-table row counts) before any live row is removed. If the re-read fails, the live rows stay. Restore and verify check the recorded checksum first, so a modified bundle is refused.

Each run is bounded: at most 50,000 rows (5,000 pipeline runs) per bundle and 20 bundles per kind per run.

## Verify by restore

Verify checks the checksum and row counts, then restores the bundle into a scratch database built from the current schema and compares the counts. The live database is untouched. The result is stored on the bundle and shown in the list.

## Restore

Restore inserts the bundle's rows back into the live tables. Rows already present are left alone, so it is safe to run twice. Pipeline runs whose pipeline no longer exists are skipped. Restored rows age out again on the next archive run.

## Full control plane dump

The whole database (secrets stay encrypted with the master key, which is never included) is covered by [control plane backup](/control-plane-backup): scheduled snapshots, off-box encrypted copies and a restore drill that restores into a scratch database. Archives are for cold history. The dump is for disaster recovery.

## Use it

<Tabs>
<Tab title="Dashboard">

Settings, Control plane backup, Archives card: archive now, browse a bundle's rows, verify, restore, download, delete, and edit the retention tiers.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli archive ls
levelrail-cli archive create --kind audit --older-than 30
levelrail-cli archive verify <id>
levelrail-cli archive restore <id>
levelrail-cli archive download <id> --out bundle.ndjson.zst
levelrail-cli archive policy --enabled=true --keep-days 365
```

`archive create` without `--kind` runs the whole policy. `verify` exits 1 when a check fails.

</Tab>
<Tab title="API">

| Method | Path |
| --- | --- |
| `GET`, `POST` | `/api/v1/archives` |
| `GET`, `PUT` | `/api/v1/archives/policy` |
| `GET`, `DELETE` | `/api/v1/archives/{id}` |
| `GET` | `/api/v1/archives/{id}/rows?table=&offset=&limit=` |
| `GET` | `/api/v1/archives/{id}/download` |
| `POST` | `/api/v1/archives/{id}/restore` |
| `POST` | `/api/v1/archives/{id}/verify` |

All need a root-scoped token. The MCP tool `list_archives` reads the list and policy.

</Tab>
</Tabs>

Scheduled archiving is off until you turn it on. It checks hourly.
