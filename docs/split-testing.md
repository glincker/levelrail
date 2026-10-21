---
title: "Split testing"
description: Send a share of an app's visitors to a branch deploy or a retained release, sticky per visitor, then promote the winner.
"og:url": "https://docs.levelrail.com/split-testing"
---

Run two versions of an app at the same domain and compare them. A split test
sends a percentage of visitors to a branch deploy or a retained release, keeps
each visitor on the variant they were given, counts requests per variant, and
lets you promote the winner to production.

Split testing is different from a [canary deploy](/canary-deploys). A canary
splits each request by weight to roll out a new image safely. A split test
assigns each visitor once, so one person never sees both versions.

## How it works

- A test has two to five variants. One is always **production** (the app
  itself). The others are a **branch deploy** (see branch deploys) or a
  **retained release** (a deploy URL service name). Weights are whole percents
  and must add up to 100.
- Ingress picks a variant for a new visitor by weight and sets a cookie. The
  cookie is `HttpOnly`, `SameSite=Lax`, `Secure` on https, and is set only
  while a test is running. Visitors with a valid cookie keep their variant.
- Variant traffic is served by the variant's own route, so its TLS, auth and
  traffic policies apply. Production traffic goes through the app's normal route.
- A variant that is not reachable (for example its branch deploy was removed)
  receives no visitors: its weight moves to production until it is back.
- Changing weights keeps existing visitors on their variant. A visitor moves
  only when their variant drops to 0 or the test ends.
- The test is stored in the database and restored at boot. Request counts are
  kept in memory and written to the database every `APP_SPLIT_FLUSH_INTERVAL`,
  so a crash can lose a few seconds of counts.

## Start, watch, promote

```
levelrail-cli apps split start web --variant branch:redesign=30
levelrail-cli apps split status web
levelrail-cli apps split weights web --set production=60 --set Redesign=40
levelrail-cli apps split promote web --variant Redesign
levelrail-cli apps split stop web
```

The same actions are on the app page, under **Split testing**: a variants
table with a weight slider per variant, request counts and share, a copyable
QA link per variant, and Promote and Stop buttons.

- **Promote** deploys the variant's image to the app through the normal image
  deploy path (readiness gate, rollback target, freeze and protection checks),
  then ends the test. It is refused while publishing is locked for the app. If
  the deploy fails the test keeps running. If the deploy works but ending the
  test fails, run promote again: deploying the image the app already runs is
  skipped and the test ends.
- **Stop** ends the test. Every visitor gets production again and the request
  counts stay visible.

## QA override

Open `https://your-domain/?lr_variant=<variant id or label>` to force that
variant for your browser. The cookie is set to it, so you keep it on later
visits. Each variant's QA link is shown in the UI and in `status`.

## Defaults

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SPLIT_COOKIE_NAME` | `lr_split` | Sticky cookie name. A test can set its own. |
| `APP_SPLIT_COOKIE_TTL` | `720h` | Sticky cookie lifetime. A test can set its own. |
| `APP_SPLIT_OVERRIDE_PARAM` | `lr_variant` | Query parameter that forces a variant. |
| `APP_SPLIT_FLUSH_INTERVAL` | `30s` | How often request counts are saved. |

## Limits

- One running test per app. Container apps with a domain only: static sites
  are not supported yet (the UI says so), although ingress can already split a
  static route once the API allows it.
- Variants are branch deploys or retained releases of the same app; promoting
  uses the variant's image.
- Variant requests reach their route through a local hop, so the variant sees
  the visitor's address in `X-Forwarded-For` rather than as the connection
  address.
- Counts are requests, not unique visitors, and there is no significance test.
  Use your own analytics for conversion.
