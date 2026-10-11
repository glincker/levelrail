---
description: Save and share log searches, group similar errors into signatures, spot new error types after a deploy and alert on log patterns, from the dashboard, the CLI or the API.
---

# Log explorer

Logs are stored on the node that runs the container and searched from the control plane. The log explorer makes those searches reusable and turns them into alerts, without shipping logs anywhere.

<InlineToc default-open />

## Saved queries

A saved query is a named search: text, minimum level, stream, field filters (`status>=500`, `user~alice`) and a relative range such as `1h`.

- **Private** queries are visible only to you. **Shared** queries are visible to everyone who can see the app, and sharing needs write access.
- **Pinned** queries sit at the top of that app's menu.
- **Recent** searches are remembered in your browser only.
- A permalink (the Search tab's "Copy permalink") carries the filters and an absolute time range.

```bash
levelrail-cli logs saved create "5xx on checkout" --app web --q checkout --field 'status>=500' --range 6h --share
levelrail-cli logs saved list --app web
levelrail-cli logs saved run slq_AbC123
levelrail-cli logs saved delete slq_AbC123
```

API: `GET/POST /api/v1/logs/queries`, `PUT/DELETE /api/v1/logs/queries/{id}`. At most `APP_LOG_SAVED_QUERY_MAX` (default 100) per user.

## Error signatures

Lines that differ only in numbers, ids, UUIDs, hex, IP addresses, emails or timestamps share one signature. `ERROR upstream 10.0.0.4 timed out after 301ms` and `ERROR upstream 10.0.0.9 timed out after 87ms` become `upstream <ip> timed out after <num>`.

The Errors tab on an app's Logs page lists the top signatures for a range with the count, first and last seen, a sparkline and a sample line. Marked **New** are signatures that did not occur in the window of equal length just before the range. "View lines" opens the Search tab filtered to that signature. "Alert on this" starts a rule from it.

```bash
levelrail-cli logs signatures web --since 24h --new
```

API: `GET /api/v1/apps/{name}/logs/signatures?from&to&level=error&baseline=true`. MCP: the read tool `log_signatures`.

### New error types after a deploy

The deploy page and the "what changed" timeline show "3 new error types since this deploy" for the newest deploy that took effect: signatures that appeared after it and were absent in the same span before it. The same note is attached to alert notifications that list recent changes.

## Log alert rules

Two rule kinds are evaluated from the local log store, once a minute by default:

| Kind | Fires when |
| --- | --- |
| `log_pattern` | at least `count` lines match the text, regex and field filters inside the window |
| `log_level_rate` | lines at or above the level (default `error`) arrive at `per_minute` or faster over the window |

Options: `window_seconds` (default 300), `cooldown_seconds` (no re-fire for that long after a firing), `for_duration` (the condition must hold that long) and the usual severity, flap and silence controls. Notifications carry the count or rate and up to five matching sample lines with credentials masked.

```bash
levelrail-cli logs alert-from web --name "checkout timeouts" --q timeout --regex 'checkout|payment' --count 10 --window 5m --cooldown 15m --channel ch_123
levelrail-cli logs alert-from web --name "error rate" --per-minute 30 --level error
```

Create one from a search with "Alert on this" on the Search tab, or from a signature on the Errors tab. Log rules are created and deleted, not edited in place: delete and recreate to change one.

### Limits

Evaluation never scans an unbounded range. Each run reads the newest lines of at most the window, up to `APP_LOG_RULE_MAX_ROWS` rows through the `(resource, timestamp)` index. If the cap is hit, a rate rule is computed over the span actually read and the notice says so. A `count` above the cap is rejected when the rule is saved.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_LOG_RULE_EVAL_INTERVAL` | `1m` | how often each log rule runs |
| `APP_LOG_RULE_MAX_WINDOW` | `1h` | longest look-back a rule may use |
| `APP_LOG_RULE_MAX_ROWS` | `5000` | rows read per evaluation |
| `APP_LOG_RULE_SAMPLES` | `5` | sample lines attached to a notification |
| `APP_LOG_SIGNATURE_MAX_ROWS` | `20000` | rows read per signature report |
| `APP_LOG_EXPORT_MAX_LINES` | `5000` | lines in one download |
| `APP_LOG_SAVED_QUERY_MAX` | `100` | saved queries per user |

## Viewer

The Search tab virtualizes the list, colours by level and has a Wrap toggle. Click a line (or press `j` and `k` to move, `Enter` to open) for pretty JSON, a chip per field that adds a filter on click, and the lines logged just before and after it. "Download logs" exports the current filters, up to `APP_LOG_EXPORT_MAX_LINES` newest lines, with credentials masked.

| Key | Action |
| --- | --- |
| `/` | focus the search box |
| `w` | toggle wrapping |
| `j` / `k` | next / previous line |
| `Esc` | close the detail |

## Safety

- Saved queries respect app visibility: a query scoped to an app you cannot see is never listed, and only the owner (or a root user) can change or delete a shared one.
- Samples in alerts, signature reports and downloads mask `password=`, `token=`, `api_key=`, bearer tokens, URL passwords and common key shapes. The live viewer shows lines as stored.
- Rule and signature reads are capped by the variables above, so a noisy app cannot make a rule slow the control plane.
