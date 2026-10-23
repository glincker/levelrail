---
title: "Migration agent threat model"
sidebarTitle: "Migration agent threat model"
description: What the read-only migration agent on the old server can and cannot do, how it is constrained, and the risks that remain.
"og:url": "https://docs.levelrail.com/migration-agent-threat-model"
---

The migration agent runs on a server you are leaving. This page lists what it is trusted with, how that is limited, and what is left over. It describes the first release, which discovers and compares only.

## What it can do

- Read Docker's list of containers, volumes and networks, and inspect containers. It reports environment variable names, never their values.
- Count files and bytes under a volume or bind mount that it reported itself in its last discovery.
- Send a heartbeat and answer the control plane.

## How it is constrained

- **Separate program.** `levelrail-migrate` is its own binary and does not link the node agent's deploy, build, exec or reconcile code. A test checks the import graph so this cannot regress silently.
- **Docker allowlist.** Every call to the Docker socket passes a filter on method and path. Only a short list of read-only `GET` calls is allowed; everything else, including exec, create, start, stop, delete, archive and logs, is refused before it reaches the socket and reported to the control plane. Each forbidden call is covered by a test.
- **Closed message set.** The control plane can send exactly three things: discover, scan and go-away. The wire contract has no deploy, exec or build message. Anything else ends the session.
- **Verification cannot be forged.** Numbers typed in with `--target` or `target_stats` mark the comparison operator-supplied and never complete `data_verified`; empty volumes need an explicit acknowledgement, which is audited together with the override count.
- **Enrollment is rate limited and cheap to refuse.** The token id has a fixed format, refusals are written to the audit log only for sessions that exist, and each source address gets a limited number of attempts per window (`APP_MIGRATION_ENROLL_RATE`, `APP_MIGRATION_ENROLL_WINDOW`, `APP_MIGRATION_AGENT_FIELD_MAX`).
- **Scans are bounded.** The agent refuses to scan `/`, `/proc`, `/sys` and `/dev`, caps targets per request, entries and directory depth (`APP_MIGRATE_SCAN_MAX_TARGETS`, `APP_MIGRATE_SCAN_MAX_ENTRIES`, `APP_MIGRATE_SCAN_MAX_DEPTH`) and stops at `APP_MIGRATE_SCAN_TIMEOUT`.
- **Scan targets are the agent's own.** A scan may only name a volume or bind path that the agent listed itself, so the control plane cannot point it at arbitrary host paths.
- **No node identity.** A migration session is never a node. Its certificate has a different subject, it cannot open a node stream, a node certificate cannot open a migration stream, and nothing is ever scheduled onto it.
- **Single-use, short-lived token.** The token works once, expires after one hour by default, and is stored only as a hash. It is compared in constant time. A leaked, unused token is useless once it expires or is revoked.
- **Pinned control plane.** The install command carries the control plane CA fingerprint and the agent refuses to enroll without it. There is no trust on first use.
- **Hard session cap.** Sessions and their certificates end after 72 hours by default. Revoking from the control plane works even when the host is unreachable, and the agent removes its own identity when it is told to stop.
- **Everything is audited.** Token issue, enrollment (and refusals), session start and end, discovery, scan, denied Docker calls, verification, step changes and revokes are written to the audit log.

## What remains

- **Docker socket access is root on the old server.** The allowlist is a seatbelt, not a sandbox. A compromised or tampered agent binary owns that host. Download the binary from the official release and check its checksum, install it only on a server you are leaving, and remove it when you are done. Running it in a container with the socket mounted does not reduce this.
- **Discovery shows names.** Container names, images, mounts and environment variable names leave the host and are stored on the control plane. Values do not.
- **A compromised control plane** can ask the agent to count files in any volume the agent reported. It cannot make the agent read file contents, change anything, or run anything.
- **Counts are not content hashes.** Matching file counts and sizes catch a missed or truncated copy, not silent corruption. Data is not streamed by this release, so this release does not claim to verify it end to end.
- **Databases are out of scope.** The agent does not dump or count database rows. Verify databases with the database copy tools.
- **Not yet run against a real production host.** The systemd hardening options and the allowlist against every Docker version are unverified outside tests.

Report a security concern with the process in the security policy of the repository.
