---
title: "Object storage"
description: Connect S3-compatible buckets for log archive, build cache, and backups.
---

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="24" height="24" /></span><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="24" height="24" /></span></div>

A storage destination is an S3-compatible bucket the control plane can write to. Log archive and the remote build cache use it, and database and volume backups use the same destinations: a destination is a backup target with a few extra options, so anything you connect appears in both **Settings, Storage destinations** and **Settings, Backup targets**.

Supported providers: AWS S3, Cloudflare R2, Backblaze B2, MinIO, Wasabi, and any custom S3-compatible endpoint.

## Connect a destination

<Tabs>
<Tab title="Dashboard">

Go to **Settings, Storage destinations, Add destination**. Pick a provider, fill in the fields it asks for, and leave "Test the connection before saving" on.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli storage providers
levelrail-cli storage add --name logs --provider r2 --account-id ACCOUNT --bucket my-logs \
  --access-key-id KEY --secret-access-key SECRET
levelrail-cli storage test <id>
levelrail-cli storage list
levelrail-cli storage delete <id>
```

`storage add` runs the connection test before saving. Pass `--skip-verify` to save without it.

</Tab>
</Tabs>

Credentials are encrypted with the control plane master key, never logged, and never returned by the API. Connecting a destination needs a master key to be configured.

| Provider (`--provider`) | Extra fields | Endpoint |
| --- | --- | --- |
| `aws` | region | resolved by the AWS SDK |
| `r2` | account id (`--account-id`) | `https://<account-id>.r2.cloudflarestorage.com` |
| `b2` | region such as `us-west-004` | `https://s3.<region>.backblazeb2.com` |
| `wasabi` | region | `https://s3.<region>.wasabisys.com` |
| `minio` | endpoint | the URL you give |
| `custom` | endpoint | the URL you give |

Everything except AWS uses path-style addressing by default. Pass `--virtual-hosted` (or tick the box in the dialog) for a provider that needs the bucket in the hostname.

### What the connection test does

The test writes a small random object under `.probe/`, reads it back, compares it, and deletes it. Each step is reported, and a failure carries a stable reason: `invalid_credentials`, `access_denied` (valid key, missing permission), `bucket_not_found`, `region_mismatch`, `endpoint_blocked`, `tls_error`, or `unreachable`. The key needs permission to put, get, delete, and list objects in the bucket.

### Private endpoints

Outbound connections to a destination go through the same SSRF guard as webhooks and log drains: loopback, private, link-local, and other internal addresses are refused after DNS resolution. A MinIO on a private network needs `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true` on the control plane.

## Provider walkthroughs

<Tabs>
<Tab title="Cloudflare R2">

<Steps>
<Step title="Create a bucket">

In the Cloudflare dashboard, create a bucket under R2 (for example `app-logs`).

</Step>
<Step title="Create an API token">

Create an R2 API token with **Object Read and Write** scoped to that bucket. Copy the access key id, the secret access key, and your account id (shown on the R2 overview page).

</Step>
<Step title="Add the destination">

In Levelrail, add a destination with provider **Cloudflare R2**, the account id, the bucket, and the key pair. The endpoint is filled in for you and the region is `auto`.

</Step>
<Step title="Run the connection test">

Every step should be green.

</Step>
<Step title="Start archiving">

Under an app's **Logs, Archive** tab (or Settings, Storage destinations for every app), choose the destination and click **Start archiving**.

</Step>
</Steps>

</Tab>
<Tab title="AWS S3">

<Steps>
<Step title="Create a private bucket">

Create a bucket in the region you want and keep it private.

</Step>
<Step title="Create an IAM user">

Create an IAM user with an access key and attach a policy limited to the bucket:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": ["s3:ListBucket"], "Resource": "arn:aws:s3:::my-logs" },
    { "Effect": "Allow", "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"], "Resource": "arn:aws:s3:::my-logs/*" }
  ]
}
```

</Step>
<Step title="Add the destination">

Add a destination with provider **AWS S3**, the bucket region, the bucket name, and the key pair.

</Step>
<Step title="Test, then start archiving">

Run the connection test, then start archiving.

</Step>
</Steps>

</Tab>
</Tabs>

## Log archive

Log archive ships node-local container logs to a destination as gzip-compressed NDJSON, on a schedule or on demand. See [Log archive](/log-archive) for what gets archived, retention, dump, and how to retrieve archived objects.

## Build cache

BuildKit can keep its layer cache in a storage destination, so a rebuild skips unchanged steps even after the build host's local cache is gone. It uses BuildKit's `s3` cache backend, so it works with every provider above.

Dashboard: an app's **Settings, Deploy settings, Build cache** card, or **Settings, Storage destinations, Build cache for all apps** for a default that every app without its own setting inherits. Pick a destination and an export mode, and save.

```bash
levelrail-cli apps build-cache set web --target bkt_1 --mode max
levelrail-cli apps build-cache set --global --target bkt_1
levelrail-cli apps build-cache show web
levelrail-cli apps build-cache clear web
levelrail-cli apps build-cache remove web
```

- Per-app prefix: layers live under `build-cache/<app>/` (change the root with `APP_BUILD_CACHE_PREFIX`), so apps never share cache and one app can be cleared without touching another. An app can opt out of the global default by saving a setting with the cache disabled (`--disable`).
- Mode: `max` (the default) caches every layer and gives the fastest rebuilds. `min` caches only the final image layers and keeps the bucket smaller.
- Credentials: they come from the destination's encrypted secrets, are passed to BuildKit for the one build, and are never logged, stored in build logs, or returned by the API.
- Fails open: if the cache cannot be read or written (bad credentials, bucket gone, network), the build carries on without it. The reason is written to the build log as a warning, shown on the deploy attempt (`cache_warning`), and kept on the setting as the last build result. A build that genuinely fails is still a failed build.

### More behavior and limits

<AccordionGroup>
<Accordion title="Private endpoints">

BuildKit dials the bucket itself, so the SSRF guard is applied to a custom endpoint before each build. An endpoint that resolves to an internal address is skipped with a warning unless `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true`.

</Accordion>
<Accordion title="Remote build nodes">

A build dispatched to a dedicated [build node](/build-node-routing) does not receive bucket credentials, so it runs without the s3 cache and records a warning. Local builds on the control plane use it. Railpack builds do not use the s3 cache yet; Dockerfile builds do.

</Accordion>
<Accordion title="Hygiene and clearing the cache">

The app card and `build-cache show` list the app's prefix (bounded to `APP_BUILD_CACHE_STATS_MAX_OBJECTS`, default 2000) for object count, size, and last export time, plus the last build outcome. **Clear cache** deletes the prefix in pages, at most `APP_BUILD_CACHE_CLEAR_MAX_OBJECTS` (default 5000) objects per call. When it reports more remaining, run it again. Removing a setting stops using the cache but leaves the objects, so clear first if you want the space back, or add a bucket lifecycle rule on `build-cache/`.

</Accordion>
</AccordionGroup>

## App buckets

A bucket is a private folder (a key prefix) inside a connected destination, bound to an app. Create one from the app's Integrations page (**Storage buckets**, **New bucket**) or with `levelrail-cli storage buckets create --name uploads --destination <id> --app web`.

- **Env vars.** At the next container start the app gets `S3_ENDPOINT`, `S3_BUCKET`, `S3_PREFIX`, `S3_REGION`, `S3_ACCESS_KEY_ID` and `S3_SECRET_ACCESS_KEY`. Give a second bucket another prefix with `--env-prefix MEDIA` to get `MEDIA_*`. Binding or unbinding a bucket restarts a running app automatically so it picks up the change. The app's **Environment** card lists these variables (and database links) as read-only **Connected resources** with secrets masked, and `env pull` includes them, masked unless you reveal after step-up.
- **Files.** **Browse files** lists (virtualized), uploads, downloads and deletes. Console uploads are capped by `APP_API_MAX_BODY_BYTES` (default 16 MiB); use `storage buckets presign <id> <key> --method PUT` for bigger files.
- **Presigned links.** Time-limited GET or PUT URLs, up to 7 days, from the console (copy link), CLI or `POST /api/v1/storage/buckets/{id}/presign`.
- **Size limit.** Enforced for console, CLI and presign issuing (a PUT presign for a full bucket fails with `413` and says to delete objects or raise `quota_bytes`), not for the app writing straight to S3 with its env credentials.
- **Expire after N days.** A rule that deletes older files, run hourly and on demand (**Run expiry now**, `storage buckets sweep`).
- **Public read.** The toggle serves files without sign-in at `/api/v1/public/buckets/<id>/<key>`, sandboxed with a restrictive content security policy. Everything else is private. The route is rate limited per client IP (`APP_PUBLIC_BUCKET_RATE_PER_MIN`, default 120, answers `429` with `Retry-After`), refuses objects over `APP_PUBLIC_BUCKET_MAX_BYTES` (default 100 MiB, `413`), and always serves HTML, SVG, XML and JavaScript as a download instead of rendering them.

<Warning>
Prefix scoping is not a security boundary. The injected credentials are the destination's own, so an app can read, write and delete every object in the whole provider bucket, including other apps' prefixes. The prefix only scopes where this feature's console, CLI, quota, lifecycle and public route read and write. There is no embedded S3 gateway that issues per-bucket keys. For apps you do not fully trust, create a dedicated destination (its own provider bucket and its own scoped key) and bind only that.
</Warning>

## Alerting

Create a `log_archive_stale` alert rule (dashboard: Alerts, or `levelrail-cli apps alerts create <app> --kind log_archive_stale`). It fires when any enabled policy's last run failed, or when a policy has gone longer than the rule's `for_duration` without a success (default: three intervals, at least two hours). It notifies through the same channels as every other rule.

## API and MCP

REST routes live under `/api/v1/storage` (including `/api/v1/storage/buckets`), `/api/v1/log-archive`, and `/api/v1/build-cache` (see the [API reference](/api-reference)). Reads need the `read` ability, policy changes and dumps need `write`, and anything that handles credentials needs `write:sensitive`. The MCP server exposes read and control tools (`list_storage_destinations`, `test_storage_destination`, `list_log_archive_policies`, `set_log_archive_policy`, `start_log_archive_dump`, `list_log_archive_runs`, `list_archived_logs`, `get_build_cache`, `set_build_cache`) but deliberately has no tool that accepts bucket credentials.

## Next steps

<CardGroup cols={2}>
<Card title="Log archive" href="/log-archive">

Ship container logs to a destination on a schedule.

</Card>
<Card title="Backups and storage" href="/backups-and-storage">

Use a destination for database and volume backups.

</Card>
</CardGroup>
