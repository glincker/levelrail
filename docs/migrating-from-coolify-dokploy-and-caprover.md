---
title: "Migrating from Coolify, Dokploy, or CapRover"
sidebarTitle: "Moved: migration guides"
description: This page moved. Start at the Levelrail migration overview, then follow the guide for Coolify, Dokploy, CapRover, Dokku or Docker Compose.
"og:url": "https://docs.levelrail.com/migrating-from-coolify-dokploy-and-caprover"
---

This guide was split into one page per platform. Your old link still works and lands here.

- [Migrate to Levelrail](/migrate-to-levelrail): the overview, the six step flow, the downtime table, the checklists and rollback
- [Migrate from Coolify](/migrate-from-coolify)
- [Migrate from Dokploy](/migrate-from-dokploy)
- [Migrate from CapRover](/migrate-from-caprover)
- [Migrate from Dokku](/migrate-from-dokku)
- [Migrate from Docker Compose](/migrate-from-docker-compose)
- [Migration reference](/migration-reference): flags, mapping tables, database copy, hub and cutover details

## Read-only agent on the old server

Reading an old platform's API misses things: a container started by hand with `docker run`, a database the platform never tracked, a volume nobody remembers. The migration agent looks at the old server itself. It is a small separate program, `levelrail-migrate`, that you install on the server you are leaving and remove when you are done.

What it does in this release:

- Lists the containers, volumes and databases it finds on the old server's Docker, including ones the Coolify, Dokploy or CapRover API does not list. Containers that match no imported app are flagged, and database containers are labelled by engine and version.
- Counts the files and bytes in each volume (and bind mount) of an imported app and compares them with the copy on this server.
- When every volume of an app matches exactly, it completes the `data_verified` cutover step for you. A single difference, a missing volume, or files that changed while they were being counted keeps the step open.

What it does not do in this release:

- It does not dump databases and does not stream volume data. You still copy data with `levelrail-cli migrate db-copy` and `migrate volumes`. Database contents are not counted by the agent; the page and the CLI tell you how many database containers it saw so you do not forget them. The step note records that databases still need separate verification.
- It never starts, stops, changes or runs anything on the old server. It cannot complete `dns_switched` or `old_stopped`; those stay yours.

### Set it up

In the dashboard open **Migrate**, then **Source agent**, name the server and create the install command. From a terminal:

```
levelrail-cli migrate agent start --label old-box
```

Run the printed command on the old server as root, with the `levelrail-migrate` binary downloaded there. The command carries a single-use token and the control plane's CA fingerprint, which the agent requires: it will not enroll against an unpinned control plane. The token expires after one hour by default and works once. The command carries the token, so it lands in your shell history: clear that history entry (or run the command with a leading space where your shell supports it) once the agent has enrolled. The agent installs a hardened systemd unit and dials out to the control plane; the old server needs no inbound port.

Then check what it found and compare an app's volumes:

```
levelrail-cli migrate agent status
levelrail-cli migrate agent verify SESSION APP
```

If the app's volumes live on another node than the control plane, pass what you measured there with `--target NAME=FILES:BYTES`. The agent cannot vouch for numbers you typed, so the comparison is marked operator-supplied and `data_verified` is never completed automatically: tick it off yourself on the cutover checklist after checking the numbers.

A volume with 0 files and 0 bytes on the source matches an empty volume here, which is also what a failed copy looks like. Such volumes block automatic completion until you confirm they are meant to be empty with `--ack-empty` (the dashboard has a checkbox for the same). The acknowledgement is written to the audit log. Every verification audit entry records how many `--target` overrides were used (`overrides=N`).

Viewing a session's discovered containers, images and paths needs the same permission as creating a session; the session list stays readable with plain read access.

### Undo and remove

An agent-completed `data_verified` shows who completed it and the evidence. Undo it from the cutover checklist (the button is labelled "Undo agent verification") or with:

```
levelrail-cli migrate agent reopen APP
```

Undoing reopens `data_verified` and the steps after it, because they depended on it. A later verification can complete the step again.

Remove the agent when you are done, or sooner:

```
levelrail-cli migrate agent remove SESSION
levelrail-migrate remove
```

The first revokes the session and its certificate on the control plane and closes the stream; it works even when the old server is unreachable. The second, run on the old server, deletes the unit, the identity and the binary. A session also ends on its own after 72 hours by default.

### Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MIGRATION_TOKEN_TTL` | `1h` | How long an install token can enroll (1 minute to 24 hours) |
| `APP_MIGRATION_SESSION_MAX` | `72h` | Hard cap on a session and its certificate (1 hour to 7 days) |
| `APP_MIGRATION_REQUEST_TIMEOUT` | `15m` | How long the control plane waits for a discovery or scan reply |
| `APP_DOCKER_VOLUMES_DIR` | `/var/lib/docker/volumes` | Where the control plane measures volumes on this host |

Every session event (token issued, enrolled, each discovery and scan, denials, verification, step advance or reopen, revoke, expiry) is written to the audit log and follows its retention. See the [migration agent threat model](/migration-agent-threat-model) for what the agent can and cannot do.

Not yet verified on a real Coolify, Dokploy or CapRover host. Treat the first run as a rehearsal: install it on a server you can afford to look at closely.
