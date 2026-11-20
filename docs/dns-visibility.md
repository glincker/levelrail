---
title: "DNS query visibility (beta)"
description: "See which app asks the mesh DNS server for which name, with per-app counters, a bounded in-memory log and three privacy levels. Off by default."
"og:url": "https://docs.levelrail.com/dns-visibility"
---

**Beta.** This feature is off by default and may change.

The control plane runs a small DNS server for the mesh (see [Connecting apps to databases](/connecting-apps-to-databases)). It answers for the mesh zone only, so app and database names resolve across nodes. DNS query visibility records what that server was asked, so you can tell which app looks up a name that does not exist, or which one is asking for names outside the zone.

It does not log what your apps ask their normal resolver for public names. It only sees queries that reach the mesh server.

## Turn it on

Four runtime settings control it. Each applies live, no restart. Under Settings, search for `dns.query_log`, or use the API or CLI settings commands.

| Setting | Default | What it does |
| --- | --- | --- |
| `dns.query_log_enabled` | `false` | Records queries and keeps per-app counters. |
| `dns.query_log_size` | `1000` | Most records kept in memory (10 to 100000). The oldest are dropped first. |
| `dns.query_log_privacy` | `names_only` | `full`, `names_only` or `counters_only`. |
| `dns.query_log_app_refresh` | `10s` | How often the container IP to app map is rebuilt. |

Each has an environment seed (`LEVELRAIL_DNS_QUERY_LOG_ENABLED`, `LEVELRAIL_DNS_QUERY_LOG_SIZE`, `LEVELRAIL_DNS_QUERY_LOG_PRIVACY`, `LEVELRAIL_DNS_QUERY_LOG_APP_REFRESH`) that only seeds the default while nothing is saved. A saved setting wins.

## Privacy levels

The level is applied before a record is stored, so lowering it never leaves older detail behind in new records.

- `full`: source IP, app, name, type, response code, answer count, latency and reason.
- `names_only`: the same without the source IP.
- `counters_only`: no records at all, only the per-app counters.

Answers are never logged beyond how many there were. Records live in memory only and are gone on restart.

## What a record holds

Each record has a time, the app the source container belongs to (empty when the source is not a known app container), the name, the query type, the response code, the answer count, latency in microseconds and a reason:

- `local_record`: answered from a mesh record.
- `no_such_app`: the name is in the mesh zone but nothing serves it.
- `no_data`: the name exists but has no record of the asked type (for example AAAA).
- `refused`: the name is outside the mesh zone.
- `format_error`: the query was malformed.

## CLI

```bash
levelrail-cli dns queries --app web --rcode NXDOMAIN --since 15m --limit 50
levelrail-cli dns stats
levelrail-cli dns queries --json
```

`--qname` takes a name prefix. `--since` takes a duration such as `15m` or an RFC 3339 time. The limit is capped at the log size ceiling.

## API

`GET /api/v1/dns/queries` takes `app`, `qname` (prefix), `rcode`, `since` and `limit`, and returns newest first. `GET /api/v1/dns/stats` returns counters per app and a total. Both need the read ability and report `enabled: false` while the setting is off. See the [API reference](/api-reference).

## Limits

- Queries from containers only get an app name when the mesh server is reachable from them, which today needs it bound to port 53.
- Counters reset on restart.
- This is visibility only. Nothing is blocked or rewritten.
