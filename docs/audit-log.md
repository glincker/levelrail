---
title: Audit log
description: Search the audit log by time, actor, method and status, verify its hash chain, and export what you filtered.
---

Every write, deploy and root-tier request is recorded, newest first. The log is free, tamper-evident and exportable.

## Filter

Dashboard: Settings, Audit log. The bar has a time range, method, actor, client, free text, failed only and agent chips. Every filter is kept in the page URL, so a filtered view can be shared as a link. A time range is pinned when you pick it, so the list, paging and export all use the same window.

API: `GET /api/v1/audit-log` accepts `since` (inclusive) and `until` (exclusive) as RFC3339 timestamps, `actor` (exact name), `status=failed`, `method`, `path`, `client_kind`, `agent`, `action`, `q` and `limit` (default 50, maximum 200).

CLI:

```
levelrail-cli audit-log --since 24h --agent ci-bot --status failed
levelrail-cli audit-log --since 7d --actor alice --export csv --output-file audit.csv
```

`--since` and `--until` take a duration such as `24h` or `7d`, or an RFC3339 timestamp. The `list_audit_log` agent tool takes RFC3339 timestamps only.

## Export

The Export CSV button, `GET /api/v1/audit-log?format=csv` and `--export csv` use the same filter parameters. A single API request or dashboard export returns at most 200 rows (the page size); when more match, the response carries the header `X-Audit-Truncated: true`. The CLI export pages with `before` until the filter is exhausted, so it writes every matching row. Use it for complete exports.

## Verify the chain

Each row carries a hash of its content and the previous row. Verify re-computes the chain and names the first row that was edited, removed or cut off.

- Dashboard: the Verify chain button opens a sheet with an optional entry range, the number of rows verified, the real chain head and the range end hash, and a warning when the chain is broken outside the range.
- API: `GET /api/v1/audit-log/verify?from=SEQ&to=SEQ`. Without a range it reports the whole chain. `to=0` or no `to` means open ended. The whole chain is always walked: `ok` covers the requested range, `chain_ok` the whole chain, and `problems_outside_range` counts breaks elsewhere. `head` and `head_hash` are the top of the chain; `range_end` and `range_end_hash` are where the range stopped. A range that reaches past the head, covers removed rows or ends on a removed row is not ok.
- CLI: `levelrail-cli audit-verify --range 100-200` (either side may be empty). It prints `range 100-200 intact` only for that range, warns when the chain is broken elsewhere, and exits 1 on any break.
- Agents: the `verify_audit_log` tool takes optional `from` and `to` and returns the same report. Root token.
