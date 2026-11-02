---
title: "Cloudflare in front checklist"
description: "The order to set up Cloudflare's proxy in front of a Levelrail server: SSL mode, client addresses, the per-app proxy, cache purge, and what a rollback does not change."
"og:url": "https://docs.levelrail.com/cloudflare-in-front"
---

Use this page as the order of operations. Each step names the setting and the command, and links to the page that explains the reasoning. [Run behind Cloudflare](/behind-cloudflare) lists what the proxy does not hide.

## Before you turn the proxy on

- [ ] A Cloudflare DNS provider is connected in Levelrail, on the Domains page or with `levelrail-cli domains cloudflare-dns set --cf-api-token <token>`. The token needs `Zone:DNS:Edit` and `Zone:Zone:Read`, and the **Cache Purge** permission if you want purges.
- [ ] Certificates for proxied hosts use DNS-01, which does not depend on the proxy: `levelrail-cli settings ingress dns01 enable`. This needs a connected DNS provider. Single-host DNS-01 is not yet verified against a real certificate authority (pending run).
- [ ] You know which hosts are proxied. Records Levelrail creates for an app follow that app's proxy setting. Records you created by hand keep their own flag.

## Set the zone

- [ ] Set the zone's **SSL/TLS** mode to **Full (strict)** once the origin certificate is valid. Off and Flexible cause a redirect loop. Full accepts any certificate, so a failed issuance stays hidden. Levelrail reads the mode and warns, and `levelrail-cli apps cdn get <app>` shows it. Levelrail never changes the zone setting.
- [ ] Check **Always Use HTTPS** and any redirect rule. A redirect that fires before the first certificate exists blocks HTTP-01, the fallback certificate is served, and Full (strict) then returns 526.
- [ ] Check that no WAF or bot rule challenges `/.well-known/acme-challenge/` requests to the origin. A challenge there fails issuance quietly in the same way.

## Set the client address source

- [ ] Turn on the Cloudflare client address preset: `levelrail-cli settings ingress cloudflare set --preset cloudflare --confirm-weakening`. Check it with `levelrail-cli settings ingress cloudflare show`.

Without the preset, every visitor appears to come from a Cloudflare address in access logs, Shield rate limits and country rules. With it, the ingress reads `CF-Connecting-IP` only when the connection comes from Cloudflare's published ranges. A forged header from any other peer is ignored. The preset is a stored setting and applies at once. The ranges refresh in the background, and the last good list is kept if a refresh fails.

## Turn the proxy on, one app at a time

- [ ] Preview the DNS changes: `levelrail-cli apps cdn resync <app> --dry-run`.
- [ ] Turn the proxy on: `levelrail-cli apps cdn set <app> --proxied on`. The command re-syncs the app's existing DNS records. It only touches records Levelrail created for that app, and only those whose proxy flag differs.
- [ ] Check the result: `levelrail-cli apps cdn get <app>` shows the resolved proxy flag and the zone's SSL/TLS mode.

For a new record, `levelrail-cli settings ingress set --dns-proxied` makes proxying the default for records Levelrail creates. Keep it off until the steps above are done.

## Upload size and long requests

- [ ] Find your Cloudflare plan's request body limit. It is lower than Levelrail's default of 1 GiB (`APP_INGRESS_MAX_BODY_BYTES`). Above the plan limit, the visitor gets Cloudflare's 413 and Levelrail never sees the request.
- [ ] For large uploads, use a DNS-only hostname or presigned uploads to object storage. See [Object storage](/object-storage).
- [ ] Long responses need keepalives. Cloudflare returns 524 when the origin sends nothing for about 100 seconds on most plans. Check your plan.

## Keep the origin reachable only through the proxy

- [ ] Restrict ports 80 and 443 to Cloudflare's published ranges in your cloud firewall. Levelrail does not edit cloud firewalls, and a range list goes stale when Cloudflare adds ranges.
- [ ] Allowing Cloudflare's ranges does not prove a request came through your zone. Any Cloudflare customer can point a proxied record at your server address.
- [ ] Turn off **Automatic app hostnames** on a server you want to hide. A sslip.io hostname contains the server address and gets a public certificate, so the address appears in certificate transparency logs.
- [ ] Keep the hostname that nodes connect to DNS only. Nodes use mutual TLS, which the HTTP proxy cannot carry. See [Run behind Cloudflare](/behind-cloudflare).

## Cache purge

When an app's records are proxied, a static publish, a deploy, a held build publish and a rollback each purge Cloudflare's cache for the app's hostnames. The purge is best effort. If it fails, the publish or rollback still succeeds, and the response carries a `cdn_warning` field. See [CDN and caching](/cdn-and-caching#cloudflare-purge-on-publish).

## Rollback does not restore the proxy

A rollback puts back an earlier release and purges the cache. It does not change the proxy setting or the DNS records. If you turned the proxy on or off after that release, the rollback leaves the current setting in place. Set it back with `levelrail-cli apps cdn set <app> --proxied on|off|inherit`.

This is read from the documented behavior, not yet confirmed on a live proxied record (pending run).

## Verify from outside

- [ ] Open the hostname from a network outside your own. The certificate is valid and the app loads (pending run).
- [ ] Try the server address directly over HTTPS with the real hostname. After the firewall step it should not connect (pending run).
- [ ] Check a rollback on a proxied app, then confirm the proxy setting and the cache purge warning (pending run).

## Related

- [Run behind Cloudflare](/behind-cloudflare): the reasoning, and what stays exposed.
- [Domains and ingress](/domains-and-ingress): DNS records, certificates and real client addresses.
- [CDN and caching](/cdn-and-caching): purge on publish and the per-app proxy setting.
