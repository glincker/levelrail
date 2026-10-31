---
title: "CDN and caching"
description: "The response cache in the ingress: what it stores, what it never stores, cache rules, purge by URL, prefix or tag, and a third party CDN in front."
"og:url": "https://docs.levelrail.com/cdn-and-caching"
---

The ingress can keep responses in memory and answer repeat requests itself, so a busy static file or public page never reaches your app twice. It is one in-process cache, with no extra service to run. The **CDN and caches** section of the dashboard shows how well it works and lets you purge it.

## Where to find it

<Tabs>
<Tab title="Dashboard">

- **CDN and caches** (Networking in the sidebar): **Overview** (requests, server errors, cache hit rate, bandwidth saved and which of your servers answered), **Caches** (hit rate over time, a table of apps, purge and purge history), **Redirects** and **Routing rules** (links to each domain's existing controls).
- An app's **Cache** tab (Traffic section): hit rate, the **Cache static assets** preset, each domain's cache state, purge and history.
- A domain's **Cache** tab: the full rule editor.

</Tab>
<Tab title="CLI">

```bash
levelrail-cli cache status [<app>]
levelrail-cli cache rules show <app> [--domain D]
levelrail-cli cache rules set <app> --preset static [--domain D]
levelrail-cli cache rules set <app> --file cache.json [--domain D]
levelrail-cli cache rules clear <app> [--domain D]
levelrail-cli cache purge <app> (--url /a?b=1 | --prefix /static | --tag blog | --all) [--domain D]
levelrail-cli cache history <app>
```

Without `--domain` a command covers every domain of the app. Every command accepts `--json` and `-o`.

</Tab>
<Tab title="API">

```text
GET  /api/v1/apps/{name}/cache              hit rate, bytes saved and per domain state
POST /api/v1/apps/{name}/cache/purge        {"scope":"url|prefix|tag|all","value":"...","domain":"optional"}
GET  /api/v1/apps/{name}/cache/purges       who purged what
POST /api/v1/apps/{name}/cache/preset       add the static assets rule
GET  /api/v1/system/cache                   instance totals and the per app table
GET  /api/v1/cdn/overview?window=5m|1h|24h  requests, errors, hit rate and servers
```

The per domain endpoints in [domain traffic controls](/domain-traffic-controls#caching) still apply: they hold the rules themselves.

</Tab>
</Tabs>

## Turning it on

Caching is off until you ask for it. The recommended start is **Cache static assets**: one rule for images, scripts, styles, fonts and media files that keeps them for an hour when your app sends no `Cache-Control`, serves them while a refresh runs for a minute, and keeps serving them for a day if your app starts failing. If your app sends `max-age` or `s-maxage` (fingerprinted assets usually send a year, often with `immutable`), that value wins. Running the preset twice changes nothing.

For anything else, add rules on the domain's **Cache** tab. A rule matches a path (prefix, exact or regex) and can:

- set a default **TTL** for when the app sends none, or override the app's `Cache-Control` entirely
- **never cache** the paths it matches (rules are checked in order, so list these first)
- ignore listed **query parameters** such as `utm_source` so tracking links share one entry
- add a **header** (`vary`) or a **cookie** value to the cache key
- serve **stale while revalidating** and **stale if error** for a number of seconds

A domain with no rule that matches is never cached.

## What is cached and what never is

- Only `GET` and `HEAD` are cached.
- Never served from or stored in the cache: requests with `Authorization`, requests with a `Cookie` (unless the rule sets `cache_with_cookies`), range requests, upgrades and requests with `Cache-Control: no-store`.
- Never stored: responses with `Set-Cookie`, `Cache-Control: private`, `no-store` or `no-cache`, `Vary: *`, a `Vary` header the rule does not list, a partial content range, event streams, a body larger than the object cap, or a status outside the rule's list (default `200`).
- A TTL of 0 means do not store.

## Key and Vary

The key is the request host, the path and query (minus ignored parameters), `Accept-Encoding`, the headers in the rule's `vary` list and any `key_cookies` values. Entries from one host are never served for another. A `Vary` header on a response is honored only for headers on that list (plus `Accept-Encoding`); any other `Vary` makes the response uncacheable rather than risk serving the wrong variant.

## Freshness, stale and revalidation

- TTL: the app's `s-maxage`, then `max-age`, then the rule's `ttl_seconds`. With `override_upstream` the rule always wins. TTLs are capped by `APP_INGRESS_CACHE_MAX_TTL`.
- **Request coalescing:** when several identical requests miss at once, one goes to the app and the others wait (up to `APP_INGRESS_CACHE_COALESCE_WAIT`, default 10 seconds) and are answered from the stored copy.
- **Stale while revalidate:** after expiry the first request refreshes the entry and others get the old copy marked `STALE` for the rule's window.
- **Stale if error:** if the app answers `5xx` or cannot be reached and an expired copy is still inside the rule's `stale_if_error_seconds`, visitors get that copy marked `STALE` instead of the error.
- **Revalidation:** an expired entry with an `ETag` or `Last-Modified` is kept for `APP_INGRESS_CACHE_REVALIDATE_WINDOW` (default 1 hour). The refresh sends `If-None-Match` or `If-Modified-Since`, and a `304` from the app renews the entry without resending the body. Browsers sending their own conditional headers get `304` from the cache when the validators match.
- Every response on a cached path carries `X-Cache: HIT`, `MISS`, `STALE` or `BYPASS`, and a hit has an `Age` header.

## Purge

| Scope | Removes |
| --- | --- |
| URL | One path and query, every `Vary` variant. Either the URL as visitors requested it or with ignored parameters removed works. |
| Prefix | Every path under a prefix. |
| Tag | Every object whose response carried the tag. Your app sends tags in the `Cache-Tag` response header (comma or space separated, up to 32 per object); `APP_INGRESS_CACHE_TAG_HEADER` renames the header. Tags are recorded when the object is stored. |
| All | Everything cached for the app (or one domain). The dashboard asks you to type the name first, because the app then receives all traffic until the cache refills. |

Every purge is recorded with who ran it, when, the scope and how many objects it removed, shown under **Purge history** and in the audit log. History keeps the newest `APP_CACHE_PURGE_HISTORY_KEEP` purges per app (default 200).

Purges apply to the ingress cache on the control plane, which is the only place entries live today, including for apps whose containers run on other nodes.

## What is measured

Per domain, in memory: hits, stale hits, misses, bypasses, body bytes served from the cache and body bytes fetched from the app on cacheable paths, with the last 30 minutes per minute. **Bandwidth saved** is the bytes served from the cache. No URLs or client addresses are recorded. These counters start at zero when the ingress starts, so a restart resets them; the dashboard says so next to the numbers. Requests, error rate and latency come from the regular request metrics and keep their normal retention.

The **Overview** attributes requests to the node each app runs on, with the node's region label when you set one. It does not know where visitors are: there is no geography data source in Levelrail, so no map of visitors is shown.

## Limits

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_INGRESS_CACHE_MAX_BYTES` | 64 MiB | Total memory for all cached responses. The least recently used objects are dropped first. |
| `APP_INGRESS_CACHE_MAX_OBJECT_BYTES` | 2 MiB | Largest single response stored. A domain can set a lower cap. |
| `APP_INGRESS_CACHE_MAX_TTL` | 7 days | Longest lifetime of any entry. |
| `APP_DOMAIN_POLICY_MAX_CACHE_RULES` | 20 | Cache rules per domain. |
| `APP_INGRESS_CACHE_TAG_HEADER` | `Cache-Tag` | Response header holding purge tags. |
| `APP_INGRESS_CACHE_REVALIDATE_WINDOW` | 1 hour | How long an expired entry with validators is kept. |
| `APP_INGRESS_CACHE_COALESCE_WAIT` | 10 seconds | How long a request waits for an identical request already fetching. |
| `APP_CACHE_PURGE_HISTORY_KEEP` | 200 | Purge records kept per app. |

The cache is per instance and starts empty after a restart.

## How it fits with other controls

Domain redirects, maintenance mode, basic auth, the WAF, [Shield](/domain-traffic-controls#bot-and-abuse-shield) and geo rules run before the cache lookup, so a blocked request never receives a cached response, and a cached response is never a way around them. Cache hits skip the app but still count in request metrics.

## Putting a CDN in front

A CDN such as Cloudflare, Akamai or Fastly can sit in front of the ingress. Levelrail does not manage it yet, but these settings make the two work together:

- Send correct `Cache-Control` from your app (`public, max-age=...` or `s-maxage=...` for shared caches, `private` or `no-store` for anything personal). Both the CDN and the ingress follow it. Send a tag header such as `Cache-Tag` if you purge Levelrail by tag; check which header your CDN uses for its own tags, since the names differ.
- Add the CDN's address ranges to `APP_INGRESS_TRUSTED_PROXIES` so visitor addresses, Shield rate limits and country rules use the real client address from `X-Forwarded-For` rather than the CDN's. See [behind an existing proxy](/behind-an-existing-proxy).
- Purge the CDN with its own tools; purging Levelrail does not purge the CDN, and the reverse. First party integrations (purge, steering) are a future plugin direction recorded in ADR 034; none is built.

## Not yet

- Image optimization and resizing.
- ISR style revalidation of rendered pages on a schedule or event.
- Visitor geography and a map of it.
- A multi-region or per node edge cache. One cache lives in the ingress on the control plane.
- Disk spill for objects that do not fit the memory cap.

## Next

- A `cache:` block in `app.yaml` with a JSON Schema.
- Disk backed storage with checksummed objects, sized by environment variables.
- Cross node purge once more than one ingress serves traffic.
- Routing rules in the CDN section will use the shared rule builder (redirect with status, rewrite, set header) instead of linking to each domain's page.
