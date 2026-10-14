---
title: "Use Hetzner Object Storage with Levelrail: S3 backups"
sidebarTitle: "Hetzner Object Storage"
description: "Send Levelrail database dumps and app volume archives to your Hetzner Object Storage bucket through its S3 compatible API."
---

Send Levelrail database dumps and app volume archives to your Hetzner Object Storage bucket through its S3 compatible API.

This page covers how to store database and app volume backups on Hetzner Object Storage.

## What you get

- Scheduled and manual backups of databases and app volumes written to your Hetzner Object Storage bucket.
- A Test connection button that checks the credential before a 3am backup depends on it.
- Retention by count and by age, applied by the control plane after each run.

## Set up

1. Prepare the credential. Object Storage S3 credentials: an access key and secret key.
2. In the dashboard, open **Settings > Backup targets** (`/settings/backup-targets`), then add a target and pick the Hetzner Object Storage tile.
3. Run Test connection on the new target before you schedule anything against it.

From the CLI:

```bash
levelrail-cli backup-targets create --name hetzner-backups --provider custom --endpoint https://fsn1.your-objectstorage.com --region fsn1 --bucket levelrail-backups --access-key-id <KEY_ID> --secret-access-key <SECRET>
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

Provider type `custom`. Endpoint: `https://{region}.your-objectstorage.com`. Default region: `fsn1`.

## Security notes

- The access key id and secret are written to the secrets store before the target record is saved, so a failed save never orphans a credential.
- They are write-only: no response echoes them. Rotate by running `backup-targets update` with a new pair.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Test connection proves the bucket exists and answers, not that the key can write or delete.
- Every backup write returns 501 until the control plane has a master key configured.

## Troubleshooting

**Test connection says authentication rejected.** The API answered 401 or 403. Re-enter the access key id and secret, and confirm the key is allowed on this bucket.

**Test connection says bucket not found.** The API answered 404. Check the bucket name, the region and the endpoint host together: the region slug in the host, such as fsn1, must match the location the bucket was created in.

**A manual backup fails with 507.** The control plane data directory has less free space than APP_MIN_BACKUP_DISK_MB (default 256). Free space there and retry.

## FAQ

**Can the same Hetzner Object Storage bucket hold database and volume backups?**

Yes. A backup target is its own resource, and any number of databases and app volumes can reference it by id.

**Will I see the secret key again after saving?**

No. Credentials are write-only: responses never echo them, and rotating means supplying a new pair with `backup-targets update`.

## Related

- [Hetzner Cloud](/integrations/cloud-providers/hetzner): Cloud node providers
- [Scaleway Object Storage](/integrations/backup-destinations/scaleway): Backup destinations
- [All backup destinations](/integrations/backup-destinations)
- [Hetzner Object Storage documentation](https://docs.hetzner.com/storage/object-storage/), maintained by the vendor
