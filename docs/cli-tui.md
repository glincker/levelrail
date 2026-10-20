---
title: "Terminal UI"
description: Run Levelrail from the terminal with a live, keyboard-driven UI for apps, databases, deploys, nodes, domains, alerts and backups.
"og:url": "https://docs.levelrail.com/cli-tui"
---

`levelrail-cli tui` is a k9s-style terminal UI for operators who live in the shell. It shows the same data as the dashboard, refreshes on its own, tails logs, and can restart, deploy, roll back, stop and start an app, always behind a typed-name confirmation.

```bash
levelrail-cli tui                  # uses your default profile
levelrail-cli tui --profile prod   # a named profile
levelrail-cli tui --read-only      # every action is disabled
```

It uses the same credentials, profiles and `--team` / `--project` scope as every other command, and only the public HTTP API, so a token with read access is enough to browse. It needs a real terminal: in a script or pipe it exits with an error and points you at `apps list` and `attention` instead.

## Views

| Key | View | Shows |
| --- | --- | --- |
| `1` | Apps | name, status, replicas, image, node, domains |
| `2` | Databases | engine, version, node, state, backup schedule |
| `3` | Deploys | recent deployments across all apps, with trigger, commit, age, duration |
| `4` | Nodes | status, address, scheduling, last seen |
| `5` | Domains | domain, owning app, TLS owner, kind |
| `6` | Alerts | the attention feed, critical first |
| `7` | Backups | recent backup attempts across databases and volumes |

The header shows the profile, server host and version, mesh health (`mesh:healthy/peers`, or `off`), and the incident and attention counts. Status colours follow the dashboard tones: green is healthy, red is failing or critical, yellow needs a look, grey is neutral or stopped, cyan is in progress.

## Key map

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | move down / up |
| `g` / `G` | top / bottom |
| `ctrl-d` / `ctrl-u` | half page down / up |
| `1` to `7`, `tab`, `shift-tab` | switch view |
| `enter` | open the selected app or database (from Deploys and Alerts it opens the related app) |
| `esc`, `q` | back (`q` quits from a top-level view) |
| `/` | filter rows, `enter` keeps it, `esc` clears it |
| `:` | command bar |
| `l` | tail logs for the selected app or database |
| `r` | restart the app |
| `d` | redeploy the app's current image |
| `b` | roll back to the newest earlier succeeded deploy |
| `x` / `o` | stop / start the app |
| `?` | help |
| `ctrl-c` | quit |

In the logs view: `j`/`k` scroll, `g`/`G` jump to top and bottom, `f` toggles follow, `c` clears the buffer. Lines are coloured by level (error red, warn yellow, info cyan, debug grey).

## Command bar

Press `:` then one of the following. The app name is optional when an app is selected.

```
:deploy [app]    :restart [app]    :rollback [app]    :stop [app]    :start [app]
:logs [name]     :apps  :databases  :deploys  :nodes  :domains  :alerts  :backups
:help            :q
```

## Confirmation and read-only mode

Every action that changes state asks you to type the app name before it runs. A mismatch changes nothing, and `esc` cancels. `--read-only` removes the action keys and the mutating commands entirely and shows a `[READ-ONLY]` banner in the header.

Secrets, environment values and tokens are never fetched or shown in the TUI. To reveal a value, use the dashboard.

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `--read-only` | off | disable restart, deploy, rollback, stop and start |
| `--refresh DURATION` | `2s` | refresh interval, minimum `2s` (also `APP_TUI_REFRESH`) |
| `APP_TUI_LOG_BUFFER` | `2000` | log lines kept in memory per stream |
| `NO_COLOR` | unset | any non-empty value turns colours off |

Apps, deployments and alerts refresh every interval with one batched list call each. Databases, nodes, domains, backups, mesh and the server version refresh every fifth interval. A request that is still running is never started twice, and a section that fails keeps its last data and shows the error when it has nothing else to show.
