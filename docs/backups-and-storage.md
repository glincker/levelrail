# Backup targets, registry credentials, and app volume backups

Three related but separate resources: `internal/api/backup_targets.go`
and `internal/backup` (where a backup gets stored), `internal/api/
registry_credentials.go` (how a deploy pulls a private image), and
`internal/api/registry_settings.go` plus `internal/api/registry_catalog.go`
(Levelrail's own built-in registry). They share one idea, an
S3-compatible bucket or a Docker registry is an external system the
control plane needs a working credential for, tested once on demand
rather than discovered as broken mid-deploy or mid-backup.

## Why a backup target is its own resource

A backup target is not a field on a database or a volume. It is a
connected S3-compatible bucket (`store.BackupTarget`: name, provider,
endpoint, region, bucket, no credential fields) that any number of
databases and any number of app volumes can point at by ID. That
indirection is the entire reason it exists as a separate resource: you
connect a bucket once in Settings, then every database's backup
schedule and every app volume's backup schedule references it by
`target_id`, so rotating a leaked access key happens in one place
instead of N.

Credentials never round-trip through the API. `access_key_id` and
`secret_access_key` are accepted only in the create/update request body
and are written to `internal/secrets` (keyed by
`store.BackupTargetSecretsKey(id)`) before the `backup_targets` row is
saved, not after: if the store write then failed, the result would be an
orphaned, harmless secret rather than a target that looks connected but
has no working credential behind it. `GET`/`PUT` responses never echo
the credential fields back.

Registry credentials follow the identical shape for a different secret:
one `password` field, written before the `registry_credentials` row,
never echoed back. The reason a private image pull needs this at all:
`build.registryCredential` in `app.yaml` names a stored credential by
its `name`, so a deploy that pulls `ghcr.io/you/private-app` can
authenticate without the password living in `app.yaml` itself.

## Testing a connection before it's needed

Both resources expose a `POST .../test` endpoint that proves the stored
credential actually works, right now, without doing the real thing it
exists for:

- `POST /api/v1/backup-targets/{id}/test` calls `HeadBucket` against the
  target's configured bucket (`internal/backup.TargetTester` /
  `S3Tester`, wired via the AWS S3 SDK). No upload, no delete.
- `POST /api/v1/registry-credentials/{id}/test` authenticates the stored
  username/password against `RegistryHost` through the same Docker
  daemon path (`ensureImage`) a real image pull uses. No image is
  pulled.

Both classify their failure into one of two buckets and never echo the
credential back in the error:

- Backup target: `401`/`403` from the storage endpoint maps to
  "authentication rejected, check the access key id and secret access
  key"; `404` maps to "bucket not found, check the bucket name, region,
  and endpoint"; anything else is a generic "could not reach backup
  target" with the underlying error appended.
- Registry credential: an unauthorized response maps to "authentication
  rejected by registry, check the username and password"; anything else
  is "could not reach registry" with the underlying error appended.

The dashboard exposes this as a "Test connection" button on each row of
the Backup targets and Registry credentials tables
(`BackupTargetTable.tsx`, `RegistryCredentialTable.tsx`), and the CLI as
`backup-targets test <id>` / `registry-credentials test <id>`. Catching a
stale key here means finding out immediately, not from a failed 3am
scheduled backup or a failed deploy mid-pull.

## How database backups and app volume backups share one concept

A backup target doesn't know or care what it's backing up. The same
`target_id` a database's backup schedule references is the same
`target_id` an app volume's backup schedule references:

| Resource | Trigger | History | Schedule |
| --- | --- | --- | --- |
| Database | `POST /api/v1/databases/{name}/backups` | `GET /api/v1/databases/{name}/backups` | `PUT`/`DELETE /api/v1/databases/{name}/backup-schedule` |
| App volume | `POST /api/v1/apps/{name}/volumes/{volume}/backups` | `GET /api/v1/apps/{name}/volumes/{volume}/backups` | `PUT`/`DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule` |

Both trigger endpoints do the same thing: validate `target_id` against a
real backup target, mint a `store.BackupHistory` row, start the actual
dump-and-upload (or volume-tar-and-upload) in a background goroutine
against `context.Background()` (not the request context, which is
cancelled the instant the handler returns), and respond `202 Accepted`
immediately. The history row's `status` starts at `running`; polling the
list endpoint is how a caller learns whether it finished.

A database's schedule config rides along on the database resource
itself (`backup_target_id`, `backup_schedule`, `backup_retain`,
`backup_retain_days` columns on `desired_databases`). An app volume's
schedule is its own row in `service_volume_backups`, because a service
can declare any number of named volumes in `app.yaml`, so there's no
single "the app" resource to attach one schedule to; that's why the
volume path has a dedicated `GET .../backup-schedule` that a database
doesn't need. Functionally identical otherwise: same cron validation
(`internal/cronexpr.Parse`, checked synchronously so a bad cron string
is a `400` immediately, not a silent skip on the next scheduler tick),
same retention fields, same target-must-exist check before the schedule
is ever saved.

On the dashboard: a database's schedule form lives on that database's
overview page (`BackupScheduleForm.tsx`, `BackupsSection.tsx`), and an
app volume's lives on that app's Volumes tab (`VolumeBackupScheduleForm.tsx`,
`AppVolumeBackupsSection.tsx`). Backup targets themselves are configured
once, account-wide, at Settings -> Backup targets
(`routes/settings/backup-targets.tsx`), never from a database or app
page directly.

## Retention: by count and by age, independently

Both dimensions are optional and combine as an OR, not an AND: a
succeeded backup is deleted if it falls outside the newest `retain`
backups by `started_at`, or if it's older than `retain_days`, whichever
condition it hits first. Setting `retain: 7` and `retain_days: 30` keeps
at most 7 backups and never keeps one older than 30 days, whichever is
more aggressive at any given moment. `0` disables that dimension; `0`
for both disables pruning entirely.

Retention only ever considers `status = succeeded` rows. A running or
failed attempt is left alone regardless of age or count: an operator who
sets `retain: 7` wants "7 successful backups", not "7 attempts of any
kind", and a failed attempt's logs have diagnostic value a count limit
was never meant to erase.

Pruning runs after every scheduled backup (`internal/backup.Scheduler`),
not after a manually triggered one: `runScheduled` (databases) and
`runScheduledVolume` (app volumes) both call
`PruneBackupHistory`/`PruneServiceVolumeBackupHistory` right after a
successful run, then best-effort delete the pruned rows' objects from
the bucket. A pruning failure is logged, not surfaced as the backup
itself failing: the backup already succeeded and is already durably
recorded, so a cleanup hiccup afterward shouldn't retroactively mark it
red.

The dashboard's schedule form defaults a new database's "keep last N
backups" field to 7 and "delete backups older than (days)" to 0 (no age
limit); both are plain number inputs, 0 meaning no limit for that
dimension, documented inline in the form itself.

## The built-in container registry

Separate from registry credentials. Registry credentials store a
password for a registry you already run somewhere else (Docker Hub,
ghcr.io, a private Harbor instance). The built-in registry is
Levelrail's own `registry:2` container (`internal/reconcile/registry`),
enabled per control plane, so a multi-node deployment gets a build
cache and image distribution point without signing up for anything
external first.

Enabling it for the first time (`PUT /api/v1/settings/registry` with
`enabled: true` and a `host`) generates a fixed username (`levelrail`)
and a random password, and returns that password exactly once, in that
call's own response. There is no other channel to retrieve it: losing it
means disabling and re-enabling to get a fresh one. Every later call
that doesn't need a fresh credential (toggling `host`, or re-enabling
after a disable that already cleared the old one) leaves whatever
credential currently exists untouched. `DELETE /api/v1/settings/registry`
disables the registry and clears its generated credentials via
`internal/secrets.Manager.DeleteAll`; it's idempotent, disabling an
already-disabled registry is not an error.

Repositories and tags are read straight from the running registry
container's own Docker Registry HTTP API v2 catalog
(`_catalog`, `<name>/tags/list`) over loopback, authenticated
server-side with the generated password so the browsing caller never
needs it. This backs the "existing image" picker when creating an app
from an image already pushed to the built-in registry
(`RegistryImagePicker.tsx`). The identical catalog client also powers
browsing an external registry credential's own catalog
(`GET /api/v1/registry-credentials/{id}/repositories` and `.../tags`),
just pointed at that credential's `registry_host` instead of the
built-in registry's loopback address.

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
| `POST` | `/api/v1/databases/{name}/backups` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/backups?limit=&before=` | `read` |
| `PUT` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `DELETE` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `POST` | `/api/v1/apps/{name}/volumes/{volume}/backups` | `write:sensitive` |
| `GET` | `/api/v1/apps/{name}/volumes/{volume}/backups?limit=&before=` | `read` |
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

levelrail-cli app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [--retain N] [--retain-days N]
levelrail-cli app-volume-backups schedule clear <app> <volume> [flags]
```

`--provider` accepts `aws`, `r2`, or `custom`. `--endpoint` is required
for `r2` and `custom` (AWS S3 resolves its own default endpoint per
region, so `aws` must omit it).

## Not built yet (deliberate follow-ups)

- **No secret deletion.** `internal/secrets.Manager` has no
  delete/revoke operation today, only `SetValue`/`Resolve`/`Exists`, so
  deleting a backup target or a registry credential leaves its
  access key, secret key, or password behind in the secrets store,
  unreferenced and unreachable through the API but not actually erased
  at rest. Both handlers' own doc comments flag this as a known,
  deliberate gap rather than a bug: adding secret deletion is its own
  change to `internal/secrets`, not bundled into either resource.
- **No registry credential expiry enforcement.** `expires_at` on a
  registry credential is purely informational: it drives the same
  three-state healthy/expiring_soon/expired badge TLS certificates get
  (`alerting.CertExpiryStatus`, 14-day default warning window), but
  nothing blocks a deploy from using an already-expired credential, and
  there's no automatic rotation or renewal.
- **No cross-provider bucket validation beyond `HeadBucket`.** The test
  endpoint proves the bucket exists and the credential can read it, not
  that the credential also has write/delete permission, which is what a
  real backup and its later retention pruning both need.
- **No UI or CLI surface for orphaned scheduled-backup failures beyond
  the log line.** A pruning failure or a per-object delete failure after
  a successful scheduled backup is logged server-side
  (`internal/backup.Scheduler`) and nothing else; there's no dashboard
  alert distinct from the backup's own success/failure status.
