---
title: "Web vitals"
description: "Collect real user LCP, INP, CLS, TTFB and FCP for an app without a vendor SDK."
---

Levelrail collects real user LCP, INP, CLS, TTFB and FCP for an app with no vendor SDK. Vendor analytics SDKs from a typical managed platform do nothing elsewhere, so this replaces them.

## Turn it on

Open the app overview, switch on Web vitals and copy the snippet. The same data prints with `levelrail-cli apps vitals <app>`.

```html
<script defer src="https://YOUR-LEVELRAIL/api/v1/vitals/snippet.js" data-app="my-app" data-key="PUBLIC_KEY"></script>
```

Optional attribute: `data-sample="25"` reports about 25 percent of visits.

## Next.js

```tsx
import Script from 'next/script'

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        {children}
        <Script
          src="https://YOUR-LEVELRAIL/api/v1/vitals/snippet.js"
          data-app="my-app"
          data-key="PUBLIC_KEY"
          strategy="afterInteractive"
        />
      </body>
    </html>
  )
}
```

## Endpoint

`POST /api/v1/vitals/{app}` takes `{"key": "...", "path": "/blog/12", "m": [{"n": "LCP", "v": 1200}]}` from `navigator.sendBeacon`. Unknown app, disabled app and wrong key all answer the same 404. Beacons are rate limited per client address (before the body is read) and per app (only after the key matched), and size capped. The Origin must be one of the app's domains or localhost (an app with no domain accepts any); this check is advisory, since a non browser client can send any Origin, and a failed domain lookup denies the beacon. The key is public by design: it only lets a page add counters. Rotate it from the card if it is abused.

## Storage and privacy

Only histogram counters are stored, per app, route, metric, hour, device class (mobile or desktop) and fixed bucket. No raw events, cookies, IP addresses, user agent strings or query strings are kept.

Routes are normalized before they are stored. Only short lowercase words joined by hyphens (up to 24 characters, at most 6 segments) are kept, for example `/blog/my-post`. Every other segment, such as numbers, hex or base64 ids, tokens, emails, short links and non ASCII text, becomes `:id`. Each app records at most `vitals.max_routes_per_app` distinct routes per day (default 200); further routes are counted under `/other`. Nothing else about the page is stored. The report and card show the busiest `vitals.report_top_routes` routes (default 50). Rollups are kept for `vitals.retention_days` days (default 30) and removed by a background job; deleting an app deletes its key and rollups. Percentiles are bucket upper bounds, so p75 is accurate to one bucket, and a value above the top bound is shown as a lower bound such as `>10 s`.

Counters are buffered in memory and written every `vitals.flush_interval_seconds` (default 10). A clean shutdown flushes them; a crash loses at most one interval.

Runtime settings: `vitals.enabled` (whether a key created by Rotate starts enabled; an app only collects once it has a saved config with collection on), `vitals.rate_limit_per_minute`, `vitals.rate_limit_per_ip_per_minute`, `vitals.max_routes_per_app`, `vitals.report_top_routes`, `vitals.flush_interval_seconds`, `vitals.retention_days` and `vitals.max_body_bytes`. Environment variables only seed them.

## Why the control plane API (ADR 074 open question 7)

The collector lives on the control plane API, not the ingress. The API already owns app records, per-app keys, runtime settings and the dashboard queries, so the beacon, the rollup and the card share one process and one store. Putting it on the ingress would need a new store handle and settings path in a hot, separately owned package, and a same-origin path would collide with app routes. The cost is a cross-origin beacon, which `sendBeacon` handles without preflight, and the per-app key plus Origin check stand in for same-origin trust.
