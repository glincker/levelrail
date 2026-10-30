---
title: "Migrate over SSH"
description: "Import apps, env, images and volumes from an old Coolify, Dokploy, CapRover or Dokku host with only an SSH login. Read only, no agent, no remote command."
"og:url": "https://docs.levelrail.com/migrate-over-ssh"
---

Use this when you have an SSH login on the old server but no platform API token, or you do not want to install anything there. The control plane opens an SSH connection, uses it only to reach the old host's Docker socket, and reads containers, volumes and images through the Docker Engine API. It never runs a command on the old host.

For the token based importer, see [Migrating from Coolify, Dokploy, or CapRover](/migrating-from-coolify-dokploy-and-caprover).

## How it works

- SSH is a transport only. The control plane opens one `direct-streamlocal@openssh.com` channel to the Docker socket (default `/var/run/docker.sock`) and speaks HTTP over it.
- There is no SSH session, no shell, no exec, no PTY, no port forwarding and no SFTP. A test fails the build if the code that does this references any of them.
- The old host's SSH server does the connecting to the socket, as the SSH user. Nothing is installed there.

## What it reads

Every request is checked on the control plane before it is sent. Anything off this list is refused with a typed error and never leaves the process.

| Request | Used for |
| --- | --- |
| `GET /_ping`, `/version`, `/info` | Connection test, Docker version, Swarm detection |
| `GET /containers/json`, `/containers/{id}/json` | The app inventory: image, env, labels, ports, mounts, health check, limits |
| `GET /volumes`, `/volumes/{name}`, `/networks`, `/networks/{id}` | Counts and context |
| `GET /images/json`, `/images/{name}/json`, `/images/{name}/get` | Host built images, streamed as an image archive |
| `GET /containers/{id}/archive?path=...` | One volume copy, only for a path the container's own inspect lists as a mount destination |
| `GET /services`, `/tasks`, `/nodes` | Swarm sources (CapRover) |

## What it never does

- Any request that is not a GET: no create, start, stop, remove, exec, attach, build, commit, image pull or load, auth or session call.
- Logs, events, stats, exports and archives of paths that are not a mount of the container.
- Write to the old host in any way. Stopping apps there, changing DNS and removing the old platform stay your decision.
- Store the SSH key or password. They are held in the control plane process memory for the session (up to two hours by default, `APP_MIGRATE_HUB_PASSWORD_TTL`), never written to the database, settings or logs, never returned by the API, and zeroed when the session is forgotten or expires. Anyone who can read that process memory during the session can read them.
- Read the old platform's stored config. Env and settings come from the running containers only. Coolify's settings database, CapRover's data folder and Dokku's app folder are not read. Domains that live only in that stored config (for example a Dokku nginx file) have to be added after the plan.

## Connect

1. Open Migrate, choose Import apps, then the **Connect over SSH** tab.
2. Enter the address, SSH user and the private key (paste it or load a file). A passphrase or a password is optional.
3. Press **Read host key**. The control plane connects without logging in and shows the host keys the server presents.
4. Compare one fingerprint with what the old server reports, pick it and tick the confirmation. Connecting is refused until a fingerprint is confirmed.
5. Press **Test connection**. It shows exactly what will be read ("read only: lists containers, volumes and images") and the counts it found.
6. Press **Read inventory**. From here the flow is the same as any import: plan, stage, images, verify, volumes, cutover, rollback.

CLI equivalent:

```bash
levelrail-cli import apps --from docker-ssh --ssh root@old.example.com --ssh-key ~/.ssh/old_key --test-connection
# lists the host keys when --fingerprint is missing, then:
levelrail-cli import apps --from docker-ssh --ssh root@old.example.com --ssh-key ~/.ssh/old_key \
  --fingerprint SHA256:... --apply
levelrail-cli import apps --session ID --transfer-images
levelrail-cli import apps --session ID --copy-volumes
levelrail-cli import apps --session ID --verify
```

The passphrase comes from `APP_IMPORT_SSH_PASSPHRASE` and a password from `APP_IMPORT_SSH_PASSWORD`. `--insecure-host-key` skips host key verification. It is warned about in the CLI, the dashboard and the session view, and it exposes the login to anyone on the path, so use it only on a network you control.

## Images and volumes

- **Images.** Host built images are streamed through the same tunnel (`GET /images/{name}/get`), loaded on the target node and verified by content and image ID. No `docker save` runs on the old host, and the Move images step needs no second login.
- **Volumes.** For a running container, each named volume is streamed with the container archive endpoint and unpacked into the staged app's volume. Extraction refuses names that leave the volume, never writes through a symlink, skips symlinks that point outside, hard links and device nodes (reported per volume), and stops at a size and file count cap (`APP_MIGRATE_VOLUME_MAX_BYTES`, default 10 GiB, and `APP_MIGRATE_VOLUME_MAX_FILES`, default 500000). Run the copy again right before the DNS switch to catch the last changes.
- **Fallback.** Stopped containers and bind mounts are not copied through the tunnel. The Volumes step keeps the printed `rsync` commands for them.

The tunnel copy writes to the volume directory on the control plane node, so it works for apps placed on that node. Apps placed on another node use the `rsync` commands.

## Failures and their fixes

The Test connection result names the cause and offers the fix as a button where the control plane can do it.

| Cause | What to do |
| --- | --- |
| Login refused | Check the key, passphrase and user, then Re-check. |
| Host key mismatch | The server shows a different key than the one you confirmed. If you expected it, trust the new fingerprint and confirm it again. |
| Permission denied on the socket | The SSH user is not allowed to use the Docker socket. Add it to the `docker` group on the old host and log in again (the dashboard shows the command in a collapsed "Fix via CLI"). |
| No socket at that path | Pick another socket path. The page offers `/var/run/docker.sock` and `/run/docker.sock`. Rootless Docker uses `/run/user/UID/docker.sock`. |
| Socket forwarding refused | `sshd` on the old host blocks forwarding to a Unix socket for this login. Allow `AllowStreamLocalForwarding` for the user. |
| Address not allowed | Private addresses need `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` on the control plane and the Allow private option. Cloud metadata and link local addresses are never allowed. |

## Hardening the old host (optional)

Whoever holds the SSH key has the same power as the SSH user on the old host. The Docker socket is root equivalent, so holding the key means root on that host. The allowlist protects what this control plane sends, not the host from the key.

Use a key made for this move and remove it from `authorized_keys` when you finish. If you want sshd to restrict the key to the socket, a line like this is worth trying:

```text
restrict,permitopen="/var/run/docker.sock" ssh-ed25519 AAAA... levelrail-migration
```

`restrict` turns off everything not re-enabled, and `permitopen` limits what the key may forward to. How `permitopen` behaves for Unix sockets (`direct-streamlocal`) varies between OpenSSH versions and this has not been verified against a live host here. Test it: run Test connection with the restricted line in place, and loosen it if the connection is refused. A restriction that breaks the connection is safe, one that is silently ignored is not, so also confirm that the key cannot do more than you intend.

## API

| Route | Purpose |
| --- | --- |
| `POST /api/v1/migration/apps/ssh/host-keys` | Read the host keys a server presents, without logging in |
| `POST /api/v1/migration/apps/ssh/test` | Log in and count what an import can read |
| `POST /api/v1/migration/apps/sessions` | Create a session with `platform: "docker-ssh"` and an `ssh` object |
| `POST /api/v1/migration/apps/sessions/{id}/volumes/copy` | Start the tunnel volume copy, then poll the same path with GET |

All of them need the `write:sensitive` ability (or root) except the GET status. The `ssh` object takes `host`, `port`, `user`, `private_key`, `passphrase`, `password`, `host_key_fingerprint`, `insecure_host_key` and `socket_path`. Secrets are accepted in the request body only and are never echoed.
