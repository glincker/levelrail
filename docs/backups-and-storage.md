# Backup targets, registry credentials, and app volume backups

This guide covers three related resources for storing backups and managing image registries:

1. **Backup targets**: S3-compatible buckets where database and app volume backups are stored.
2. **Registry credentials**: Authentication for pulling private images from external registries.
3. **Built-in registry**: Levelrail's own `registry:2` container for managing and caching images locally.

All three share the same pattern: an external system that requires a working credential, tested on demand rather than discovered as broken mid-deploy or mid-backup.

## Why a backup target is its own resource

A backup target is not a field on a database or volume. It is a separate resource: a connected S3-compatible bucket that any number of databases and app volumes can reference by ID.

This design means:
- Connect a bucket once in Settings.
- Every database and app volume's backup schedule references it by `target_id`.
- Rotating a leaked access key happens in one place, not N places.

### Credentials are write-only

Credentials never round-trip through the API. For backup targets:

- `access_key_id` and `secret_access_key` are accepted only in create/update request bodies.
- Credentials are written to `internal/secrets` *before* the `backup_targets` row is saved, not after.
- This ensures credentials are never orphaned if the database write fails.
- `GET` and `PUT` responses never echo credential fields back.

Registry credentials follow the same pattern:

- Only the `password` field is accepted in the request body.
- It's written before the `registry_credentials` row, never echoed back.
- When you reference a registry credential in `app.yaml` by name, the deploy can authenticate without the password being stored in the file.

## Testing a connection before it's needed

Both resources expose a `POST .../test` endpoint that validates the credential without performing the actual operation.

### Backup targets

`POST /api/v1/backup-targets/{id}/test` calls `HeadBucket` against the target bucket. No upload, no delete happens.

**Error messages:**

- `401` or `403`: Authentication rejected - check the access key ID and secret access key.
- `404`: Bucket not found - check the bucket name, region, and endpoint.
- Other errors: Could not reach the backup target.

### Registry credentials

`POST /api/v1/registry-credentials/{id}/test` authenticates the stored username/password against the registry host using the same path that a real image pull uses. No image is actually pulled.

**Error messages:**

- Unauthorized response: Authentication rejected by registry - check the username and password.
- Other errors: Could not reach the registry.

### Dashboard and CLI

Test connection before a deploy or scheduled backup fails:
- Dashboard: "Test connection" button on each row of Backup targets and Registry credentials tables.
- CLI: `levelrail-cli backup-targets test <id>` or `levelrail-cli registry-credentials test <id>`

::: tip
Finding a stale credential here means fixing it immediately, not discovering it from a failed 3am backup or a failed deploy mid-pull.
:::

Credentials are never echoed back in error responses.

## How database backups and app volume backups share one concept

A backup target doesn't know or care what it's backing up. Both databases and app volumes reference the same `target_id`.

| Resource | Trigger | History | Schedule |
| --- | --- | --- | --- |
| Database | `POST /api/v1/databases/{name}/backups` | `GET /api/v1/databases/{name}/backups` | `PUT`/`DELETE /api/v1/databases/{name}/backup-schedule` |
| App volume | `POST /api/v1/apps/{name}/volumes/{volume}/backups` | `GET /api/v1/apps/{name}/volumes/{volume}/backups` | `PUT`/`DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule` |

### How backups are triggered

Both trigger endpoints work the same way:

1. Validate `target_id` against a real backup target.
2. Create a `store.BackupHistory` row.
3. Start the dump-and-upload (or volume-tar-and-upload) in a background goroutine.
4. Return `202 Accepted` immediately.

Poll the history endpoint to check whether the backup finished (status starts at `running`).

### How schedules are stored

**Database schedules** are stored alongside the database resource itself (`backup_target_id`, `backup_schedule`, `backup_retain`, `backup_retain_days` columns on `desired_databases`).

**App volume schedules** are stored in their own `service_volume_backups` row. A service can declare multiple named volumes in `app.yaml`, so there's no single "the app" resource to attach one schedule to.

Both use:
- Same cron validation (`internal/cronexpr.Parse`) - checked at request time, so a bad cron string returns `400` immediately.
- Same retention fields.
- Same check that the target must exist before saving.

On the dashboard: a database's schedule form lives on that database's
overview page (`BackupScheduleForm.tsx`, `BackupsSection.tsx`), and an
app volume's lives on that app's Volumes tab (`VolumeBackupScheduleForm.tsx`,
`AppVolumeBackupsSection.tsx`). Backup targets themselves are configured
once, account-wide, at Settings -> Backup targets
(`routes/settings/backup-targets.tsx`), never from a database or app
page directly.

## Instance-wide backup visibility

All previous sections are scoped to one resource at a time (a database's overview page, an app's volumes page). Use `GET /api/v1/backups` to see all backups across all databases and app volumes in one place.

**API details:**

- Newest first, cursor-paginated (`?limit=&before=`).
- Each entry includes `resource_kind` (`database` or `volume`) and identity fields (`database_name` or `service_name`/`volume_name`).
- Allows routing back to the resource's own trigger, download, verify, or delete endpoints.

**Dashboard:** Top-level Backups page (not nested under Settings). Lists all backups with resource, status, size, and timestamps. Download, verify, and delete actions are available here.

**CLI:** `levelrail-cli backups list-all` lists all backups across all resources.

**Note on triggering and restoring:** Both actions stay on their resource-scoped pages because they need context (which target, which confirmation flow) that the aggregated view doesn't provide.

::: tip
Without this page, an operator running multiple databases or apps had no single place to verify whether everything backed up successfully - they had to visit each resource's page in turn.
:::

## Retention: by count and by age, independently

Retention uses two independent dimensions (both optional):

- **`retain` (count):** Keep the newest N successful backups by `started_at`.
- **`retain_days` (age):** Keep only backups less than N days old.

These combine as an OR (delete if either condition is met):
- `retain: 7` and `retain_days: 30` means: keep the 7 newest backups AND never keep anything older than 30 days (whichever is more aggressive at any moment).
- `0` disables that dimension; `0` for both disables pruning entirely.

::: warning
Only succeeded backups are considered for deletion. Running or failed attempts are always kept, regardless of age or count. A failed attempt's logs have diagnostic value and should not be erased by a count limit.
:::

### When pruning runs

Pruning runs after every scheduled backup, not after manually triggered ones:

1. `runScheduled` (databases) or `runScheduledVolume` (app volumes) completes successfully.
2. `PruneBackupHistory` or `PruneServiceVolumeBackupHistory` removes old backups immediately after.
3. Pruning failures are logged, not surfaced as backup failures. The backup already succeeded and is durably recorded, so a cleanup issue afterward should not retroactively fail it.

The dashboard's schedule form defaults a new database's "keep last N
backups" field to 7 and "delete backups older than (days)" to 0 (no age
limit); both are plain number inputs, 0 meaning no limit for that
dimension, documented inline in the form itself.

## Deleting one archived backup on demand

Retention schedules remove old backups automatically. To delete one specific backup right now without affecting the schedule, use an on-demand delete:

**API:**
- Database: `DELETE /api/v1/databases/{name}/backups/{historyId}`
- App volume: `DELETE /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}`

**Dashboard:** Every non-running backup history row has a Delete action behind a confirm dialog.

**CLI:**
- `levelrail-cli backups delete <database> <backup-id>`
- `levelrail-cli app-volume-backups delete <app> <volume> <backup-id>`

No separate `--confirm` flag is needed; naming the exact backup ID is the deliberate signal.

### How deletion works

1. Remove the stored object first (best-effort).
2. Remove the `backup_history` row.

If the storage operation fails, a target is already deleted, or the object is already gone, the failure is logged and skipped. An operator who explicitly asked to delete should not be stuck with it still showing up in history because removing the bytes hit a snag.

::: warning
A backup that is still `running` cannot be deleted (returns `409`). Only `succeeded` or `failed` attempts can be deleted.
:::

**Permission level:** `write:sensitive` (same as triggering a backup or deleting a backup target). Nothing here touches live data, only historical archives.

## Alerting on a missing or failing scheduled backup

A `kind=backup_missing` alert rule watches each database and app volume's backup schedule and fires if no successful backup has occurred within the expected interval (plus a grace period).

**How it works:**

- Reads the same schedule and backup history rows that the scheduler uses (no separate calculation).
- Fires when the last successful backup trails the expected interval by more than `for_duration` (default: 6 hours).
- Surfaces through the usual alert channels: dashboard badge, webhook, Slack, Discord, or other connected notification channel.

This catches:
- Scheduled backups that silently stopped running.
- Repeated failures with no recent success.

See `docs/observability.md` for the full alert-rule table and configuration details.

## The built-in container registry

This is separate from **registry credentials** (which authenticate to external registries like Docker Hub or ghcr.io).

The **built-in registry** is Levelrail's own `registry:2` container, giving you a local build cache and image distribution point without external dependencies.

### Enabling the registry

Call `PUT /api/v1/settings/registry` with `enabled: true` and a `host`.

This:
- Generates a fixed username (`levelrail`) and a random password.
- Returns the password once in the response. It cannot be retrieved later.
- If lost, disable and re-enable to get a new password.

Every later call (toggling host, re-enabling after disable) leaves existing credentials untouched.

**Disabling:** `DELETE /api/v1/settings/registry` clears generated credentials. This is idempotent.

### Viewing repositories and tags

Repositories and tags are read from the running registry container's Docker Registry HTTP API v2 catalog, authenticated server-side (the browsing caller never needs the password).

**Dashboard:** "Existing image" picker when creating an app from an image already pushed to the built-in registry.

**API:** 
- Built-in registry: `GET /api/v1/registry/repositories` and `/api/v1/registry/tags?repository=...`
- External registry credential: `GET /api/v1/registry-credentials/{id}/repositories` and `/api/v1/registry-credentials/{id}/tags?repository=...`

## Integration walkthrough

1. **Connect a backup target**:

   ```bash
   levelrail-cli backup-targets create \
     --name "primary-r2" \
     --provider r2 \
     --endpoint https://<account-id>.r2.cloudflarestorage.com \
     --bucket levelrail-backups \
     --access-key-id AKIA... \
     --secret-access-key ...
   ```

   Response (credentials never echoed back):

   ```json
   {
     "id": "bkt_9f2a1c...",
     "name": "primary-r2",
     "provider": "r2",
     "endpoint": "https://<account-id>.r2.cloudflarestorage.com",
     "bucket": "levelrail-backups",
     "created_at": "2026-09-12T10:00:00Z"
   }
   ```

2. **Prove the credential actually works**:

   ```bash
   levelrail-cli backup-targets test bkt_9f2a1c...
   ```

   Exits non-zero with a specific message ("authentication rejected...",
   "bucket not found...") if it doesn't; silent success (`204`) if it
   does.

3. **Schedule a database's backups against it**:

   ```bash
   levelrail-cli backups schedule set my-postgres \
     --target bkt_9f2a1c... \
     --cron "0 3 * * *" \
     --retain 7 \
     --retain-days 30
   ```

4. **Or schedule an app volume's backups against the same target**:

   ```bash
   levelrail-cli app-volume-backups schedule set my-app uploads \
     --target bkt_9f2a1c... \
     --cron "0 4 * * *" \
     --retain 14
   ```

5. **Connect a registry credential for a private image pull**, then
   reference it from `app.yaml`:

   ```bash
   levelrail-cli registry-credentials create \
     --name ghcr-private \
     --registry-host ghcr.io \
     --username you \
     --password ghp_...
   ```

   ```yaml
   services:
     web:
       build:
         type: image
         image: ghcr.io/you/private-app:latest
         registryCredential: ghcr-private
   ```

6. **Test it before the next deploy relies on it**:

   ```bash
   levelrail-cli registry-credentials test <id>
   ```

7. **Enable the built-in registry** if you'd rather push there than
   manage an external one:

   ```bash
   levelrail-cli registry enable --host registry.internal.example
   ```

   Output includes the password once:

   ```
   enabled:         true
   host:            registry.internal.example
   username:        levelrail
   has_credentials: true
   status:          running

   password:        <printed once, never again>

   This password is shown once and never again. Store it now, e.g.:
     docker login registry.internal.example -u levelrail
   ```

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/backup-targets` | `read` |
| `POST` | `/api/v1/backup-targets` | `write:sensitive` |
| `GET` | `/api/v1/backup-targets/{id}` | `read` |
| `PUT` | `/api/v1/backup-targets/{id}` | `write:sensitive` |
| `DELETE` | `/api/v1/backup-targets/{id}` | `write:sensitive` |
| `POST` | `/api/v1/backup-targets/{id}/test` | `write:sensitive` |
| `GET` | `/api/v1/registry-credentials` | `read` |
| `POST` | `/api/v1/registry-credentials` | `write:sensitive` |
| `GET` | `/api/v1/registry-credentials/{id}` | `read` |
| `PUT` | `/api/v1/registry-credentials/{id}` | `write:sensitive` |
| `DELETE` | `/api/v1/registry-credentials/{id}` | `write:sensitive` |
| `POST` | `/api/v1/registry-credentials/{id}/test` | `write:sensitive` |
| `GET` | `/api/v1/registry-credentials/{id}/repositories` | `read:sensitive` |
| `GET` | `/api/v1/registry-credentials/{id}/tags?repository=...` | `read:sensitive` |
| `GET` | `/api/v1/settings/registry` | `read` |
| `PUT` | `/api/v1/settings/registry` | `root` |
| `DELETE` | `/api/v1/settings/registry` | `root` |
| `GET` | `/api/v1/registry/repositories` | `read` |
| `GET` | `/api/v1/registry/tags?repository=...` | `read` |
| `GET` | `/api/v1/backups?limit=&before=` | `read` |
| `POST` | `/api/v1/databases/{name}/backups` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/backups?limit=&before=` | `read` |
| `DELETE` | `/api/v1/databases/{name}/backups/{historyId}` | `write:sensitive` |
| `PUT` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `DELETE` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `POST` | `/api/v1/apps/{name}/volumes/{volume}/backups` | `write:sensitive` |
| `GET` | `/api/v1/apps/{name}/volumes/{volume}/backups?limit=&before=` | `read` |
| `DELETE` | `/api/v1/apps/{name}/volumes/{volume}/backups/{historyId}` | `write:sensitive` |
| `GET` | `/api/v1/apps/{name}/volumes/{volume}/backup-schedule` | `read` |
| `PUT` | `/api/v1/apps/{name}/volumes/{volume}/backup-schedule` | `write:sensitive` |
| `DELETE` | `/api/v1/apps/{name}/volumes/{volume}/backup-schedule` | `write:sensitive` |

Every write-tier endpoint above returns `501` if the control plane has
no master key configured (`internal/secrets.Manager` never wired up):
none of these resources can hold a working credential without one.

## CLI

```bash
levelrail-cli backup-targets list [flags]
levelrail-cli backup-targets get <id> [flags]
levelrail-cli backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [--endpoint URL] [--region REGION]
levelrail-cli backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [--access-key-id ID --secret-access-key KEY] [--endpoint URL] [--region REGION]
levelrail-cli backup-targets delete <id> [flags]
levelrail-cli backup-targets test <id> [flags]

levelrail-cli registry-credentials list [flags]
levelrail-cli registry-credentials get <id> [flags]
levelrail-cli registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [--expires-at RFC3339]
levelrail-cli registry-credentials update <id> --name NAME --registry-host HOST --username USER [--password PASS] [--expires-at RFC3339]
levelrail-cli registry-credentials delete <id> [flags]
levelrail-cli registry-credentials test <id> [flags]
levelrail-cli registry-credentials repositories <id> [flags]
levelrail-cli registry-credentials tags <id> <repository> [flags]

levelrail-cli registry status [flags]
levelrail-cli registry enable --host HOST [flags]
levelrail-cli registry disable [flags]
levelrail-cli registry repositories [flags]
levelrail-cli registry tags --repository NAME [flags]

levelrail-cli backups schedule set <database> --target ID --cron EXPR [--retain N] [--retain-days N]
levelrail-cli backups schedule clear <database> [flags]
levelrail-cli backups delete <database> <backup-id> [flags]
levelrail-cli backups list-all [--limit N] [--before RFC3339] [flags]

levelrail-cli app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [--retain N] [--retain-days N]
levelrail-cli app-volume-backups schedule clear <app> <volume> [flags]
levelrail-cli app-volume-backups delete <app> <volume> <backup-id> [flags]
```

`--provider` accepts `aws`, `r2`, or `custom`. `--endpoint` is required
for `r2` and `custom` (AWS S3 resolves its own default endpoint per
region, so `aws` must omit it).

## Not built yet (deliberate follow-ups)

**No secret deletion**
- Deleting a backup target or registry credential leaves its secrets behind in the store, unreferenced and unreachable through the API but not erased at rest.
- Adding secret deletion requires its own change to `internal/secrets`, not bundled into either resource.

**No registry credential expiry enforcement**
- `expires_at` on a registry credential is informational only (drives a health/expiring_soon/expired badge, like TLS certificates).
- Nothing blocks a deploy from using an already-expired credential.
- No automatic rotation or renewal.

**No cross-provider bucket validation beyond `HeadBucket`**
- The test endpoint proves the bucket exists and can be read.
- It does not verify write/delete permission, which backups and retention pruning need.

**No UI or CLI surface for retention-pruning failures**
- Pruning or per-object delete failures after a successful backup are logged server-side only.
- No dashboard alert for this specific case.
- The backup itself silently stopping or failing is caught by the `kind=backup_missing` alert rule (see "Alerting on a missing or failing scheduled backup").
