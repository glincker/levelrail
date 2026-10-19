---
title: "Alert inbox"
description: An in-app inbox of the alerts that reached you, with per-account read state, routing rules and an optional daily digest, plus the guardrails on mass actions.
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

## Mass action guardrails

- `apps bulk` already supports `dry_run` and demands the exact app list to delete. Restart, stop and redeploy of more than 10 apps (`APP_BULK_CONFIRM_ABOVE`) now also need `confirm_count` equal to the target count, so a script cannot fan out by accident. The dashboard and CLI send it after you confirm the preview.
- `POST /api/v1/nodes/{id}/drain?dry_run=true` (and `levelrail-cli nodes drain <id> --dry-run`) lists the apps and databases a drain would move, without cordoning or moving anything.
- Per-app spend caps with threshold alerts, throttle and pause live under the app's Budget tab.
