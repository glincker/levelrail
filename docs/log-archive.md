---
description: Ship node-local container logs to a connected storage destination as gzip-compressed NDJSON, on a schedule or on demand, and retrieve them by CLI or API.
---

# Log archive

Container logs are stored node-local (see [observability](/observability)) with a bounded retention window. Log archive ships them to a [storage destination](/object-storage) as gzip-compressed NDJSON, on a schedule you set per app or once for every app, so logs outlive the node-local store without needing a third-party log shipper.

A storage destination must exist first; see [object storage](/object-storage) for connecting one.

## What gets archived

Every line a container writes to stdout or stderr, exactly as the node-local log store received it. Objects are written to:

```
log-archive/<kind>/<name>/<yyyy>/<mm>/<dd>/<hh>/<start-ns>.ndjson.gz
```

for example `log-archive/service/web/2026/09/24/13/1790255400000000000.ndjson.gz`. The prefix can be changed with `APP_LOG_ARCHIVE_PREFIX`. Each line is a JSON object:

```json
{"resource":"service:web","stream":"stdout","ts":"2026-09-24T13:10:00.123Z","message":"request completed","structured":true,"fields":{"request completed":true}}
```

`fields` is only present when `message` parsed as a JSON object (`structured: true`); plain-text lines carry `message` only.

## How it behaves

- **Pull, not push.** A scheduler wakes once a minute (`APP_LOG_ARCHIVE_TICK`), checks which policies are due, and reads new lines from the node-local store. With no policies it does one small query per minute.
- **Idempotent and resumable.** Each policy keeps a watermark. Objects are named by their chunk start, so a retry after a failure overwrites the same key instead of duplicating lines. A failed window keeps the watermark in place and is retried on the next run.
- **Bounded.** One archive run at a time, at most 24 hour-windows per scheduled run, and at most 200,000 lines per object (`APP_LOG_ARCHIVE_MAX_LINES_PER_OBJECT`). Lines are spooled to a temp file first so the log database is never held open during an upload. A new policy archives forward from the moment it is created; use a dump for history.
- **Compaction-safe.** Objects are immutable and contiguous, and sort by name, so a later compaction job can merge them by prefix.
- **Every run is tracked.** Each scheduled tick or manual dump is recorded as a run with a status (`running`, `succeeded`, `failed`), the time range it covered, and object/line/byte counts, visible via `levelrail logs archive status` or `GET /api/v1/log-archive/runs`.

## Retention

Set "Keep for (days)" on a policy (0 means forever) to have the archiver delete objects older than that after each successful scheduled run, up to 500 deletions per run. The global policy (no app set) leaves apps that have their own policy alone, so a per-app retention setting always wins over the global one. If you would rather let the provider manage deletion, add a bucket lifecycle rule on the `log-archive/` prefix and leave retention at 0.

### Managing policies

Dashboard: an app's **Logs, Archive** tab, or **Settings, Storage destinations** for the global policy covering every app.

CLI:

```
levelrail logs archive set --target <id> --app web --interval 1h --retention-days 30
levelrail logs archive set --target <id> --interval 6h        # every app
levelrail logs archive status
levelrail logs archive remove --app web
```

Intervals run from 5 minutes to 24 hours (`PUT /api/v1/log-archive/policy`).

## Dump now

Archive a past range immediately, without waiting for the schedule:

```
levelrail logs dump --target <id> --app web --from 24h --wait
levelrail logs dump --target <id> --from 2026-09-24T00:00:00Z --to 2026-09-24T06:00:00Z
```

`--from` and `--to` take an RFC3339 timestamp or a duration back from now. The range is capped at 31 days. The dashboard has the same control on the Archive tab. Under the hood this is `POST /api/v1/log-archive/dump`, which answers `202 Accepted` with a run to poll (`GET /api/v1/log-archive/runs`) rather than blocking until the dump finishes.

## Retrieving archived logs

```
levelrail logs ls --target <id> --app web
levelrail logs fetch --target <id> --key log-archive/service/web/2026/09/24/13/1790255400000000000.ndjson.gz
```

Equivalently, `GET /api/v1/log-archive/objects?target_id=<id>&app=web` lists keys page by page (100 per page, follow `next`), and `GET /api/v1/log-archive/objects/download?target_id=<id>&key=<key>` streams the object back with `Content-Type: application/gzip`. Download is limited to keys under the archive prefix; a key outside it is rejected.

`zcat file.ndjson.gz | jq .` reads an object. There is no search index over archived objects on purpose; use the bucket provider's tooling (Athena, DuckDB, `zgrep`) for that.

## Alerting

Create a `log_archive_stale` alert rule (dashboard: Alerts, or `levelrail apps alerts create <app> --kind log_archive_stale`) to be notified when a policy's last run failed, or when it has gone longer than the rule's `for_duration` without a success (default: three intervals, at least two hours).

## API and MCP

REST routes live under `/api/v1/log-archive` (see the [API reference](/api-reference)). Reads need the `read` ability, policy changes and dumps need `write`. The MCP server exposes `list_log_archive_policies`, `set_log_archive_policy`, `start_log_archive_dump`, `list_log_archive_runs`, and `list_archived_logs`.
