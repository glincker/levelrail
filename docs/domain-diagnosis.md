---
title: "Diagnose a domain"
description: Find out why a domain is not working, see the exact DNS record to create, and fix it from the dashboard, the CLI or the API.
"og:url": "https://docs.levelrail.com/domain-diagnosis"
---

When a domain does not load, open it on the **Domains** page and choose **Diagnose**. The control plane runs a fixed list of checks (no AI is involved), shows each result as it arrives, and gives every problem a fix. The same diagnosis is available as `levelrail-cli domains diagnose` and as an API call.

It builds on the [domain doctor](/domains-and-ingress#domain-doctor): same checks, plus Cloudflare proxy detection, wildcard awareness, plain language certificate errors, propagation timing, and fixes that the platform can apply for you.

## What it checks

| Check | What it tells you |
| --- | --- |
| `dns.resolves` | What this server and two public resolvers (1.1.1.1 and 8.8.8.8) answer, asked in parallel. Disagreement means DNS is still propagating. |
| `dns.points_here` | Whether the answers include this server's public address. |
| `dns.proxied` | The domain is behind Cloudflare's proxy (orange cloud), found from Cloudflare's published address ranges or a CNAME into Cloudflare. |
| `dns.aaaa_mismatch` | AAAA records that send IPv6 visitors somewhere else. |
| `dns.wildcard` | A wildcard record that also answers for this name, or, for `*.example.com`, whether a DNS provider is connected for the DNS-01 challenge it needs. |
| `dns.cname_apex` | A CNAME at the zone apex. |
| `caa.blocks_issuer` | CAA records that do not allow Let's Encrypt. |
| `net.port80`, `tls.certificate`, `http.status`, `redirect.chain`, `hsts.risk` | Ports 80 and 443 and what answers on them. These only run when the domain resolves to this server; the diagnosis never connects to other addresses. |
| `cert.renewal` | The stored certificate and the certificate authority's last error, translated (see below). |

Every check has a state: `pass`, `warn`, `fail`, `skipped` or `unavailable`.

## Certificate errors in plain language

The raw error from Let's Encrypt is kept, but the diagnosis leads with what it means:

| Raw error contains | Plain language |
| --- | --- |
| `rateLimited`, `too many certificates` | Let's Encrypt is rate limiting this domain. It lifts by itself within a week; the platform retries. |
| `CAA` | A CAA record does not allow Let's Encrypt. |
| `dns-01` or `_acme-challenge` with `403`, `unauthorized`, `forbidden` | The DNS provider refused the platform's token. It needs permission to edit DNS for the zone. |
| `NXDOMAIN`, `no valid A records` | Let's Encrypt could not find DNS records for the domain. |
| `Invalid response from http://` | Something other than this platform answered on port 80. |
| `timeout`, `connection refused`, `firewall` | Let's Encrypt could not reach port 80 (the challenge failed). |

## Fixes

Each problem carries one fix of one of three kinds:

| Kind | What you get |
| --- | --- |
| `auto` | A **Create the record** (or **Start app**, **Renew now**) button. The platform does it. For a missing DNS record this needs a connected Cloudflare or Route 53 account. |
| `guided` | The exact record to enter at your DNS provider, with a copy button, or a link to the page that holds the setting. |
| `manual` | An explanation of what to change, for things only you can change, such as a proxy setting at another company. |

After any fix, press **Re-check**. The diagnosis never assumes a fix worked.

The record to create depends on the name: an address record (`A`, or `AAAA` for an IPv6-only server) at a zone apex such as `example.com` or `example.co.uk`, and a `CNAME` to this server's public hostname for a subdomain such as `app.example.com`. When the server has no public hostname set, a subdomain gets an address record too.

## Propagation

The result includes a `propagation` block:

| State | Meaning | Check again |
| --- | --- | --- |
| `settled` | Every resolver agrees. | Not needed. |
| `waiting` | No resolver knows the name yet. | In 30 seconds. |
| `propagating` | Resolvers disagree while an old answer's TTL runs out. | After the longest TTL seen, between 15 seconds and 5 minutes. |

The dashboard polls on that schedule while the dialog or panel is open.

## From the dashboard

- **Domains page:** open a domain's row and choose **Diagnose**. Results stream in, DNS first.
- **Add domains dialog:** after adding, each domain shows the exact records to create and a live propagation status, so you can leave the dialog with DNS done.

## From the CLI

```bash
levelrail-cli domains add my-app shop.example.com        # add, then print the exact records
levelrail-cli domains diagnose shop.example.com          # every check with its fix
levelrail-cli domains diagnose shop.example.com --fix    # apply auto fixes, then diagnose again
levelrail-cli domains diagnose shop.example.com --output json --query "checks[?state=='fail'].id"
```

`--fix` only runs fixes of kind `auto`. Guided and manual fixes are printed, never applied.

## From the API

| Call | Ability | Returns |
| --- | --- | --- |
| `POST /api/v1/domains/{domain}/diagnose` | `read` | One JSON report in canonical check order, with `records` and `propagation`. |
| `GET /api/v1/domains/{domain}/diagnose/stream` | `read` | Server-sent events: one `{"type":"check"}` message as each check finishes, then `{"type":"done"}` with the full report. |
| `POST /api/v1/apps/{name}/domains/{domain}/doctor` | `read` | The same report when you already know the app. |

The owning app is found for you, and a caller who cannot see that app gets `404`. Applying an `auto` fix is a normal API call (for a record, `POST /api/v1/apps/{name}/domains/{domain}/dns-records`, which needs `root`).

## Speed

All network probes have a time limit and run concurrently: the three resolvers (and the A and AAAA lookups of each) in parallel, and the checks that do not depend on each other at the same time. The whole run is capped by `APP_DOMAIN_DOCTOR_TIMEOUT` (default 20 seconds) and each probe by `APP_DOMAIN_DOCTOR_PROBE_TIMEOUT` (default 4 seconds). On a healthy domain the first results arrive as soon as DNS answers.

## What it does not do

It does not change DNS on its own, does not connect to addresses that are not this server's, and does not retry certificate issuance by itself; **Renew now** is a button you press.
