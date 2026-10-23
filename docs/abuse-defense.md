---
title: "Ingress abuse defense"
description: "Per-app rate limit by client address, address and User-Agent deny lists, and a log only mode with a replay of recent traffic before you enforce, from the dashboard, CLI or API."
"og:url": "https://docs.levelrail.com/abuse-defense"
---

The abuse defense turns away clients that hit an app too hard or that you know are bad, before the request reaches the container. It is set per app, covers every proxied domain of the app, and starts in a mode that blocks nothing so you can see its effect first.

It sits in front of the per-domain [abuse shield](domain-traffic-controls) and the instance [firewall](firewall). Use the shield for per-domain concurrency and body limits, and this for a per-app rate with a safe rollout.

## What it checks

| Check | Result |
| --- | --- |
| Client address in a deny CIDR or equal to a denied IP | 403 |
| No `User-Agent` header, when required | 403 |
| `User-Agent` contains a denied substring (any case) | 403 |
| More than `rps` requests per second from one client address, beyond `burst` | 429 with `Retry-After` |

The deny checks run first, so a denied client never spends a rate token. The rate limit is a token bucket per client address: `rps` is the refill rate, `burst` the bucket size. A rate of 0 turns the limit off and leaves the lists.

## Modes

| Mode | Behavior |
| --- | --- |
| `off` | Nothing is checked. The default for an app with no policy. |
| `log_only` | Counts what would be blocked, by reason, and blocks nothing. |
| `enforce` | Turns matching requests away. |

Move from `log_only` to `enforce` once the replay shows nothing you want to keep would be blocked. The dashboard only offers enforcing after the replay for the current settings has loaded.

## Client address

The client address is the one the access log and firewall use: the TCP peer, unless it is a configured trusted proxy, in which case the forwarded header is read from the right, skipping trusted hops. A forwarded header from any other peer is ignored, so a client cannot dodge a limit or frame someone else by sending `X-Forwarded-For`. Behind a proxy that hides client addresses and with no trusted proxies set, the status shows `ClientIPHidden`: every visitor would share one limit.

## Status

Each policy reports a condition with a reason: `Enforcing`, `LogOnly`, `Off`, `NoRoutes` (the app has no proxied route, for example a static site or a stopped app), `ClientIPHidden`, `InvalidPolicy` or `Pending` (waiting for the next ingress pass).

## Fail open

The defense never takes an app down. If the policy store cannot be read, the stored policy is invalid, or evaluating a request panics, the request is served and a `fail_open` counter goes up. The dashboard and `apps abuse get` show it.

## Counters

Per app, per window, in the traffic view and metrics: `allowed`, `would_block` and `blocked` by reason (`rate_limit`, `deny_cidr`, `deny_user_agent`, `missing_user_agent`), and `fail_open`. Metric names start with `http_abuse_`.

## Replay before you enforce

The replay runs the last minutes of the control plane's in-memory access log through a draft policy and reports how many requests it would have blocked and which clients were hit most. It reads memory only and changes nothing. Client addresses and User-Agents are kept in memory for this and are never returned by the access log API or written to disk, so the replay covers only what the log still holds (a restart empties it) and needs the `read:sensitive` ability.

<Tabs>
<Tab title="Dashboard">

Open the app, then **Traffic**. The Abuse defense card shows the mode, the per-client rate (prefilled from the app's recent peak), the block lists and a live preview of what the draft would have blocked.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli apps abuse get web
levelrail-cli apps abuse set web --mode log_only --deny-agent python-requests,scrapy
levelrail-cli apps abuse dry-run web --rps 5 --minutes 30
levelrail-cli apps abuse set web --mode enforce
```

`set` starts from the saved policy, or from the suggested rate for an app with none, so a flag changes only its own field. `--deny-cidr none` clears a list.

</Tab>
<Tab title="API">

```bash
curl -H "Authorization: Bearer $APP_API_TOKEN" "$APP_API_URL/api/v1/apps/web/abuse"
curl -X PUT -H "Authorization: Bearer $APP_API_TOKEN" -d '{"mode":"log_only","rps":20,"burst":40}' "$APP_API_URL/api/v1/apps/web/abuse"
curl -X POST -H "Authorization: Bearer $APP_API_TOKEN" -d '{"minutes":15,"policy":{"mode":"enforce","rps":5}}' "$APP_API_URL/api/v1/apps/web/abuse/dry-run"
```

Reading needs `read`; changing the policy needs `write:sensitive` (root, or a policy granting it on the app) and is audited. The replay needs `read:sensitive`.

</Tab>
</Tabs>

## Limits and tuning

All caps and defaults come from the environment, never from code. Every variable is optional.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_ABUSE_MAX_RPS` | 10000 | Highest `rps` a policy may set |
| `APP_ABUSE_MAX_BURST` | 100000 | Highest `burst` |
| `APP_ABUSE_MAX_DENY_ENTRIES` | 256 | Addresses and ranges per policy |
| `APP_ABUSE_MAX_AGENT_ENTRIES` | 64 | User-Agent substrings per policy |
| `APP_ABUSE_DEFAULT_RPS` / `APP_ABUSE_DEFAULT_BURST` | 20 / 40 | Suggested start without traffic data |
| `APP_ABUSE_MAX_TRACKED_CLIENTS` | 100000 | Client addresses tracked per app |
| `APP_ABUSE_SHARDS` | 64 | Lock shards per app limiter |
| `APP_ABUSE_IDLE_EVICT` | 5m | Drop a client's bucket after this idle time |
| `APP_ABUSE_MAX_RETRY_AFTER` | 1m | Cap on the `Retry-After` value |
| `APP_ABUSE_DRYRUN_DEFAULT_WINDOW` / `APP_ABUSE_DRYRUN_MAX_WINDOW` | 15m / 1h | Replay window |
| `APP_ABUSE_DRYRUN_TOP_CLIENTS` | 10 | Clients listed in a replay |
| `APP_ABUSE_SUGGEST_CLIENT_SHARE` | 0.25 | Share of the app's peak rate one client may use in the suggestion |
| `APP_ABUSE_SUGGEST_BURST_SECONDS` | 2 | Seconds of rate the suggested burst holds |

Memory is bounded by `APP_ABUSE_MAX_TRACKED_CLIENTS` per app. When a shard is full, an arbitrary tracked client is dropped, so a flood of distinct addresses cannot grow memory. Static sites served directly by the ingress are not covered yet.
