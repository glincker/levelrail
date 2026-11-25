---
title: "Collapse repeated log lines"
description: Fold runs of identical log lines into one row with a count and time span, in the API, the CLI and the dashboard, without changing what is stored.
"og:url": "https://docs.levelrail.com/log-collapse"
---

A crash loop or a retry storm can print the same line thousands of times and bury the one line that matters. Collapse folds each run of identical consecutive lines into a single entry. It happens when the logs are read. Nothing is dropped at write time, so the raw lines are always still there.

Collapse is off by default.

## What counts as a repeat

Consecutive lines with the same stream, container and message. A different line in between ends the run, so order is never rearranged.

A collapsed entry carries the first line's timestamp plus three fields: `count`, `first` and `last`. Entries that did not repeat look the same as before.

## API

Add `collapse=1` to the app logs query.

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$URL/api/v1/apps/web/logs?collapse=1&from=2026-01-01T00:00:00Z&to=2026-01-01T01:00:00Z"
```

`total` still counts the raw lines that matched. `limit` applies to the collapsed entries.

To reach the raw lines behind an entry, repeat the same query without `collapse` and set `from` and `to` to the entry's `first` and `last`.

## CLI

```bash
levelrail-cli apps logs web --since 1h --collapse
```

A repeated line prints as `... retrying (x240 until 2026-01-01T00:10:00Z)`. `--collapse` cannot be combined with `--follow`.

## Dashboard

On an app's **Logs** page, press **Collapse** next to **Wrap**. A repeated line shows a count badge, and hovering it shows the first and last time. Press the button again to see every raw line.
