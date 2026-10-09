---
description: Browse schemas and table data, run read-only SQL, and inspect Redis keys for a managed database from the dashboard or the CLI.
---

# Database viewer

Look inside a managed database without opening a port or handing out its
password. The dashboard's Explorer and SQL Console tabs, the CLI, and the API
all go through the control plane, which runs the engine's own client inside
the database container. Credentials never reach the browser and are never
logged.

Supported engines: Postgres, MySQL, and MariaDB (schema, table data, SQL),
and Redis, KeyDB, and Dragonfly (read-only key browser). MongoDB and
ClickHouse are not supported yet.

## Explorer

Open a database and choose **Explorer**.

- Schemas, tables, and views with a row estimate and total size.
- Columns with type, nullability, default, and primary key; indexes with
  their definitions.
- A paginated data grid (100 rows per page) with column sorting and a
  column filter (contains, equals, is null, is not null). Long values are
  truncated in the grid and open in full when clicked.

For Redis-compatible engines the Explorer is a key browser: pattern scan with
`SCAN`, then each key's type, TTL, and value. It cannot write.

## SQL console

Open **SQL Console**. Statements run read-only by default:

- One statement per request. Multiple statements, backslash commands, and
  `COPY` are rejected.
- The statement runs in a read-only transaction, with a statement timeout, a
  row limit, and a result size cap.
- `INSERT`, `UPDATE`, `DELETE`, DDL, `SELECT INTO`, data-modifying CTEs, and
  functions that read server files or signal other sessions are rejected.
- **Explain** and **Explain analyze** show the query plan.
- History is kept per user and database. Statements can be saved by name.

### Allowing writes

An administrator (the `root` ability) can switch the console to write mode:
turn on **Allow writes**, then type the database name to confirm. Each
statement then runs in its own committed transaction and is recorded in the
audit log with its fingerprint (never its text or results). Write mode is off
again whenever the page is reloaded.

## CLI

```bash
levelrail-cli databases schema main --columns
levelrail-cli databases query main --sql "select id, email from users order by id desc limit 20"
levelrail-cli databases query main --file report.sql --json
levelrail-cli databases query main --sql "select * from orders where total > 100" --explain
levelrail-cli databases query main --sql "update users set plan = 'free' where id = 7" --write --confirm main
```

`db` works as a short alias for `databases`.

## Permissions

| Action | Ability |
| --- | --- |
| Schema, table data, key browser, read-only SQL, explain, history, saved queries | `read:sensitive` |
| Write mode | `root` |

A plain `read` token cannot see row data. IAM policies scoped to one database
apply as they do for the rest of the database routes.

## Limits

Defaults are tunable through environment variables on the control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DB_QUERY_TIMEOUT` | `15s` | Statement timeout. |
| `APP_DB_QUERY_MAX_ROWS` | `1000` | Rows returned per statement. |
| `APP_DB_QUERY_MAX_BYTES` | `4194304` | Result size cap in bytes. |
| `APP_DB_QUERY_MAX_CELL_BYTES` | `65536` | A single cell is cut at this size. |
| `APP_DB_QUERY_MAX_SQL_BYTES` | `65536` | Largest accepted statement. |
| `APP_DB_SCHEMA_MAX_ROWS` | `5000` | Tables listed by the schema browser. |
| `APP_DB_QUERY_HISTORY_KEEP` | `200` | History rows kept per user and database. |

## API

| Route | Purpose |
| --- | --- |
| `GET /api/v1/databases/{name}/schema` | Schemas, tables, columns, indexes. |
| `GET /api/v1/databases/{name}/tables/{schema}/{table}/rows` | One page of rows (`limit`, `offset`, `sort`, `dir`, `filter_column`, `filter_op`, `filter_value`). |
| `POST /api/v1/databases/{name}/query` | Read-only statement (`{"sql": "..."}`). |
| `POST /api/v1/databases/{name}/query/write` | Write statement (`{"sql": "...", "confirm": "<name>"}`), `root` only. |
| `POST /api/v1/databases/{name}/explain` | Query plan (`{"sql": "...", "analyze": false}`). |
| `GET /api/v1/databases/{name}/keys`, `GET /api/v1/databases/{name}/key?key=` | Redis-compatible key browser. |
| `GET`, `DELETE /api/v1/databases/{name}/query-history` | Your statement history. |
| `GET`, `POST /api/v1/databases/{name}/saved-queries`, `DELETE .../{id}` | Your saved statements. |

## Security notes

- The console connects over the container's local socket as the database's
  own admin role, so the read-only transaction and the statement guard are the
  protection, not a reduced-privilege role. Do not grant `read:sensitive` to
  principals you would not trust with the data.
- Statement text is stored in your history. Results are never stored or
  logged. Server logs carry only the database name, mode, a literal-free
  fingerprint, duration, and row count.
