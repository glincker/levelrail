---
title: "Use Local MinIO (same machine) with Levelrail: S3 backups"
sidebarTitle: "Local MinIO (same machine)"
description: "Send Levelrail database dumps and app volume archives to your Local MinIO (same machine) bucket through its S3 compatible API. Same machine only."
---

Send Levelrail database dumps and app volume archives to your Local MinIO (same machine) bucket through its S3 compatible API. Same machine only.

This page covers how to store database and app volume backups on Local MinIO (same machine).

## What you get

- A quick way to exercise scheduled backups and restores without a cloud account.
- The same Test connection, schedule and retention behavior as an off-site target.
- A clear warning in the dialog that backups on the same machine do not survive that machine.

## Set up

1. Prepare the credential. The access key and secret key of a MinIO running on the control plane host.
2. In the dashboard, open **Settings > Backup targets** (`/settings/backup-targets`), then add a target and pick the Local MinIO (same machine) tile.
3. Run Test connection on the new target before you schedule anything against it.

From the CLI:

```bash
levelrail-cli backup-targets create --name local-backups --provider custom --endpoint http://127.0.0.1:9000 --region us-east-1 --bucket levelrail-backups --access-key-id <KEY_ID> --secret-access-key <SECRET>
levelrail-cli backup-targets test <target-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Reach the bucket | Needed by Test connection, which calls HeadBucket. |
| Write objects | Needed to upload each dump or volume archive. |
| Read objects | Needed for restore and backup verification. |
| Delete objects | Needed for retention pruning. |

## Endpoint and format

Provider type `custom`. Endpoint: `http://127.0.0.1:9000`. Default region: `us-east-1`.

## Security notes

- The access key id and secret are written to the secrets store before the target record is saved, so a failed save never orphans a credential.
- They are write-only: no response echoes them. Rotate by running `backup-targets update` with a new pair.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Backups kept on the machine they protect are lost with it. Use this for trials and add an off-site target for real data.
- Test connection proves the bucket exists and answers, not that the key can write or delete.
- Every backup write returns 501 until the control plane has a master key configured.

## Troubleshooting

**Test connection says authentication rejected.** The API answered 401 or 403. Re-enter the access key id and secret, and confirm the key is allowed on this bucket.

**Test connection says bucket not found.** The API answered 404. Check the bucket name, the region and the endpoint host together: this preset points at a MinIO on 127.0.0.1:9000, so a not-found result usually means that process is not running or the bucket was not created.

**A manual backup fails with 507.** The control plane data directory has less free space than APP_MIN_BACKUP_DISK_MB (default 256). Free space there and retry.

## FAQ

**Is this safe for production backups?**

No. The preset is flagged as same-machine in the dialog because a disk or host failure takes the data and its backups together.

**How do I move to an off-site target?**

Create a second target, point the schedule at it with `backups schedule set`, and keep the local one only for as long as you need it.

## Related

- [MinIO](/integrations/backup-destinations/minio): Backup destinations
- [Cloudflare R2](/integrations/backup-destinations/r2): Backup destinations
- [All backup destinations](/integrations/backup-destinations)
- [Local MinIO (same machine) documentation](https://min.io/docs/), maintained by the vendor
