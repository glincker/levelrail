# ADR 039: Control plane HA, WAL shipping chosen over a Postgres driver

Status: Accepted (decision), prototype only (implementation)

Date: 2026-10-19

Supersedes the "pursue option 1 next" line of ADR 037 with measurements.

## Context

ADR 037 shipped an opt-in snapshot replica: a full verified `VACUUM INTO`
copy every interval. Its ship cost grows with database size, so it does not
reach second-level RPO on large installs. Open decision 4 in the project plan
asks whether continuous replication should be Litestream-style WAL shipping or
a Postgres driver. This spike measures the first and sizes the second.

## Prototype

`internal/walship` is a small in-process WAL shipper using only the SQLite
file and its `-wal` file, no new dependencies (gzip from the standard
library, the existing `modernc.org/sqlite` driver).

- It holds a read transaction between syncs so SQLite cannot restart the WAL
  under it, reads the WAL header, validates every frame (salts and the
  cumulative checksum, both byte orders) and ships only up to the last commit
  frame, gzip compressed.
- A generation is a DB file snapshot plus the WAL epochs after it. The
  snapshot is copied under the read hold and replay starts at the first frame
  of the current epoch, so a page the primary checkpoints mid-copy is
  rewritten by replay.
- It owns checkpointing: at a size threshold it takes the writer lock, ships
  the tail, releases its hold, runs a PASSIVE checkpoint and re-takes the hold
  with no gap, which lets the next writer restart the WAL. The header
  checkpoint sequence must advance by exactly one per epoch; any jump means
  frames were lost, and the shipper starts a new generation instead of
  guessing.
- `Restore` takes the newest snapshot, replays each epoch (write header plus
  frames as a `-wal`, open, `wal_checkpoint(TRUNCATE)`), and stops at the first
  missing segment.
- Off by default, no setting reads it, not started by the control plane.

Deliberate exception to the "UI, CLI and API together" rule: there is no
dashboard, CLI or API surface in this task. It is a measured prototype behind
a package nothing imports. The operator surface (status, lag, restore drill)
lands with the real wiring, not before the recommendation is accepted.

## Measured

Apple laptop, local directory sink, store-like pragmas (WAL, single
connection), deploy churn: per deploy one insert, 12 status updates (each its
own commit) and one commit of 6 event rows of 512 B log text. Run once,
numbers are single-run unless a range is shown. Reproduce with
`go test -run TestLive ./internal/walship` (skipped under `-short`).

Replication lag (commit to segment durable in the sink), 5600 commits:

| Sync interval | p50 | p95 | p99 | max |
| ------------- | --- | --- | --- | --- |
| 100 ms | 75 ms | 155 ms | 226 ms | 294 ms |
| 1 s | 547 ms | 1.01 s | 1.08 s | 1.21 s |

Bytes shipped (logical MB is the payload the workload wrote, not WAL bytes):

| Case | Logical writes | Raw WAL frames | Shipped (gzip) | Shipped per logical MB | 5 s snapshot replica, same run |
| ---- | -------------- | -------------- | -------------- | ---------------------- | ------------------------------ |
| 1.4 MB database, 100 ms | 1.44 MB | 31.0 MB | 2.52 MB | 1.75 MB | 1.07 MB per logical MB |
| 1.4 MB database, 1 s | 1.44 MB | 29.6 MB | 2.25 MB | 1.56 MB | 0.89 MB per logical MB |
| 127 MB database, 100 ms | 1.08 MB | not recorded | 2.59 MB | 2.39 MB | 190 MB per logical MB |

Raw WAL frames are about 21x the logical bytes (whole 4 KB pages per small
commit), gzip brings that to roughly 1.6 to 2.4x. On a tiny database the
snapshot replica is cheaper; at 127 MB WAL shipping ships about 80x less. The
first snapshot of the 127 MB database took 11.1 s (22.6 MB shipped, in-process
gzip), a one-time cost per generation.

Overhead of the shipper in the writing process (600 deploys, 100 ms interval,
three paired runs, shipper off then on, CPU from `getrusage`, RSS sampled with
`ps`):

| Round | CPU delta | Share of wall | RSS delta |
| ----- | --------- | ------------- | --------- |
| 0 | +0.63 s | 3.1% | +30 MB |
| 1 | +3.57 s | 8.6% | +142 MB |
| 2 | +1.32 s | 5.6% | -85 MB |

These are noisy (shared laptop, Go GC timing, one run had a 41 s wall time),
so read them as "single digit percent of one core, RSS dominated by gzip and
snapshot buffers" and not as a precise figure. A soak on a quiet server must
replace this row before anything ships. The 127 MB churn run took 40.7 s for
work that took about 21 s on the small database; no shipper-off baseline was
taken for it, so that slowdown is unattributed and is a soak item.

Restore correctness (5 trials, primary SIGKILLed at 3 to 7 s into the churn
with the WAL rolling over every 256 KB, then `Restore` into a fresh file):

| Check | Result |
| ----- | ------ |
| `PRAGMA integrity_check` on replica | ok in 5 of 5 |
| Primary integrity after kill | ok in 5 of 5 |
| Replica ids contiguous (no holes, a true prefix) | 5 of 5 |
| Replica row count above primary | never |
| Committed deploys lost (RPO in practice) | 0, 0, 10, 2, 0 (6, 0, 60, 12, 0 event rows) |
| WAL epochs replayed | 19 to 57 |

The worst loss was 10 deploys, about 140 commits, a few hundred milliseconds
of writes at this pace, consistent with the 100 ms interval plus checkpoint
stalls. The replica was always a consistent prefix of the primary.

## Postgres driver, sized not built

Counts from this tree (non-test Go unless stated):

- 244 files in `internal/store` with about 1390 `Query`/`Exec`/`QueryRow`
  call sites there, and 233 files across `internal` and `cmd` importing
  `database/sql`. Every query uses `?` placeholders (mechanical rebind).
- Dialect work that is not mechanical: `INSERT OR IGNORE/REPLACE` (29 sites),
  `strftime` (75 in Go, 27 in migrations), `json_extract` (16), `PRAGMA`
  (14), `RETURNING` (8, fine), FTS5 (5 plus 1 migration, needs a different
  search design), `AUTOINCREMENT` (23 migration uses), `COLLATE NOCASE` (4).
  `ON CONFLICT` (176) mostly ports as is.
- 342 files in `internal/store/migrations`, forward-only SQLite dialect. A
  second driver means a second migration tree or a translation layer plus a
  dual-dialect CI lane, forever.
- File-based machinery that assumes SQLite: `cpbackup` (`VACUUM INTO`,
  snapshot replica), `integrity`, the rollback host, restore drills, the
  separate alerting and telemetry databases, and the single-connection
  writer assumptions behind `SetMaxOpenConns(1)`.
- Estimate (not measured): four to six engineer weeks for a working driver
  plus a permanent test-matrix cost, and it removes "one binary, one file"
  from the footprint story unless Postgres is embedded, which is its own
  project.

## Options rejected

- **Postgres driver now.** It buys zero RPO and several active control plane
  instances, neither of which a single-operator control plane needs yet, at
  the cost above and the loss of the footprint claim. Revisit when a
  requirement needs more than one active control plane instance.
- **Keep snapshot replicas only.** Correct and simple (ADR 037) but the table
  shows the cost per change scales with database size, not with writes. Kept
  as the default opt-in for small installs.
- **Litestream as a sidecar or library.** Solves the same problem, but adds a
  second process or a large dependency to a single static binary, and its
  checkpoint ownership would still need the soak below. The prototype is
  about 630 lines.
- **Replicate at the application layer (event log).** Would touch all 1390
  write sites and still need snapshots. No.

## Decision

1. Recommend WAL frame shipping as the continuous replication path. It
   measured sub-second lag at a 100 ms interval, restored a consistent prefix
   in every kill trial and ships about 2x the logical write volume instead of
   a full snapshot per interval.
2. Do not add a Postgres driver. Failover stays manual (restore the replica on
   a new machine), as in ADR 037.
3. Before it becomes a real feature, in order: an object-store sink reusing the
   `cpbackup` remote and age encryption per segment; generation retention and
   a periodic fresh snapshot; a soak on a quiet server (days, checkpoint
   ownership, WAL growth when a sink is down, CPU and RSS baseline); a restore
   drill through the existing restore path; then the operator surface
   (status, lag, last verified restore) in the dashboard and CLI.
4. Until then `internal/walship` stays unwired and off. It must not run
   beside the ADR 037 snapshot replica on the same database without the soak,
   because both observe checkpointing.

## Consequences

- The prototype read hold blocks WAL restart between checkpoints, so a stopped
  or failing sink makes the WAL grow. The real version needs a size alarm and
  a policy (drop the hold and re-snapshot) when the sink is down.
- Frame loss is detected, not prevented, in a narrow window around the
  hand-off after a checkpoint: a restart sequence jump forces a new
  generation, so correctness holds and cost is one extra snapshot.
- Restore replays WAL epochs through SQLite's own recovery, which re-verifies
  every frame checksum, so a corrupt segment truncates the replay instead of
  corrupting the database.
- This is still not zero RPO and not automatic failover. The docs must keep
  saying so.
