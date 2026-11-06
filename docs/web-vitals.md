# Web vitals

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

`POST /api/v1/vitals/{app}` takes `{"key": "...", "path": "/blog/12", "m": [{"n": "LCP", "v": 1200}]}` from `navigator.sendBeacon`. Unknown app, disabled app and wrong key all answer the same 404. Beacons are rate limited per app and size capped, and the Origin must be one of the app's domains or localhost (an app with no domain accepts any). The key is public by design: it only lets a page add counters. Rotate it from the card if it is abused.

## Storage and privacy

Only histogram counters are stored, per app, route, metric, hour, device class (mobile or desktop) and fixed bucket. No raw events, cookies, IP addresses, user agent strings or query strings are kept, and id-like path segments fold into `:id`. Percentiles are bucket upper bounds, so p75 is accurate to one bucket.

Runtime settings: `vitals.enabled` (default for apps without their own switch), `vitals.rate_limit_per_minute`, `vitals.retention_days` (default 30) and `vitals.max_body_bytes`. Environment variables only seed them.

## Why the control plane API (ADR 074 open question 7)

The collector lives on the control plane API, not the ingress. The API already owns app records, per-app keys, runtime settings and the dashboard queries, so the beacon, the rollup and the card share one process and one store. Putting it on the ingress would need a new store handle and settings path in a hot, separately owned package, and a same-origin path would collide with app routes. The cost is a cross-origin beacon, which `sendBeacon` handles without preflight, and the per-app key plus Origin check stand in for same-origin trust.
