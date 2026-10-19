---
title: "Log explorer"
description: Search logs, save queries, group errors into signatures, and create alert rules.
---

Logs are stored on the node that runs the container and searched from the control plane. The log explorer makes those searches reusable and turns them into alerts, without shipping logs anywhere.

## Saved queries

A saved query is a named search: text, minimum level, stream, field filters (`status>=500`, `user~alice`) and a relative range such as `1h`.

- Private queries are visible only to you. Shared queries are visible to everyone who can see the app, and sharing needs write access.
- Pinned queries sit at the top of that app's menu.
- Recent searches are remembered in your browser only.
- A permalink (the Search tab's "Copy permalink") carries the filters and an absolute time range.

```bash
levelrail-cli logs saved create "5xx on checkout" --app web --q checkout --field 'status>=500' --range 6h --share
levelrail-cli logs saved list --app web
levelrail-cli logs saved run slq_AbC123
levelrail-cli logs saved delete slq_AbC123
```

The Saved menu is on every tab of an app's Logs page. Picking a query from Live, Errors or Archive opens it in Search with its filters and range applied.

API: `GET/POST /api/v1/logs/queries`, `PUT/DELETE /api/v1/logs/queries/{id}`. At most `APP_LOG_SAVED_QUERY_MAX` (default 100) per user.

## Search syntax

The search box and `apps logs --q` take an expression, not just a phrase.

| Write | Matches |
| --- | --- |
| `timeout upstream` | lines with both words |
| `"connection refused"` | the exact phrase |
| `conn*` | words that start with `conn` |
| `error OR panic` | either word (`OR`, `AND` and `NOT` are upper case) |
| `error NOT timeout` or `error -timeout` | `error` without `timeout` |
| `(refused OR reset) upstream` | grouping with parentheses |

A malformed expression, such as an unclosed quote or a trailing `OR`, is refused with a message saying what to fix. Log alert rules use the same syntax and are checked when you save them.

## Error signatures

Lines that differ only in numbers, ids, UUIDs, hex, IP addresses, emails or timestamps share one signature. `ERROR upstream 10.0.0.4 timed out after 301ms` and `ERROR upstream 10.0.0.9 timed out after 87ms` become `upstream <ip> timed out after <num>`.

The Errors tab on an app's Logs page lists the top signatures for a range with the count, first and last seen, a sparkline and a sample line. Signatures marked **New** did not occur in the window of equal length just before the range. "View lines" opens the Search tab filtered to that signature. "Alert on this" starts a rule from it.

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

## Relevance or newest

With a search phrase, the Search tab can order matches by **Relevance** (BM25: lines where the terms are frequent and the line is short rank first, words under `NOT` never score) instead of **Newest**. The same choice is `GET /api/v1/apps/{name}/logs?order=relevance` and `levelrail-cli logs query <app> --text PHRASE --order relevance`. The limit then keeps the best matches rather than the newest ones, and `total` still counts every match. Scores compare lines within one result, not across searches.

## Viewer

The Search tab virtualizes the list, colours by level and has a Wrap toggle. Click a line (or press `j` and `k` to move, `Enter` to open) for pretty JSON, a chip per field that adds a filter on click, and the lines logged just before and after it. "Download logs" exports the current filters, up to `APP_LOG_EXPORT_MAX_LINES` newest lines, with credentials masked.

| Key | Action |
| --- | --- |
| `/` | focus the search box |
| `w` | toggle wrapping |
| `j` / `k` | next / previous line |
| `Esc` | close the detail |

The Live tab never blocks on a slow browser: when it falls behind, lines are skipped for that viewer and a marker line says how many ("skipped N lines while the viewer fell behind"). The skipped lines are still stored and searchable.

## Safety

- Saved queries respect app visibility: a query scoped to an app you cannot see is never listed, and only the owner (or a root user) can change or delete a shared one.
- Samples in alerts, signature reports and downloads mask `password=`, `token=`, `api_key=`, bearer tokens, URL passwords and common key shapes. The live viewer shows lines as stored.
- Rule and signature reads are capped by the variables above, so a noisy app cannot make a rule slow the control plane.

## Troubleshooting

### Saved query not appearing or deleted unexpectedly

Likely cause: The query is private to another user, or you lost write access to the app.

How to confirm: Check if the query still exists: `levelrail-cli logs saved list --app <app> | grep <query-name>`. If it is missing, it was deleted. If it was shared, only the owner and root can modify it.

Fix: If it was deleted by mistake, recreate it: `levelrail-cli logs saved create "<name>" --app <app> --q "<search>" --range 1h`. Make sure you have write access to the app to create or modify queries.

Still stuck: Check the audit log for deletion events: `levelrail-cli audit-log --resource <app> | grep "saved.*delete"`.

### Log alert rule firing too often or not at all

Likely cause: The window or threshold is wrong, or the log stream is delayed.

How to confirm: Test the rule manually: `levelrail-cli apps logs <app> --since 10m --q "<pattern>"` and count the lines. Compare to the rule's `count` or `per_minute` threshold. Check when the rule was last evaluated: `levelrail-cli logs alert-from <app> --id <rule-id> --json | jq .last_fired_at`.

Fix: Adjust the threshold or window: delete and recreate the rule with better parameters. For a pattern rule firing too often, increase the `count` threshold. For a rate rule, increase `per_minute`. To reduce noise from oscillating alerts, increase the cooldown: `--cooldown 30m`.

Still stuck: Check control plane logs for rule evaluation errors: `levelrail logs --since 1h | grep "alert\|log.*rule"`.

### Cannot find error signature for a specific error message

Likely cause: The error is not being logged at the required level, or it has very high variance (many unique identifiers).

How to confirm: Query the logs directly: `levelrail-cli apps logs <app> --q "<error text>" --level error --since 1h`. If it does not appear, the log line exists but is at a different level (check with lower level like `--level warn`). If it appears but with many different variations (different IDs or timestamps), the signature algorithm groups them separately.

Fix: If the error is at the wrong level, adjust the application to log it as an error. If the signature has too much variance, use an alert rule with a regex pattern instead: `levelrail-cli logs alert-from <app> --name "my error" --regex '<pattern>' --count 1`.

Still stuck: See [Error signatures](/log-explorer#error-signatures) for how the grouping algorithm works.

### Log search returning no results

Likely cause: The time range is outside the retention window, or the search text does not match any logs.

How to confirm: Try a broader search: `levelrail-cli apps logs <app> --since 24h` with no filters. If it returns logs, the issue is with your search text. If it returns nothing, logs may not exist in that range.

Fix: Verify the app is generating logs (run `levelrail-cli apps status <app>` to confirm it is running). Adjust the time range to a more recent window: `--since 1h`. Simplify the search text: try short keywords instead of exact phrases.

Still stuck: Check if logs are being stored: `levelrail logs <app> --level debug --since 1h | wc -l` to see the total count.

### Download logs showing "encrypted" or "masked" secrets

Likely cause: Sensitive data like passwords or API keys appear in the logs, and the system is masking them.

How to confirm: Check the downloaded file for lines like `password=***` or `token=***`. This is intentional security masking applied to exported logs.

Fix: Logs are automatically masked for common key shapes (password, api_key, token, bearer tokens). If a secret is still exposed, that pattern is not recognized. Update your application to log less sensitive information, or use environment variables and avoid printing them.

Still stuck: Report the pattern that should be masked (open a discussion on GitHub), and review the app's logging to avoid printing secrets.

## Next steps

<CardGroup cols={2}>
<Card title="Observability" href="/observability">

Metrics, dashboards, and alert rules.

</Card>
<Card title="Log archive" href="/log-archive">

Ship logs to storage for long-term retention.

</Card>
</CardGroup>
