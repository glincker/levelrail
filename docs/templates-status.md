---
title: "Template status"
description: Measured pass rate of the one-click template catalog from the local smoke run
---

Measured by `scripts/catalog-smoke.sh`: each template's compose file is brought up with `docker compose up --wait`, checked for running and healthy containers, then torn down with its volumes. Host ports are dropped so runs never collide. Regenerate with `scripts/catalog-smoke.sh -r` (all of the top 50) or pass `-i id1,id2` for a subset. A fail with an unhealthy container means the template's own healthcheck never passed on this host, so check that healthcheck and the image first.

Last run: 2026-10-19. **8 of 12 passed (67%).** Templates that need operator configuration are skipped and listed in the run output, not counted.

| Template | Result | Seconds | Note |
| --- | --- | --- | --- |
| `actual-budget` | fail | 35 | container lrsmoke-actual-budget-actual-1 is unhealthy |
| `adminer` | pass | 33 |  |
| `filebrowser` | pass | 8 |  |
| `gotify` | fail | 39 | container lrsmoke-gotify-gotify-1 is unhealthy |
| `homepage` | pass | 10 |  |
| `it-tools` | pass | 33 |  |
| `linkding` | pass | 16 |  |
| `memos` | fail | 34 | container lrsmoke-memos-memos-1 is unhealthy |
| `ntfy` | pass | 32 |  |
| `pocketbase` | pass | 13 |  |
| `uptime-kuma` | pass | 16 |  |
| `vaultwarden` | fail | 64 |  |
