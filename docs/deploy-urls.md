---
title: "Deploy URLs"
description: "Every recent successful deploy keeps its own address, so you can open and compare a past deployment without rolling back."
"og:url": "https://docs.levelrail.com/deploy-urls"
---

Each of an app's newest successful deploys keeps an address of its own, for
example `web-3f9a2c1.apps.example.com`. The address never changes and never
points at a newer release, so you can open an older deploy next to the live one.
The deploy history shows a Visit button on each of them, and the live deploy
keeps answering on the app's own URL.

```bash
levelrail-cli apps deploys url web da_4Kq9xT2mVnQ1
```

## How it works

For each kept deploy the control plane runs a retained copy of that deploy's
pinned image with its recorded config. A retained copy has no volumes, host
ports, hooks or automatic rollback, and it sleeps after
`APP_DEPLOY_URL_IDLE_MINUTES` without traffic (default 5), then wakes on the
first visit, so old deploys do not run idle.

The hostname is one label under the apps base domain. Without a base domain it
falls back to the sslip.io address of the control plane's public IP. With
neither, no URLs are created.

## Retention

| Setting | Default | Meaning |
| --- | --- | --- |
| `APP_DEPLOY_URL_KEEP` | 3 | Newest successful deploys per app that keep a URL. 0 turns the feature off and removes the copies. |
| `APP_DEPLOY_URL_IDLE_MINUTES` | 5 | Idle time before a retained copy sleeps. |
| `APP_DEPLOY_URL_INTERVAL` | 1m | How often new deploys are picked up. |

A deploy only keeps a URL while its image can still be redeployed: it needs a
registry digest or a rollback pin. Rollback pins and image cleanup are not
changed by this feature.

A retained copy runs the old code with the app's env, so jobs and webhooks of an
old deploy can fire while it is awake. It does not get the app's volumes.

Retained copies are internal: they are left out of the app list, search and the dashboard, and the delete API refuses them with a 409. They are removed automatically when their deploy ages out.
