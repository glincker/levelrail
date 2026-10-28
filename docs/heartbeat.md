# Off-host heartbeat

Every alert Levelrail sends comes from the control plane itself. If the box loses power, the disk fills, or the process is killed, nothing is left to send "I am down". The heartbeat closes that gap: while the control plane is healthy it calls a URL you choose every interval. A monitor outside your server alerts when the calls stop.

## What counts as healthy

A ping goes out only when all of these hold:

- the database answers,
- the reconcile loop completed a pass within `APP_HEARTBEAT_RECONCILE_WINDOWS` (default 3) resync intervals,
- the local agent is registered (skipped on a fresh install with no local node yet).

If a check fails the ping is held back and the monitor alerts after its grace period. If you also set a failure URL, that URL is called instead, so the monitor can alert at once and name the cause.

## Setup

Dashboard: Settings, Notification channels, "Off-host heartbeat". Paste the URL and save. The URL is stored encrypted and only its host is shown afterwards.

CLI:

```
levelrail-cli settings heartbeat set --url -        # reads the URL from stdin
levelrail-cli settings heartbeat show
levelrail-cli settings heartbeat clear
```

Env (takes precedence over the dashboard, the dashboard then becomes read-only):

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_HEARTBEAT_URL` | unset | ping URL, http or https |
| `APP_HEARTBEAT_FAIL_URL` | unset | called when the control plane knows it is unhealthy |
| `APP_HEARTBEAT_INTERVAL` | `60s` | time between pings, at least 5s |
| `APP_HEARTBEAT_TIMEOUT` | `10s` | per request timeout |
| `APP_HEARTBEAT_METHOD` | `POST` | `GET` or `POST` |
| `APP_HEARTBEAT_BODY` | on | `off` sends no body at all |
| `APP_HEARTBEAT_ALLOW_PRIVATE` | off | `1` allows loopback and private destinations (self-hosted monitors on your LAN) |

An invalid URL stops the server at boot with a clear message instead of silently never pinging.

## Pair it with a monitor

Healthchecks.io: create a check, set its period to your interval and its grace to a few minutes, and use its ping URL (`https://hc-ping.com/<uuid>`). For the failure URL use the same URL with `/fail` appended.

Uptime Kuma: add a Push monitor, set the heartbeat interval a little above `APP_HEARTBEAT_INTERVAL`, and use the push URL it shows. Older versions only accept GET on push URLs, so set `APP_HEARTBEAT_METHOD=GET`. For the failure URL use the same push URL with `status=down` in the query.

Better Stack and similar heartbeat monitors work the same way: copy the heartbeat URL, match the period.

Run Uptime Kuma on a different machine or provider than the server it watches. A monitor on the same box dies with it.

## What is sent

By default a small JSON body: `{"status":"ok","version":"...","uptime_seconds":123}`. A failure ping adds a one word `reason` (`store`, `reconcile` or `agent`). No hostnames, no app names, no identifiers. Set `APP_HEARTBEAT_BODY=off` to send nothing. Redirects to another host are refused, the URL never appears in logs or API responses (only its host), and private destinations are blocked unless you allow them.

## What it catches that in-box alerts cannot

Host power loss, kernel panic, OOM kill of the control plane, a full disk that stops the process, a cloud provider outage, a lost network route. It also catches a control plane that is running but wedged, because a stalled reconcile loop stops the pings.

It does not tell you which app is down. Keep your normal alerts for that; the heartbeat only answers "is the server itself alive".
