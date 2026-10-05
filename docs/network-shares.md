---
description: Register NFS and CIFS (SMB) network shares with the control plane, test that the host is reachable, and what is and is not wired up today for mounting a share into an app.
---

# Network shares

A network share is a saved record of an NFS export or a CIFS (SMB) share on your network, such as a NAS. You register it once, with the host, remote path and credentials, and test that the host answers. The record is stored by the control plane, and a CIFS password is stored encrypted with the master key, never returned by the API.

::: warning Registration only, for now
Today a network share is a managed record plus a reachability test. Nothing in the control plane yet creates a Docker volume from a share or mounts one into an app. The translation to Docker's built-in `local` driver NFS and CIFS options exists in `internal/docker`, but no code path calls it. The dashboard page text describes attaching a share as an app volume source; treat that as the intended direction, not a shipped capability. To use a share in an app today, mount it on the node yourself and bind-mount the path with a [`hostPath` volume](app-spec-reference.md).
:::

## Add a share

<Tabs :items="['Dashboard', 'CLI']">
<Tab value="Dashboard">

Open **Settings**, then **Network shares**, and add a share. Choose NFS or CIFS and fill in the fields below.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli network-shares create --name nas-media --protocol nfs --host nas.internal --remote-path /exports/media
levelrail-cli network-shares create --name office-files --protocol cifs --host files.internal --remote-path /team --username svc-backup --password 'change-me'
```

</Tab>
</Tabs>

| Field | CLI flag | Required | Notes |
| --- | --- | --- | --- |
| Name | `--name` | yes | Display name. Must be unique; a duplicate returns a conflict error. |
| Protocol | `--protocol` | yes | `nfs` or `cifs`. |
| Host | `--host` | yes | Hostname or IP of the server, for example `nas.internal`. |
| Remote path | `--remote-path` | yes | The export or share path, for example `/exports/data`. |
| Mount options | `--mount-options` | no | Extra mount options passed through as given. |
| Username | `--username` | CIFS only | Required for a CIFS share. |
| Password | `--password` | CIFS only | Required for a CIFS share. Stored encrypted. |

Creating a share with credentials needs the control plane's master key to be configured. Without one, the API refuses with a "not implemented" error. An NFS share carries no credentials.

## Test, update and remove

```bash
levelrail-cli network-shares list
levelrail-cli network-shares get <id>
levelrail-cli network-shares test <id>
levelrail-cli network-shares update <id> --name nas-media --protocol nfs --host nas.internal --remote-path /exports/media
levelrail-cli network-shares delete <id>
```

- **`test`** opens a TCP connection from the control plane to the share's host on the protocol's standard port (2049 for NFS, 445 for CIFS) with a 10 second timeout. A failure returns the address it could not reach. This is a reachability check only. It does not authenticate and never reads the CIFS password, so a passing test does not prove the credentials or the export path are right.
- **`update`** takes the full set of required fields again. Omit `--password` to keep the existing password; pass it to rotate it.
- **`delete`** removes the record. The control plane's secrets store has no delete operation yet, so a deleted CIFS share's encrypted password stays in the store, unreferenced.

All subcommands accept the standard `--token`, `--api-url`, `--profile`, `--json`, `--output` and `--query` flags.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/network-shares` | Read |
| `POST` | `/api/v1/network-shares` | Sensitive write |
| `GET` | `/api/v1/network-shares/{id}` | Read |
| `PUT` | `/api/v1/network-shares/{id}` | Sensitive write |
| `DELETE` | `/api/v1/network-shares/{id}` | Sensitive write |
| `POST` | `/api/v1/network-shares/{id}/test` | Read |

A share does not appear on the [project topology graph](service-topology-graph.md), because no stored link ties a share to an app volume.

<CardGroup :cols="2">
<Card title="Backups and storage" href="/backups-and-storage">

Backup targets and volume backups.

</Card>
<Card title="App spec reference" href="/app-spec-reference">

Volumes and host-directory bind mounts.

</Card>
</CardGroup>
