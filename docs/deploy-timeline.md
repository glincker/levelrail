---
title: "Deploy timeline"
description: "The fleet deploy timeline shows every deploy across your apps as bars on one time axis, with live updates, filters, a hover card and a matching CLI command."
---

The Deploy timeline (`/deploy-timeline`) puts every deploy across every app you can read on one shared time axis, one row per app. Use it to see what changed across the fleet around a given moment, and which deploys failed or rolled back.

## Reading the chart

- **Bars** run from the deploy start to its finish. A running deploy extends to the right edge and pulses, unless your system asks for reduced motion.
- **Color** follows the outcome: green for ready, red for failed, blue for building, grey for rolled back, canceled and superseded, amber for held or awaiting approval.
- **Markers**: failed deploys carry a red cross and rollbacks a back arrow above the bar.
- **Hover or focus** a bar to see its commit, actor, duration, strategy, trigger and, for a rollback, which deploy it returns to. Select a bar to open that deploy's logs.

Rows are virtualized, so a fleet with hundreds of apps stays responsive. Use the Tab key to move between bars and Enter to open one.

## Zoom and filters

Switch between 1 hour, 24 hours and 7 days. Filter by app, project, node or status. The page loads one window at a time and offers **Load earlier deploys** when a window holds more than a page.

New and finishing deploys arrive over the same live stream as the [Deployments page](deployments-page.md) (server-sent events), so a running deploy turns green or red without a refresh. The pill in the header shows whether the stream is connecting, live or reconnecting.

Incident and alert markers are not drawn on this chart yet.

## CLI

```sh
levelrail apps deploys timeline --since 24h
levelrail apps deploys timeline --since 7d --status failed,rolled_back
levelrail apps deploys timeline --project PROJECT_ID --json
```

The text view draws one row per deploy, oldest first, with a bar placed on a shared scale: `=` ready, `x` failed, `~` in flight, `<` rolled back. `--json` prints the raw page.

## API

`GET /api/v1/deployments/timeline` returns compact deploy bars for a window, newest first.

| Parameter | Meaning |
| --- | --- |
| `since`, `until` | RFC 3339 time or a duration such as `24h` or `7d`. Defaults to the last 24 hours, at most 30 days wide. |
| `app`, `project`, `node` | Narrow to one app, project or node. |
| `status` | One or more statuses, comma separated. |
| `limit`, `cursor` | Page size (at most 200) and the `next_cursor` of the previous page. |

The response lists `items` with the deploy id, app, status, trigger, commit, author, strategy, start, finish, duration and rollback source, plus `next_cursor` when more rows exist. The first page (no `cursor`) also carries `alerts`, up to 200 fired alerts from the alert history in the same window, drawn as markers on each app lane. Later pages leave `alerts` empty. See the [API reference](api-reference.md).
