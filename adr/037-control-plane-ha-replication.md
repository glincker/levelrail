# ADR 037: Control plane HA, snapshot replica first

Status: Accepted (spike, opt-in)

Date: 2026-10-19

## Context

The control plane keeps all state in one SQLite file (ADR 007). Section 9 of
the project plan leaves HA open: replicate SQLite Litestream-style, or add a
Postgres driver. Until that is decided "no single point of failure" cannot be
claimed. The data plane already survives a control plane outage (ADR 025), so
the question is how much state a lost control plane machine takes with it.

Today's answer is the scheduled encrypted off-box backup (daily by default)
plus restore drills, which bounds loss at a day.

## Options

1. **Litestream-style WAL shipping.** Hold a read transaction so SQLite cannot
   reset the WAL, copy new frames to object storage, release, checkpoint.
   RPO is about a second. Cost: the shipper must own checkpointing, so a bug
   can grow the WAL without bound or lose frames silently, and restore must
   replay a snapshot plus a frame chain. Pure Go, keeps the single binary.
2. **Postgres driver.** Real HA (streaming replicas, synchronous commit, RPO
   zero) and a path to several control plane instances. Cost: a second
   database to run, a second dialect for every query and migration, and the
   end of "one binary and one file", which is the product's footprint story.
3. **Snapshot replica (this spike).** On an interval, if the database changed,
   take the same verified `VACUUM INTO` snapshot the backups use, encrypt it to
   the same age recipients and overwrite one of two alternating slots. RPO is
   the interval plus the ship time. No change to how SQLite checkpoints.

## Measured

Local object store on a development laptop, encrypted, full snapshot each
time (internal/cpbackup `TestReplicaMeasuredShipTime`, run without `-short`):

| Database | Shipped | Ship time |
| -------- | ------- | --------- |
| fresh (migrations only) | 88 KB | 83 ms |
| 50k rows | 2.8 MB | 234 ms |
| 200k rows | 13.7 MB | 833 ms |

With the 5 s minimum interval the measured RPO is about 5 to 6 s for these
sizes, and the ship cost grows linearly with database size, which is why this
is not the end state: at hundreds of MB the interval has to stretch to minutes.
Restore uses the existing verified restore path (checksum, decrypt, integrity
check, install-id check, atomic swap) and falls back to the older slot when the
newest fails verification (tested with a flipped bit).

## Decision

- Ship option 3 now, off by default (`APP_CONTROL_PLANE_REPLICA_INTERVAL`), as
  the cheap step that turns a day of exposure into seconds for small installs
  with no new failure modes.
- Pursue option 1 next as the real continuous path, behind the same setting,
  once the WAL chain format and a checkpoint-ownership soak test exist.
- Do not add Postgres until a request needs more than one active control plane
  instance. Failover stays manual: restore the replica on a new machine
  (`restore` runbook in docs/disaster-recovery.md).

## Consequences

- Change detection is a stat of the database and WAL files plus a content hash
  of the snapshot, so an idle control plane uploads nothing.
- The replica contains wrapped secrets like every backup and is only readable
  with the age identity; the master key still travels by escrow.
- This is not automatic failover and not zero RPO. The docs say so.
