# ADR 019: Storage destinations and log archive

Status: Accepted

Date: 2026-09-24

## Context

Backups already had connected S3-compatible buckets (`backup_targets`, credentials in `internal/secrets`). Log archive, and later build cache export, need the same thing: a bucket, credentials, and a way to prove they work. Node-local logs (ADR 009) were the only copy, so a node loss lost history.

## Decision

**A storage destination is a backup target plus an options row.** `storage_destination_options` (migration 0127) adds the provider preset, path-style flag and account id per target. Existing targets keep working unchanged and infer their preset from `provider`. Credentials keep the existing `backup-target/<id>` secrets key, so one credential is used by backups and log archive alike.

**Provider presets are code, not schema.** aws, r2, b2, minio, wasabi and custom resolve an endpoint from region or account id in `internal/objectstore`. The `backup_targets.provider` CHECK constraint (aws, r2, custom) is untouched; b2, minio and wasabi are stored as `custom` with the preset recorded in the options row.

**One S3 client for the new work, SSRF-guarded.** `internal/objectstore` builds the client on `netguard.NewClient()`, so a destination cannot be pointed at internal addresses (private MinIO needs the existing opt-in env var). Connection tests do a put, get and delete of a random `.probe/` object and classify failures into stable reason codes.

**Log archive is pull-based and watermark-driven.** A per-policy watermark (migration 0127) advances only after every object in an hour window is uploaded. Object keys use the chunk start only, so retries overwrite rather than duplicate. Logs are spooled to a temp file before upload because the telemetry database has a single connection. Manual dumps and scheduled runs share one slot for backpressure. Run history lives in `log_archive_runs` (migration 0128).

**Staleness is an alert kind.** `log_archive_stale` reads policy health (last success, last error) instead of adding a second notification path.

## Rejected alternatives

- **Rebuild `backup_targets` with a wider provider CHECK.** SQLite needs a table rebuild with foreign keys from five tables. Risk to backup history for no gain over an options row.
- **A separate `storage_destinations` table with its own credentials.** Duplicates the connect flow and forces users to enter the same bucket twice for backups and logs.
- **Streaming every log line to the bucket.** Adds constant idle cost and network use, the opposite of the node-local design. Scheduled pull keeps idle near zero.
- **Reusing `backup.S3Uploader`.** It builds an unguarded client and is shaped around multipart streaming of dumps. Left alone so backup behavior does not change; unifying the two clients is a follow-up.
- **A search index over archived objects.** Out of scope: objects are plain gzip NDJSON so any external tool can query them.

## Consequences

Backups still use their own S3 client (no SSRF guard, HeadBucket test). Build artifact and cache export can reuse `objectstore.Client` and the same destinations, but is not part of this change.
