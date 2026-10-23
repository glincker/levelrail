---
title: "Alert inbox"
description: An in-app inbox of the alerts that reached you, with per-account read state, routing rules and an optional daily digest, plus the guardrails on mass actions.
"og:url": "https://docs.levelrail.com/alert-inbox"
---

The alert inbox is the first tab of **Alerts**, and the bell in the header links to it with your unread count. It lists firings and resolutions newest first, built from the same alert history the History tab shows, so nothing is duplicated and nothing new is sent.

## What shows up by default

- Warning and critical firings, resolutions, flapping notices and grouped alert summaries.
- Hidden: silenced, inhibited, grouped member and rate limited entries, and info level alerts. They stay in the History tab.
- Only apps you can see appear.

Read state belongs to your account (or API token), so marking an item read does not clear it for anyone else. **Mark all read** moves a cursor forward instead of writing one row per item.

## Routing rules and the daily digest

Under the list, choose a minimum severity, a comma separated list of apps to mute, and optionally a daily digest: one plain text summary of the last 24 hours sent at the UTC hour you pick through any notification channel. A day with nothing to report sends nothing.

```bash
levelrail-cli alerts inbox list --unread
levelrail-cli alerts inbox read --all
levelrail-cli alerts inbox prefs --min-severity critical --mute noisy-app
levelrail-cli alerts inbox prefs --digest on --digest-hour 8 --digest-channel chn_abc
```

API: `GET /api/v1/inbox`, `GET /api/v1/inbox/unread-count`, `POST /api/v1/inbox/read` (`{"ids":[...]}` or `{"all":true}`), `GET` and `PUT /api/v1/inbox/preferences`. Reading needs the `read` ability; marking read and saving preferences need `write`.

## Keeping the inbox short

The Inbox page (`/inbox`) holds stored alert items plus items derived live from other tables: approval requests, failed deploys and failed database upgrades. Only stored alert items can be **deleted**. Derived items can only be **archived**, because they would otherwise come back while their source still exists.

- Per item: Archive (`e`), Delete (`#`) and Undo (`z`, or the toast button, kept for `VITE_INBOX_UNDO_SECONDS`, default 8). Delete hides the item from your inbox only; the alert History tab keeps the record.
- Bulk: tick rows (shift-click selects a range) or use select all for the current filter, then Archive, Mark read or Delete. Delete is disabled while any selected item is not a stored alert.
- Tidy menu: Archive all read, Archive all resolved, Clear resolved and Delete all archived. Destructive ones ask first and state the count.
- Repeats of one rule on the same target within `VITE_INBOX_GROUP_WINDOW_MINUTES` (default 60) show as one expandable row such as "Certificate expires within 1 days resolved, x14", with archive and delete for the whole group. Stored data is not changed.
- The header shows the item and unread counts, each filter tab shows its own count, and the list is virtualized and loads older pages as you scroll.

A fired alert counts as resolved once a later resolution exists for the same rule and target.

### Auto tidy

A sweep runs on the control plane and, for every account that has used the inbox, archives resolved alert items after a delay and removes archived stored alerts after a retention period. There is no settings screen for this: it is configured with environment variables only.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_INBOX_AUTO_ARCHIVE_RESOLVED_AFTER` | `30m` | Archive resolved items older than this. `0` turns it off. |
| `APP_INBOX_ARCHIVE_RETENTION` | `720h` | Remove archived stored alerts this long after they were archived. `0` keeps them. |
| `APP_INBOX_TIDY_INTERVAL` | `10m` | How often the sweep runs. |

Restoring a resolved item older than the delay lets the next sweep archive it again. Archived approvals, deploys and upgrades are never pruned.

```bash
levelrail-cli inbox archive --resolved --older-than 24h
levelrail-cli inbox archive --read
levelrail-cli inbox delete --id ID --yes
levelrail-cli inbox clear --archived --older-than 720h --yes
```

`delete` and `clear` refuse to run without `--yes` and print how many items they would remove. API: `POST /api/v1/inbox/delete` (`{"ids":[...],"restore":false}`) and `POST /api/v1/inbox/clear` (`{"action":"archive|delete","scope":"read|resolved|archived","older_than_seconds":0,"dry_run":false,"confirm_count":0}`). Deleting through `clear` needs `confirm_count` equal to the number of matching items, so run it with `dry_run` first. Both need the `write` ability, like archiving, and every delete, undo and clear writes an audit entry (`inbox.delete`, `inbox.restore`, `inbox.clear`).

## Mass action guardrails

- `apps bulk` already supports `dry_run` and demands the exact app list to delete. Restart, stop and redeploy of more than 10 apps (`APP_BULK_CONFIRM_ABOVE`) now also need `confirm_count` equal to the target count, so a script cannot fan out by accident. The dashboard and CLI send it after you confirm the preview.
- `POST /api/v1/nodes/{id}/drain?dry_run=true` (and `levelrail-cli nodes drain <id> --dry-run`) lists the apps and databases a drain would move, without cordoning or moving anything.
- Per-app spend caps with threshold alerts, throttle and pause live under the app's Budget tab.
