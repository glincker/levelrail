---
title: Audit log
description: Search the audit log by time, actor, method and status, verify its hash chain, and export what you filtered.
---

Every write, deploy and root-tier request is recorded, newest first. The log is free, tamper-evident and exportable.

## Filter

Dashboard: Settings, Audit log. The bar has a time range, method, actor, client, free text, failed only and agent chips. Every filter is kept in the page URL, so a filtered view can be shared as a link.

API: `GET /api/v1/audit-log` accepts `since` (inclusive) and `until` (exclusive) as RFC3339 timestamps, `actor` (exact name), `status=failed`, `method`, `path`, `client_kind`, `agent`, `action`, `q` and `limit`.

CLI:

```
levelrail-cli audit-log --since 24h --agent ci-bot --status failed
levelrail-cli audit-log --since 7d --actor alice --export csv --output-file audit.csv
```

`--since` and `--until` take a duration such as `24h` or `7d`, or an RFC3339 timestamp. Agents get the same filters from the `list_audit_log` tool.

## Export

The Export CSV button, `GET /api/v1/audit-log?format=csv` and `--export csv` all use the same filter parameters, so the file matches what you see.

## Verify the chain

Each row carries a hash of its content and the previous row. Verify re-computes the chain and names the first row that was edited, removed or cut off.

- Dashboard: the Verify chain button opens a sheet with an optional entry range, the number of rows verified, the head hash and the first break.
- API: `GET /api/v1/audit-log/verify?from=SEQ&to=SEQ`. The whole chain is walked, and the report covers the range.
- CLI: `levelrail-cli audit-verify --range 100-200` (either side may be empty). Exit code 1 on a break.
- Agents: the `verify_audit_log` tool takes optional `from` and `to`.
