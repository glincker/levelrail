---
title: Run behind Cloudflare
description: "Put Cloudflare's proxy in front of Levelrail: SSL mode, certificates, websockets, upload limits, real client IPs, cache purge, and an honest list of what the proxy does not hide."
"og:url": "https://docs.levelrail.com/behind-cloudflare"
---

Cloudflare's orange cloud puts its network between visitors and your server. Levelrail works behind it, but a few settings have to match or the site fails quietly. This page covers those settings and says plainly what the proxy does **not** protect.

## What the proxy does

For a proxied hostname, visitors connect to Cloudflare, and Cloudflare connects to your server on their behalf. You get Cloudflare's caching, DDoS absorption and WAF rules for that hostname, and the public DNS answer for it is a Cloudflare address instead of your server's.

It does not make your server unreachable. Anyone who learns the server address can still connect to it directly and skip every Cloudflare rule. See [What stays exposed](#what-stays-exposed).

## Turn the proxy on for an app

With a connected Cloudflare DNS provider, Levelrail creates the app's records and can set the proxy flag per app:

```bash
levelrail-cli apps cdn get <app>
levelrail-cli apps cdn set <app> --proxied on
```

`apps cdn get` shows the resolved proxy flag and the zone's SSL/TLS mode. Details are in [CDN and caching](/cdn-and-caching#cloudflare-purge-on-publish).

## SSL/TLS mode: use Full (strict)

Set the zone's **SSL/TLS** mode in the Cloudflare dashboard.

| Mode | Result |
| --- | --- |
| Off, Flexible | Redirect loop. Cloudflare talks plain HTTP to your server, and Levelrail redirects HTTP to HTTPS. Never use these. |
| Full | Works, but Cloudflare accepts any certificate, including Levelrail's internal fallback. A failed Let's Encrypt issuance looks healthy. |
| Full (strict) | Recommended. Cloudflare checks the origin certificate, so a missing or expired certificate shows up as a 526 instead of staying hidden. |

Levelrail only reads the SSL mode today. It warns on Off and Flexible and suggests strict for Full. It never changes your zone setting.

## Certificates

Full (strict) needs a real certificate on the origin.

- **HTTP-01** works through the proxy when port 80 on your server is reachable from Cloudflare, because Cloudflare forwards `/.well-known/acme-challenge/` to the origin. It fails when the zone redirects HTTP to HTTPS before the first certificate exists ("Always Use HTTPS" or a redirect rule), or when a WAF or bot rule challenges the validator. The failure is quiet: the internal fallback certificate is served, and Full (strict) then returns 526.
- **TLS-ALPN-01** never works through the proxy, because Cloudflare ends TLS. Caddy tries it and falls back, so it only costs time.
- **DNS-01** does not depend on the proxy, and survives port 80 being closed. It is the recommended challenge for proxied hosts. Turn it on for every host with `levelrail-cli settings ingress dns01 enable` (needs a connected DNS provider). DNS-01 for single hosts is not yet verified against a real certificate authority.

## WebSockets, streams and timeouts

- WebSockets work on every Cloudflare plan. Cloudflare can drop long-lived connections when it restarts edge processes, so apps need reconnect logic.
- Cloudflare returns 524 when the origin sends nothing for a long stretch (about 100 seconds on most plans; check yours). Streams and SSE that send keepalives are fine.
- Keep `APP_INGRESS_IDLE_TIMEOUT` longer than your app's ping interval.

## Upload size

Cloudflare caps request bodies by plan, and the cap is lower than Levelrail's own default of 1 GiB (`APP_INGRESS_MAX_BODY_BYTES`). Above the cap the visitor gets Cloudflare's 413 and Levelrail never sees the request. Check your plan's limit in Cloudflare's documentation, and for large uploads use a direct (DNS only) upload hostname or presigned uploads to object storage.

## Real client IP

Without configuration, every visitor looks like a Cloudflare address to access logs, [Shield](/domain-traffic-controls#bot-and-abuse-shield) rate limits, country rules and bans. Turn on the Cloudflare preset and the ingress reads the visitor address from `CF-Connecting-IP`, but only when the connection comes from Cloudflare's published ranges. A forged header from any other peer is ignored.

```bash
levelrail-cli settings ingress cloudflare show
levelrail-cli settings ingress cloudflare set --preset cloudflare --confirm-weakening
```

The same switch is in the dashboard under Domains, Platform settings, "Cloudflare client IP", and in the API at `GET`/`PUT /api/v1/settings/ingress/cloudflare` (root only; `confirm_weakening` is required because the ingress starts trusting a header). It is a stored setting that applies at once. `APP_INGRESS_TRUSTED_PROXY_PRESET` only seeds the default, and entries in `APP_INGRESS_TRUSTED_PROXIES` stay trusted.

- A snapshot of the ranges ships in the binary and is refreshed in the background from Cloudflare's official endpoint. The last good list is kept if a fetch fails or returns too few ranges. The interval (default 24h), timeout, size limit and sanity floor are runtime settings `ingress.cloudflare_ips_*`, and the last refresh and error show in the settings.
- Shield never bans or rate limits an address inside the Cloudflare ranges, whatever the preset, because that would block every visitor behind it. It logs a warning once.
- Any Cloudflare customer can point a proxied record at your server, so trusting Cloudflare's ranges does not prove a request came through your zone.

See also [Real client IPs behind a CDN or proxy](/domains-and-ingress#real-client-ips-behind-a-cdn-or-proxy).

## Cache purge

When an app's records are proxied, a static publish, deploy, held-build publish and rollback each purge Cloudflare's cache for the app's hostnames. The token needs the **Cache Purge** permission. The purge is best effort, and records you created by hand are not purged. See [Cloudflare purge on publish](/cdn-and-caching#cloudflare-purge-on-publish).

## What stays exposed

The proxy hides the address behind proxied hostnames only. These still reveal your server's IP or let people reach it:

- **Mail records.** MX targets, `mail.` hosts, `autoconfig`, `mta-sts` and similar names must be DNS only, and SPF in `server` mode publishes the address in a TXT record. Every message your server sends also carries its address in the `Received:` header. Running mail on the same server and hiding its address are not compatible. See [Mail DNS](/mail-dns).
- **Direct subdomains.** Any DNS only record that points at the server (`ftp.`, `direct.`, an old app name) publishes it, and so does an unproxied AAAA record.
- **Direct IP access.** Anyone who knows the address can connect with the real hostname and reach the app without passing through Cloudflare's WAF or rate limits. Restrict ports 80 and 443 to Cloudflare's published ranges in your cloud firewall (Oracle security list, Hetzner or DigitalOcean cloud firewall, AWS security group). Levelrail does not edit cloud firewalls, and a range list goes stale when Cloudflare adds ranges. Allowing Cloudflare's ranges alone does not prove the request came from your zone, because any Cloudflare customer can point a proxied record at your address.
- **Automatic sslip.io hostnames.** A fallback hostname such as `app.203-0-113-10.sslip.io` contains the IP and gets a public certificate, so it appears in certificate transparency logs. Turn the **Automatic app hostnames** setting off on a server you want to hide.
- **Outbound connections.** Webhooks, link previews, OAuth callbacks, image fetchers and git pulls show the server's address in the logs of whatever they contact.
- **DNS history.** If a hostname was ever DNS only, historical DNS databases keep the old address. Only changing the public IP helps, and only once nothing publishes the new one.
- **Multi-node installs.** Node agents use mutual TLS, which the HTTP proxy cannot carry, so the agent hostname must be DNS only or use a private network address.

Levelrail's own checks run on the server, so they report configuration and cannot prove what the internet sees. To test from outside, run `curl --resolve app.example.com:443:<server-ip> https://app.example.com/` from a machine that is not on your network.

## Safe workflow

1. Connect Cloudflare DNS in Levelrail and issue certificates with DNS-01.
2. Set the zone's SSL/TLS mode to Full (strict) after the certificate is valid.
3. Turn the proxy on per app and check `levelrail-cli apps cdn get <app>`.
4. Turn on the Cloudflare client IP preset.
5. List every DNS only record in the zone and remove or proxy the ones that are not mail or agent records.
6. Disable automatic hostnames if you want the address out of certificate logs.
7. Restrict ports 80 and 443 to Cloudflare ranges at your cloud firewall, keeping SSH on a port you can still reach.
8. Treat any address that was ever published as known. Changing it is the only fix.

## Next steps

- [CDN and caching](/cdn-and-caching)
- [Domains and ingress](/domains-and-ingress)
- [Mail DNS](/mail-dns)
- [Run behind an existing proxy](/behind-an-existing-proxy)
