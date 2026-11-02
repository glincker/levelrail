---
title: "Send a crashloop alert to ntfy"
icon: "/assets/icons/bell.svg"
sidebarTitle: "Alerts to ntfy"
description: Connect an ntfy topic as a notification channel, alert when an app restarts in a loop, and confirm that the alert reaches your phone.
"og:url": "https://docs.levelrail.com/tutorials/alerts-to-ntfy"
---

An app that restarts in a loop is one of the most common first-week problems. This tutorial sends a crashloop alert to an [ntfy](https://ntfy.sh) topic, so the message reaches your phone. The same channel works for any alert kind in the [CLI reference](/cli-reference).

## Before you start

- A running Levelrail instance and the CLI logged in to it ([installing](/installing)).
- An app you want to watch. This tutorial uses `web`.
- An ntfy topic. You can use a public server or your own. If the topic needs a token, keep the token ready.

## Step 1: Build the ntfy URL

The URL is the ntfy server, the topic name, and the token in an `auth` query parameter:

```text
https://ntfy.example.com/levelrail-alerts?auth=TOKEN
```

Levelrail reads the token from the `auth` parameter and removes it from the URL before it calls the server. It sends the token in an `Authorization` header. The token is stored encrypted and is not shown again. If the topic is public, leave out `?auth=TOKEN`.

## Step 2: Create the channel and send a test message

```bash
levelrail-cli channels create --name ntfy-alerts --kind ntfy \
  --notify-url "https://ntfy.example.com/levelrail-alerts?auth=TOKEN"
levelrail-cli channels test <id>
```

Replace `<id>` with the id that `channels create` printed. The test sends a real message, so check your phone now, not after the first outage.

```bash
levelrail-cli channels deliveries <id>
```

`deliveries` lists the sends for this channel, newest first. A failed send shows its error here.

## Step 3: Create the crashloop alert

```bash
levelrail-cli apps alerts create web --name web-crashloop --kind crashloop \
  --restart-count-threshold 3 --restart-window 10m
levelrail-cli apps alerts list web
```

The alert fires when the app restarts at least 3 times within 10 minutes. The window format accepts a duration such as `10m`. Confirm that your version accepts it with `apps alerts create --help` (pending run on a real host).

## Step 4: Check that the alert reaches the channel (pending run)

Make the app fail on purpose in a test environment, for example by deploying an image whose command exits at once. Then:

1. Wait for the restart threshold to pass.
2. Check your phone for the alert.
3. Check `levelrail-cli channels deliveries <id>` for the send.

Which channels receive a crashloop alert, and whether the channel must be attached to the alert explicitly, has not been confirmed on a real host. Treat this step as pending run.

## Quiet hours

If the channel should stay silent overnight, set quiet hours on it.

```bash
levelrail-cli channels quiet-hours <id> --start 22:00 --end 07:00 --timezone UTC
levelrail-cli channels quiet-hours <id> --clear
```

Use quiet hours only for a channel that is not the only way you learn about a failure. A dead server does not raise a crashloop alert at all, so pair this tutorial with [Get a ping when your server stops](/tutorials/heartbeat-ping-out).

## Where to go next

- [Alert inbox](/alert-inbox): what the alerts look like once they fire.
- [Debug an app with logs and metrics](/tutorials/debug-an-app-with-logs-and-metrics): what to look at after a crashloop alert.
