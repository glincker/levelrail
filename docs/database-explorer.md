---
description: How the database explorer lists tables, what its empty and error states mean, and what the dashboard shows for databases, reserved resources and storage.
---

# Database explorer and fleet overview

## Explorer

The explorer (Beta) reads schemas through a short-lived helper that runs inside the database container. It is read-only and subject to the same row, cell and time limits as the SQL console.

- The table list loads as an outline: names, row estimates and sizes. Columns, indexes, keys and the definition load when a table is opened.
- Search matches every word against `schema.table`. The last opened table is remembered per database in the browser.
- The data view filters per column in the database. Type text for "contains", `=text` for an exact match, `is:null` or `not:null`.
- Press Enter on the grid to open a row in the detail drawer. JSON is pretty printed. Space selects a row, Escape clears the selection.
- Copy as CSV, JSON or INSERT applies to the selected rows, or all loaded rows when none are selected.
- Relationships shows one hop of foreign keys. Every box opens that table.

### Empty and error states

The explorer never shows an empty database when the read failed.

| State | What you see |
| --- | --- |
| Loading | A skeleton and "Reading the schema from the database". |
| Could not read | The reason in plain words, Retry, and the raw error behind a details toggle. |
| Empty | Size on disk, schemas scanned, system schemas excluded, table count and the time of the check. |
| No search match | The database has tables; only your search filtered them out. |

A database that was created as a migration target also shows its copy status. A copy marked verified that has no tables is flagged as an error.

## Dashboard

- Databases: count, healthy, unhealthy, stopped, engines, total size, largest and busiest.
- Reserved versus used: configured CPU and memory limits (replicas included) against node capacity and current usage. A resource without a limit is counted separately and never guessed.
- Storage: named volume sizes on the control plane node and stored backup bytes.

Over-commit is only reported when every node reported its capacity. CPU capacity is the core count of the control plane node. The idle threshold can be changed with `VITE_RESERVED_IDLE_RATIO` at build time.
