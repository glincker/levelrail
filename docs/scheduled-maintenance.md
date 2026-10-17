---
title: "Scheduled maintenance"
description: Announce planned downtime ahead of time with a dashboard banner, a status page entry and a channel notice, and optionally hold automatic deploys for the window.
---

A maintenance window is a one-off period you plan in advance. Users see it coming, the public status page lists it as scheduled maintenance, and your notification channels hear about it once, a lead time you choose before it starts.

This is different from [alert maintenance windows](/observability), which are recurring cron silences that mute alerts. A scheduled maintenance window announces and, if you ask, freezes deploys. It does not mute alerts. Domain maintenance mode is the exception: turning it on also creates its own alert silence for down, DNS, certificate and request error or latency alerts only (CPU, memory, disk, restart and crash alerts still fire), see [maintenance mode](/domains-and-ingress#maintenance-mode).

## What a window does

| When | What happens |
| --- | --- |
| Up to 24 hours before the start (or your announce lead time if longer) | A banner appears at the top of the dashboard. It can be dismissed. |
| At the announce lead time | One notice goes to every enabled notification channel. |
| During the window | The banner turns into "maintenance in progress" and cannot be dismissed. With **all** scope, every status page component shows as under maintenance. |
| Any time | The window is listed on the public status page, in the scheduled and in-progress maintenance section, then for 14 days after it ends. |
| Between start and end, if freeze is on | Automatic deploys are held, exactly as for a [deploy freeze](/deploy-safety). Manual deploys need an override with a reason. |

The status page shows only the title and message. The scope target (an app or project name) is never published.

## Schedule one

<Tabs>
<Tab title="Dashboard">

Open **Settings, Scheduled maintenance** and choose **Schedule maintenance**. Times are entered in your browser time zone and stored in UTC.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli maintenance create \
  --title "Database storage upgrade" \
  --start 2026-11-01T02:00:00Z --duration 2h \
  --scope app --target web \
  --freeze --notify-before 1h \
  --message "Writes are paused for about 10 minutes."

levelrail-cli maintenance list
levelrail-cli maintenance delete mw_0123456789abcdef
```

</Tab>
</Tabs>

### Fields

| Field | Meaning |
| --- | --- |
| `title` | Up to 120 characters. Shown everywhere. |
| `message` | Optional details, up to 2000 bytes. |
| `scope` | `all`, `project` (target is a project id) or `app` (target is an app name). Scope decides which apps a freeze covers. |
| `starts_at`, `ends_at` | RFC 3339. A window lasts at most 30 days and must end in the future. |
| `freeze` | Hold automatic deploys for the window. |
| `announce_lead_minutes` | Minutes before the start to notify channels, 0 to 10080. 0 sends no notice. |

## API

| Method and path | Ability | Purpose |
| --- | --- | --- |
| `GET /api/v1/scheduled-maintenance` | read | Windows from the last 7 days onward. `?current=true` returns only what the banner shows. |
| `POST /api/v1/scheduled-maintenance` | write:sensitive | Schedule a window. |
| `DELETE /api/v1/scheduled-maintenance/{id}` | write:sensitive | Cancel a window. A freeze ends with it. |

## Limits

- The banner is shown to everyone who can open the dashboard, labelled with its scope. It is not filtered per user.
- A cancelled window is removed, not kept in history.
- Putting a domain into [maintenance mode](/domains-and-ingress#maintenance-mode) with an end time creates a window for it. The domain stays in maintenance exactly as long as that window: when it ends, or you cancel it here, the domain's maintenance page goes away too.
- The lead-time notice is sent once. If no channel accepts it, it is retried every minute until the window ends.
- A domain in maintenance mode without an end time has no window. Use `--max-duration` or `APP_MAINTENANCE_MAX_DURATION` so it switches off on its own; see [maintenance mode](/domains-and-ingress#maintenance-mode).

## Windows created by domain maintenance

Domain maintenance mode links a window to itself so the status page and banner see it. With an end time the window is dated and ends maintenance. Without one the window is marked ongoing: the public status page shows it as in progress with no end, and it becomes completed the moment maintenance ends, whatever the cause. Ongoing windows have a placeholder end a year out that is never shown.
