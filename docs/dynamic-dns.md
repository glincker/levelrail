---
title: "Dynamic DNS"
description: "Keep a DNS name pointed at a server whose public IPv4 changes. Configure it from the dashboard or the CLI, no restart needed."
"og:url": "https://docs.levelrail.com/dynamic-dns"
---

A home or small-office server often gets a new public IPv4 address when the router reconnects. The dynamic DNS updater checks the address on a timer and calls your DNS provider's update endpoint when it changes, so the name you use for [domains](/domains-and-ingress) keeps pointing at the server.

Status: written and unit tested. It has not yet been run against a real DuckDNS account or any other live provider, so treat every provider detail below as not yet verified.

## How it is configured

Levelrail is an infrastructure deployment, so this is a runtime setting. You manage it from the dashboard or the CLI, a stored value applies at once with no restart, and `show` prints the effective value and where it came from (`stored` or `default`). The `APP_DDNS_*` environment variables only seed the default while nothing is stored. Once you save a setting, the stored value always wins. See [Environment variables](/environment-variables#dynamic-dns).

## Set it up

Dashboard: Settings, Ingress, "Dynamic DNS" card.

CLI, DuckDNS:

```bash
levelrail-cli settings ddns set --provider duckdns --domains myserver --target -
levelrail-cli settings ddns enable
levelrail-cli settings ddns status
```

`--target -` reads the DuckDNS token from stdin. You can also export `LEVELRAIL_DDNS_TARGET` instead, which keeps the secret out of shell history.

CLI, generic update URL (the URL must contain `{ip}`):

```bash
levelrail-cli settings ddns set --provider generic --target -
levelrail-cli settings ddns enable
```

Paste a URL such as `https://dns.example.com/update?host=home&key=SECRET&ip={ip}` on stdin. The updater sends a GET request with `{ip}` replaced by the detected address. Any 2xx answer counts as success.

Other verbs:

```bash
levelrail-cli settings ddns show
levelrail-cli settings ddns disable
levelrail-cli settings ddns set --interval 10m --probe-quorum 2 --fail-alert-after 3
```

Tuning flags on `set`: `--interval`, `--timeout`, `--min-update-gap`, `--reassert` (durations such as `5m`), `--probe-urls` (comma separated), `--probe-quorum`, `--fail-alert-after`, `--allow-private on|off`, `--allow-insecure-http on|off`.

## Providers

| Provider | Target | Notes |
| --- | --- | --- |
| `generic` | an https update URL containing `{ip}` | GET request, any 2xx answer is success. |
| `duckdns` | your DuckDNS token, plus `--domains` | Calls the DuckDNS update endpoint and accepts an answer that starts with `OK`. |

Not built yet: a Cloudflare provider and IPv6 (AAAA) updates. The updater writes IPv4 only.

## How the address is found

Each check asks the probe URLs for the public address. The defaults are `https://api.ipify.org`, `https://icanhazip.com` and `https://ifconfig.me/ip`. A new address is accepted only when at least `probe-quorum` probes agree (default 2). If they disagree, or the answer is not a public address, the check is logged as a warning and nothing is written. This stops one bad probe from pointing your name at the wrong place.

The record is written when the address changes, but never more often than the minimum update gap. It is also written again after the reassert interval even if nothing changed, in case the provider lost the record.

## Defaults

| Setting | Default |
| --- | --- |
| interval | 5m |
| probe quorum | 2 |
| timeout | 10s |
| minimum update gap | 5m |
| reassert interval | 24h |
| fail alert after | 3 consecutive failures |
| allow private destinations | off |
| allow plain http | off |

After the configured number of consecutive failures the updater raises an alert, and clears it on the next success. `settings ddns status` shows the last check, last address, last result, failure count and last error.

## The target is secret

The update URL or token is stored encrypted and is never returned by the API, the CLI or the dashboard. After saving, only the host is shown. It needs the secrets store, so the master key must be set; without it the API answers that the feature is not available. The target is also masked in logs and error messages. The update URL must use https unless you turn on `allow-insecure-http`, and private or loopback destinations are refused unless you turn on `allow-private`.

Disabling detection with `APP_PUBLIC_IP_DETECT=off` also keeps the env default off. See [Environment variables](/environment-variables#dynamic-dns).

## API

| Method | Path | Meaning |
| --- | --- | --- |
| GET | `/api/v1/settings/ddns` | effective setting, source, whether a target is set, its host, whether the loop runs |
| PUT | `/api/v1/settings/ddns` | partial update, all fields optional, `target` is write-only |
| GET | `/api/v1/settings/ddns/status` | loop state: last check, last change, last address, result, failures, alert |

All three need a root token. See the [API reference](/api-reference).

## Related

- [Domains and ingress](/domains-and-ingress)
- [Standalone domains](/standalone-domains)
- [Environment variables](/environment-variables)
