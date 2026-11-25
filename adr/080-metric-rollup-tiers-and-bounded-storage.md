# ADR 080: Metric rollup tiers and bounded storage

Status: Proposed

## Context

Container metrics are 7 series per app (cpu_percent, memory_usage_bytes, memory_limit_bytes, network_rx_bytes, network_tx_bytes, disk_read_bytes, disk_write_bytes, see `sampleValues` in `internal/telemetry/collector.go`), sampled every 15 s while someone watches and every minute otherwise (`APP_METRICS_IDLE_INTERVAL`). A local measurement of 200 apps at 15 s for a simulated day gave 8.06M raw rows, 840 MB on disk, 104 bytes per row (table plus ts index). The 1 GiB default cap (`APP_METRICS_MAX_DB_BYTES`) is reached after about 29 hours, so today the cap, not the 15 day raw retention, decides how much history a user keeps. Deleting one hour of raw rows (336k rows) through the batched sweep took 29 s and a concurrent reader waited up to 22 s.

What exists on `release/v1.5.10`, read in code:

- Tiers already exist: raw `metric_samples`, and `metric_rollups` with tier 60 (1 minute) and tier 3600 (1 hour), columns `sum, min, max, count`, primary key `(tier, resource_id, metric, ts)` (`migrations/0004_metric_rollups.sql`).
- `Rollup` (`rollup.go`) is incremental through `rollup_state.done_through`, closes buckets after `Lag` (30 s), builds the hour tier from the minute tier, and upserts, so a rerun overwrites. One run may cover up to `MaxBucketsPerRun` (1440) buckets in one `INSERT ... SELECT` transaction, which on catch up is a full day of raw rows in one write.
- `Prune` sweeps each tier past retention in batches (`retention_batch.go`), then while the file is over `MaxBytes` removes the oldest tenth of the finest tier that the next tier already covers. Raw is never dropped before it is rolled up.
- `pickTier` and `queryTier` (`tiers.go`) pick a tier by span (raw up to 6 h, minute up to 7 d) and fill the not yet rolled up tail from the finer tier, so there are no gaps at the watermark seam.
- Defaults: raw 15 d (runtime setting `metrics.retention`, live), minute 30 d, hour 365 d. Only raw retention is a live setting; the other tier knobs come from env through `TierConfigFromEnv` at boot.
- The telemetry DB is one connection for reads and writes (`db.go`, `SetMaxOpenConns(1)`). The main store already has a query_only read pool (`internal/store/readpool.go`).

Problems found while reading, beyond growth:

1. Averages of averages. `queryRollup` returns one value per bucket (`sum/count` for gauges) and drops `count`, `min` and `max`. The API then runs `telemetry.Aggregate` (`federate.go`), which averages those points with weight 1 each. A chart step that spans the seam weighs one raw sample the same as a whole minute bucket, and the reported `Max` is the max of averages, not the true max.
2. Cumulative counters roll up wrongly. The network and disk series are running totals from the container engine. Their rollup `sum` is a sum of totals, which means nothing, and a restart resets the total. Only `http_*` series are treated as per tick deltas (`IsSumMetric`).
3. The size cap measures the whole telemetry file (`liveBytes` is page_count minus freelist), which also holds logs, spans, errors and traffic tables. Logs can fill the file and the metric prune then strips metric history to make room it cannot win back.
4. Empty buckets are skipped, so a chart draws a line across a period with no samples.

## Decision

Keep the tiered design, change the tier set and the row shape, and fix reads. In order of value:

1. Tiers become raw, 5 minute, 1 hour and 1 day. The 1 minute tier is retired. Each tier is built only from the tier directly below it.
2. Every rollup row stores `sum, min, max, count` and the meaning of `sum` depends on the metric kind (below). Averages are only ever computed as `SUM(sum) / SUM(count)` at the last step, never from other averages.
3. Series keys move to a dictionary: `metric_series(id INTEGER PRIMARY KEY, resource_id, metric, kind)`, and raw and rollup rows carry `series_id` instead of two text columns. This is what makes the defaults below fit; without it the same retention costs about 2.3 times more.
4. Telemetry reads move to a query_only read pool, the single writer stays. WAL lets readers run beside the one writer, so the 22 s reader stall goes away; the writer still serializes collector, rollup and prune, which is fine at these write sizes.
5. The cap measures metric tables only and prunes to a headroom target, not to the edge.
6. All tier boundaries, retentions, the cap and the job pacing are live runtime settings. Env only seeds the default.

### Metric kinds and how they roll up

| Kind | Examples | Raw value | Rollup `sum` | Shown as |
| --- | --- | --- | --- | --- |
| gauge | cpu_percent, memory_usage_bytes, memory_limit_bytes, disk_used_bytes | level at sample time | sum of values | avg = sum/count, plus min and max |
| delta | http_requests, http_bytes_in, latency bucket counts | amount since last tick | sum of values | total per step, rate = sum / step seconds |
| cumulative | network_rx_bytes, network_tx_bytes, disk_read_bytes, disk_write_bytes | running total | reset aware increase inside the bucket | increase per step, rate = increase / step seconds |

Kind lives on `metric_series.kind`, set from a code table keyed by metric name with gauge as the fallback. The increase for a cumulative series is computed at the 5 minute rollup with a `LAG` window over raw rows ordered by ts: each delta is `value - prev`, or `value` when it is negative (a reset), and is credited to the bucket of the later sample. The scan starts one lookback before the bucket (2 times the idle sample interval) so the delta across a bucket edge is not lost. Coarser tiers add increases, and add min, max and count as usual. For cumulative rows `min` and `max` keep the raw totals so the latest total stays readable.

Combining rows of one series across tiers or steps is always: `sum = SUM(sum)`, `count = SUM(count)`, `min = MIN(min)`, `max = MAX(max)`. A raw sample enters that same algebra as `sum=value, count=1, min=max=value` (cumulative raw samples enter as their delta). `telemetry.Aggregate` changes to this algebra, so a step that covers raw tail plus rollup rows weighs each by its real count.

### Tier boundaries and defaults

| Setting key | Env seed | Default | Today |
| --- | --- | --- | --- |
| metrics.retention (raw) | APP_METRICS_RETENTION | 6h | 360h |
| metrics.retention_5m | APP_METRICS_RETENTION_5M | 72h | (1m tier: 720h) |
| metrics.retention_1h | APP_METRICS_RETENTION_1H | 840h (35 d) | 8760h |
| metrics.retention_1d | APP_METRICS_RETENTION_1D | 9600h (400 d) | none |
| metrics.raw_query_max_range | APP_METRICS_RAW_QUERY_MAX_RANGE | 3h | 6h |
| metrics.5m_query_max_range | APP_METRICS_5M_QUERY_MAX_RANGE | 72h | (1m: 7 d) |
| metrics.1h_query_max_range | APP_METRICS_1H_QUERY_MAX_RANGE | 840h | none |
| metrics.max_db_bytes | APP_METRICS_MAX_DB_BYTES | 1 GiB | 1 GiB |
| metrics.cap_target_percent | APP_METRICS_CAP_TARGET_PERCENT | 85 | none |
| metrics.rollup_lag | APP_METRICS_ROLLUP_LAG | 30s | 30s |
| metrics.rollup_every | APP_METRICS_ROLLUP_EVERY | 1m | fixed 1m |
| metrics.prune_every | APP_METRICS_PRUNE_EVERY | 1h | fixed 1h |
| metrics.rollup_budget | APP_METRICS_ROLLUP_BUDGET | 5s | none |
| telemetry.read_conns | APP_TELEMETRY_READ_CONNS | 4 (0 sends reads to the writer) | none |
| telemetry.read_timeout | APP_TELEMETRY_READ_TIMEOUT | 10s | none |

Validation keeps each tier's retention at least its query max range plus one bucket, and each coarser retention longer than the finer one. A save that breaks that is rejected with the reason, not clamped silently. Chart spans then map as: up to 3 h raw (720 points at 15 s, capped by `APP_METRICS_MAX_POINTS`), 3 h to 3 d from 5 minute, 3 d to 35 d from 1 hour, longer from 1 day.

A raw default of 6 h is a real loss of detail versus 15 d. It is the choice that fits the budget, and the 5 minute tier keeps min and max, so short spikes stay visible as a max even after raw is gone.

### Size estimate

Assumptions (ESTIMATE): 7 series per app, worst case live sampling at 15 s, raw row 40 B and rollup row 65 B with the series dictionary (today: 104 B measured raw, about 140 B rollup with text keys, not measured). Request series (`http_*`, 12 latency slots) and host series are not included; an app behind ingress with traffic adds up to about 17 delta series, written only on ticks with traffic.

| Tier | Rows per series | 10 apps | 200 apps | 1000 apps |
| --- | --- | --- | --- | --- |
| raw 6 h | 1440 | 4 MB | 81 MB | 403 MB |
| 5 min 72 h | 864 | 4 MB | 79 MB | 393 MB |
| 1 h 35 d | 840 | 4 MB | 76 MB | 382 MB |
| 1 d 400 d | 400 | 2 MB | 36 MB | 182 MB |
| Total | | 14 MB | 272 MB | 1.36 GB |
| Total at today's row size | | 31 MB | 622 MB | 3.1 GB |
| Total with idle sampling (raw 4x smaller) | | 11 MB | 211 MB | 1.06 GB |

At 1000 apps the default cap is reached and the cap decides, as below. The cap is a setting; the dashboard shows the estimate so an operator can raise it or shorten a tier.

### Rollup job

- Runs every `metrics.rollup_every` on the writer, one tier at a time, raw to 5 m, 5 m to 1 h, 1 h to 1 d. A tier never passes the watermark of the tier below.
- One transaction covers one bucket of the target tier (5 m: 28k raw rows at 200 apps) and advances that tier's watermark in the same commit. Between transactions it sleeps the retention pause setting, so the collector and log ingest get the writer. It stops at `metrics.rollup_budget` and resumes next run. This replaces the up to 1440 bucket transaction.
- Buckets are aligned to the tier width in UTC, chunks never split a bucket, and the upsert overwrites, so a rerun of any range gives the same rows. Day buckets are UTC days; local time days are an open question.
- A sample that arrives after its bucket closed (past `rollup_lag`) is in raw but not in the rollup. That is accepted and counted in a `metrics_rollup_late_samples` counter, so it is visible rather than silent.

### Queries and tier seams

- `pickTier` picks by span, then moves coarser if the finer tier no longer holds the range start. This stays.
- The seam rule stays: read the chosen tier up to its watermark and fill the tail from the next finer tier, recursively down to raw. Because rollup buckets start at the watermark and raw rows start there too, rows never overlap and never leave a hole.
- Rollup rows return `sum, min, max, count` to the API and `Aggregate` merges them with the algebra above. The step is never finer than the chosen tier's width.
- Empty steps are emitted with `count = 0` and no value, so a chart draws a gap. The API adds this behind a `fill=gaps` query parameter so older clients keep their shape until the web chart opts in.

### Cap and prune

- Retention sweeps per tier keep the batched delete. With reads off the writer, a long sweep delays writes only, and the collector has 15 s of slack per tick.
- The cap counts metric tables only, through `dbstat` (already used in `internal/alerting/history_stats.go` and `internal/store/dbspace.go`), falling back to rows times measured bytes per row when `dbstat` is missing. Logs keep their own budget (`APP_LOG_BUDGET_MB`).
- Over the cap, prune removes the oldest tenth of the finest tier already covered by the next tier, as today, until metric bytes are under `cap_target_percent` of the cap. When raw keeps less than `raw_query_max_range`, a warning event fires and the settings page says the cap, not retention, is deciding.
- After each prune run: `PRAGMA incremental_vacuum` bounded in pages, then `PRAGMA wal_checkpoint(PASSIVE)`. A checkpoint cannot pass an open reader, so `telemetry.read_timeout` bounds every read and the WAL cannot grow without limit behind a stuck chart query. `journal_size_limit` is set on the telemetry DB as it is on the main store.

### Migration of existing data

1. A migration creates `metric_series`, `metric_points (series_id, ts, value)` and `metric_aggs (tier, series_id, ts, sum, min, max, count)`, plus watermarks for the new tiers. Old tables stay untouched.
2. A backfill job, paced like the rollup job, fills the dictionary, copies raw rows newer than the raw retention, builds 5 m from the old tier 60 rows (exact for gauges and deltas, since 5 m edges are 1 m edges), copies old tier 3600 rows to 1 h, and builds 1 d from 1 h.
3. For cumulative series the old minute rows only have sum of totals, so the 5 m increase is taken as `max - min` per minute bucket. That is exact without a reset inside the minute and undercounts a bucket with a reset. Old hour rows of cumulative series have the same limit.
4. Until the backfill watermark reaches the cutover point, reads and writes use the old tables. At cutover, one transaction flips a `metrics_schema` state row, the collector writes to the new tables, and queries read them.
5. Old tables are dropped by a later release, not this one. `DROP TABLE` is near instant, unlike a row delete.

### Failure and rollback

- Rollup or prune error: logged, `footprint` loop shows the error, watermarks did not move, the next run retries the same bucket. No partial bucket is visible because the bucket and its watermark commit together.
- Process killed mid backfill: resumes from its watermark; the old tables keep serving.
- Disk full: the disk reserve in the data dir already gives SQLite room to commit; rollup skips while disk is at the critical threshold, prune keeps running.
- Rollback to the previous binary before cutover: nothing changed for it. After cutover: the old binary reads the old tables, which stopped at cutover, so charts show a gap from cutover to rollback, and new tables are ignored. No data is corrupted. The release notes say this.
- A wrong kind for a metric (for example a counter left as gauge) only affects how new buckets read; fixing the table and recomputing the affected range from raw is possible while raw still holds it, not after.

## Alternatives rejected

- Keep the 1 minute tier and only shorten retentions. 1400 series at 1 minute is 2M rows a day; even 3 days of it is larger than the whole proposed set.
- Downsample only at query time (what `AutoStep` does). It bounds points per chart, not bytes on disk.
- Per day raw tables or per day attached files, so retention is a `DROP TABLE`. SQLite has no native partitioning, so every query spanning days becomes a `UNION ALL` built in code, schema changes go to every partition, and attach limits apply. With raw at 6 h the raw table holds hours, not days, so the gain is small. Kept as the next step if measured sweeps still hurt writes after this ADR.
- A separate metrics database file. It would isolate the cap and the WAL from logs, but adds a second file to back up, restore and migrate. The per table cap gets most of the benefit; revisit if logs and metrics still fight over checkpoints.
- An external time series database. It adds a service to every install, against the single binary decision (ADR 001).
- Storing only averages per bucket. That is the current read bug, and it makes correct merging impossible.

## Security

No new endpoints. Settings go through the existing runtime settings API with its admin authorization and range checks, so only an admin can change or disable the cap. The read pool opens the same file with `query_only` so a bug in a read path cannot write. Series keys come from resource ids the control plane owns, not from user input; metric names in queries keep using bound parameters. Backfill and rollup log counts only, never sample values or resource names beyond what logs already carry.

## What cannot be verified locally

- Sizes per row after the dictionary change (40 B and 65 B are estimates until measured with the existing growth harness).
- Sweep and rollup timing on a real VPS disk with fsync cost, many nodes, and remote agents delivering samples late.
- `dbstat` availability in every shipped build (the code paths already guard for its absence).
- Backfill time on a real 1 GiB telemetry file with months of hour rows.
- How the web chart renders gaps until it opts in to `fill=gaps`.

## Testing plan

- Unit: rollup algebra per kind, including a cumulative series with a reset inside a bucket and across a bucket edge; avg from sum and count against a brute force over raw.
- Seam: a range that covers 1 d, 1 h, 5 m and raw tail with the watermarks at different points, overlapping retention windows, checked against the same range computed from raw only (avg, min, max, count equal within float tolerance).
- Idempotency: run rollup twice and after a simulated crash between buckets; rows equal.
- Concurrency: a reader on the read pool during a full prune of 336k rows has max wait under the read timeout (target under 100 ms), using the existing growth harness, skipped under `-short`.
- Cap: logs table filled past the cap does not cause metric prune; metric bytes end under the target percent.
- Migration: a fixture DB in the old shape, backfill, cutover, compare charts before and after for gauges and deltas; cumulative within the stated limit.
- Settings: invalid tier orderings rejected, live change applies on the next run.

## Build slices

Order is strict where noted; files listed are owned by that slice only.

| # | Slice | Files | Review |
| --- | --- | --- | --- |
| 1 | Telemetry read pool and read timeout, checkpoint and journal size limit after prune | `internal/telemetry/db.go`, new `internal/telemetry/readpool.go`, read call sites in `internal/telemetry/tiers.go` and `query_many.go`, `internal/runtimesettings/registry_telemetry_reads.go` (new) | lr-reviewer |
| 2 | Fix query merging: rollup rows carry sum, min, max, count; `Aggregate` uses the count algebra; seam test with overlapping windows | `internal/telemetry/tiers.go`, `internal/telemetry/federate.go`, `internal/api/metrics.go`, tests | lr-reviewer |
| 3 | Metric kinds table and cumulative increase with reset handling in rollup (still on current tables) | new `internal/telemetry/metrickind.go`, `internal/telemetry/rollup.go`, `red.go` (IsSumMetric moves into kinds) | lr-reviewer |
| 4 | Tier settings as live runtime settings with ordering validation, new defaults, rollup and prune intervals and budget from settings | `internal/telemetry/tiers.go` (config source), new `internal/runtimesettings/registry_metrics_tiers.go`, `cmd/levelrail/request_metrics.go`, `cmd/levelrail/boot_storage_settings.go` | no |
| 5 | Schema and tier change: dictionary, new tables, 5 m, 1 h, 1 d tiers, one bucket per transaction with pause and budget (after 3 and 4) | new `internal/telemetry/migrations/0012_metric_series.sql`, `internal/telemetry/rollup.go`, `store.go`, `tiers.go` | lr-reviewer |
| 6 | Backfill job and cutover flag from old tables (after 5) | new `internal/telemetry/metrics_backfill.go`, `cmd/levelrail/request_metrics.go` | lr-reviewer |
| 7 | Cap on metric tables via dbstat with fallback, headroom target, over cap warning event | `internal/telemetry/rollup.go` (Prune, liveBytes), `internal/telemetry/compact.go` | no |
| 8 | Gap fill behind `fill=gaps`, web chart opt in, settings page size estimate, docs | `internal/api/metrics.go`, `internal/telemetry/federate.go`, web metrics chart and settings files, `docs/known-limits.md` | no |

Slices 1, 2 and 7 help on their own and can ship first; slice 1 removes the measured stall.

## Consequences

- At 200 apps metrics settle near 270 MB instead of hitting 1 GiB in a day, and history depth follows the settings, not the cap.
- Raw detail drops from 15 d to 6 h by default; older views show 5 minute or coarser points with true min and max.
- Charts that cross a tier seam become correct, which can change how existing charts look (max lines that were maxima of averages become true maxima, so they rise).
- Network and disk charts change from running totals to increase per step. Anything reading the old shape (alerts on these series, autoscale inputs) must be checked in slice 3.
- More settings to explain; the settings page needs the size estimate to make them usable.

## Open questions

1. Is 6 h raw acceptable as the default, or is 24 h worth about 240 MB more at 200 apps?
2. Do any alert rules, autoscale inputs or exports read network or disk totals directly and expect a running total?
3. Should day buckets follow UTC or the server's time zone?
4. Can remote agents deliver samples later than the rollup lag, and if so should lag scale with node latency?
5. Is the release that drops the old tables the next minor or two releases out, given the rollback gap it ends?
6. Should request series (`http_*` and latency slots) share these tiers and the same cap, or get their own budget, since they can outnumber container series?
