---
title: "Get a ping when your Levelrail server stops"
icon: "/assets/icons/heart.svg"
sidebarTitle: "Heartbeat ping out"
description: Point the off-host heartbeat at a monitor you control, so you are alerted when the control plane stops pinging, even if it cannot send its own alerts.
"og:url": "https://docs.levelrail.com/tutorials/heartbeat-ping-out"
---

Levelrail sends its alerts from inside the control plane. If the server or the control plane dies, those alerts stop too. The off-host heartbeat fixes that by making the control plane call a URL you choose while it is healthy. A monitor outside the server alerts you when the calls stop.

This tutorial sets up the ping and the monitor. For what the heartbeat checks and what the failure ping says, read [Off-host heartbeat](/heartbeat). For the other outside checks, read [External monitoring](/external-monitoring).

## Before you start

- A running Levelrail instance with its dashboard open, and an admin login.
- A monitor service you control that accepts a dead man's switch ping: a URL it expects a request on at a set period. Any service with that feature works the same way.

## Step 1: Create the check in your monitor

Create a heartbeat or dead man's switch check in your monitor and copy its ping URL.

- Set the expected period a little longer than the heartbeat interval. The default interval is 60 seconds (`APP_HEARTBEAT_INTERVAL`, at least 5 seconds), so 2 to 3 minutes is a reasonable expected period.
- Set the grace period to a few minutes, so one slow ping does not page you.

## Step 2: Save the ping URL in Levelrail

In the dashboard, open **Settings, Notification channels**, and find the **Off-host heartbeat** card. Paste the ping URL and save.

The URL is stored encrypted. After you save, the card shows only its host, so the full URL is not visible again.

If the server sets `APP_HEARTBEAT_URL` instead, the card becomes read-only and shows that the URL comes from the environment. Change the environment variable and restart the control plane to change it.

## Step 3: Confirm the first ping arrives

Wait one interval, then check the monitor. Its last ping should be within the expected period.

The default ping is a POST with a small JSON body:

```json
{"status":"ok","version":"1.2.3","uptime_seconds":123}
```

The body holds no hostnames, app names or identifiers. Set `APP_HEARTBEAT_BODY=off` on the server to send no body at all.

## Step 4: Add a failure URL (optional)

When a check inside the control plane fails and a failure URL is set, the control plane calls the failure URL instead of the ping URL. The failure ping uses `"status":"degraded"` and adds a `reason` field, so your monitor can say which part failed.

If no failure URL is set, nothing is sent on failure, and the monitor alerts after its grace period. That is enough for a dead server, so the failure URL is optional.

The server reads it from `APP_HEARTBEAT_FAIL_URL`.

## Step 5: Test the alert (pending run)

Stopping the control plane on a test server is the only way to see the alert fire end to end. Do this on a server you can afford to restart:

1. Stop the control plane service.
2. Wait past the expected period and the grace period.
3. Check that your monitor sends the alert you configured.
4. Start the control plane and check that the pings resume.

This step has not been run on a real host yet, so treat the exact timing as pending run.

## What this does not cover

- An app that is down while the server is up. The heartbeat says the server is alive, not which app works. Keep app alerts, such as the crashloop alert in [Get alerts in ntfy](/tutorials/alerts-to-ntfy), next to it.
- A missing ping caused by a network path between the server and the monitor. The monitor cannot tell a dead server from a blocked route.

## Where to go next

- [Off-host heartbeat](/heartbeat): every setting and what each failure reason means.
- [Scheduled jobs](/scheduled-jobs#heartbeat-pings): a ping after each job finishes.
