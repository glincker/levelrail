---
title: "Traffic analytics: page views, API calls and bots"
description: How the edge classifies each request as page, API, asset, bot or prefetch so the Visitors card's Top pages lists page views only. Beta, off by default.
"og:url": "https://docs.levelrail.com/traffic-analytics"
---

The Visitors card on an app's **Traffic** tab is built from the edge access log. Without this feature, **Top pages** ranks every request path, so an app whose dashboard polls its own API shows `/api/...` calls as its most visited "pages". The beta `traffic-analytics` feature fixes that: the edge sorts each request into one class, and **Top pages** counts page views only.

It is off by default. Turn it on in Settings, Beta features (it applies live), with `levelrail-cli settings beta-features set traffic-analytics on`, or list it in `APP_EXPERIMENTAL`. See [Experimental features](/experimental-features).

## The classes

Each request gets exactly one class when the response header is known. The first step that matches wins.

| Step | Rule | Class |
| --- | --- | --- |
| 1 | A path rule matches. Defaults: `/api/*`, `/graphql`, `/trpc/*`, `/_next/data/*` and `/socket.io/*` are `api`; `/_next/static/*`, `/assets/*`, `/static/*`, `/build/*`, `/favicon.ico`, `/robots.txt`, `/sitemap*.xml`, `/manifest.json` and `/.well-known/*` are `asset` | per rule |
| 2 | The User-Agent is empty, contains `HeadlessChrome`, matches the built-in list of known crawler and tool tokens, or the abuse shield already turned the request away | `bot` |
| 3 | `Sec-Purpose` or `Purpose` contains `prefetch`, `X-Moz: prefetch`, or the request is a Next.js router prefetch | `prefetch` |
| 4 | `RSC: 1` (a Next.js client navigation) | `page`, counted as a page view |
| 5 | `Sec-Fetch-Dest`: `document` is `page`, `empty` (fetch and XHR) is `api`, other destinations such as `script`, `style`, `image`, `font` and `iframe` are `asset` | per value |
| 6 | No `Sec-Fetch-Dest` (older browsers, curl): a GET answered with `text/html` to a client that accepts `text/html` is `page`; JSON, event streams and `X-Requested-With: XMLHttpRequest` are `api`; a static file extension is `asset` | per rule |
| | Nothing matched | `other` |

Path rules come first so an operator can correct a misclassified framework. Bots come before `Sec-Fetch-Dest` because headless browsers send `document`. `Sec-Fetch-Dest` beats the response type, so an XHR that returns HTML (HTMX, Turbo partials) is not a page view.

A page-class request counts as a page view only when it is a GET with a 2xx status. Redirects and 404 or 5xx documents are not page views. Bot names come from the client's own User-Agent: they are claims, not verified identities, and crawlers are not checked by reverse DNS.

## What you see

With the feature on:

- **Top pages** lists page views only, per normalized path (ids collapsed, trailing slash folded, at most 128 bytes).
- A small split under the totals shows page, API, asset, bot, prefetch and other request counts, so the calls that used to crowd the list are still visible.
- An app with requests but no page views shows "No page views yet" with the API, asset and bot counts instead of an empty list.
- `levelrail-cli apps visitors <app>` prints the same split and a `PAGE  VIEWS` table. The JSON body of `GET /api/v1/apps/{name}/visitors` gains `classes`, `top_pages` and `pages_only`; `top_paths` and every older field are unchanged. The CSV export gains `class` and `page` rows.

Visitors, status classes, referrers and the series still cover all requests. This release changes **Top pages** and adds the split only.

Rollups saved before upgrading carry no class, so a 7 or 30 day window shows page views only from the upgrade onward.

## Limits and settings

- Single page apps that change routes in the browser without a request (other than Next.js) are invisible to the edge.
- `APP_TRAFFIC_PAGE_LABELS` (default `500`) caps the distinct page paths kept per app and hour in the stored rollups. Paths past the cap are counted under `(other)`.
- `APP_TRAFFIC_RULES_MAX` (default `50`) caps path rules per app.
- Memory: the class is one byte on each access log record. The rollup keeps at most `APP_TRAFFIC_PAGE_LABELS` labels per app, domain and hour, and flushes the top 10 (`APP_VISITOR_ROLLUP_TOP_N`).
- No client address, User-Agent or query string is stored by this feature.

## Status

Beta. It is unit tested at the classifier, edge handler, rollup, store, API and CLI layers. There is no end-to-end test and no recorded run on a real host. See [Feature status](/feature-status).
