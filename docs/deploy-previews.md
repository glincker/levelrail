---
description: Opt-in thumbnails of each deploy, captured once by a short-lived browser container. What it costs, what it can leak, how it is cleaned up and every APP_PREVIEW_* setting.
---

# Deploy preview screenshots

When a deploy goes live, the control plane can save a small thumbnail of the running app and show it in the app's deploy history. It is off by default, twice: the server switch is off, and each app has to opt in.

Nothing runs while idle. There is no resident browser. A capture starts a container, takes one screenshot and removes the container again.

## Turn it on

1. Start the control plane with `APP_PREVIEW_ENABLED=true`.
2. Open the app, then **Deploy settings**, then **Deploy preview screenshots**, and switch it on. Or use the CLI:

```bash
levelrail preview enable web --path /pricing
levelrail preview status web
levelrail preview capture web
```

The next successful deploy gets a thumbnail. **Recapture** in deploy history (or `levelrail preview capture`) redoes the current release on demand.

## What a capture does

- Pulls the browser image the first time it is needed (`docker.io/chromedp/headless-shell`, pinned to a version, about 140 MB compressed).
- Starts one container with a 512 MB memory cap, 1 CPU, a read-only root filesystem, a small tmpfs, all capabilities dropped, a non-root user and a process limit.
- Attaches it to the app's own Docker network and nothing else. It reaches the app by service name and port. You never supply a URL, only a path on the app.
- Loads the page at 1280x800 without cookies or credentials, waits for the page to settle (at most about 10 seconds), takes a screenshot, and stops the container. The whole capture has a 30 second limit.
- Shrinks the screenshot to a 640 pixel wide JPEG, usually 20 to 60 KB.

A capture takes 2 to 15 seconds after the first one. It runs one at a time across all apps. A burst of deploys queues, and a newer deploy of an app replaces its older queued capture.

## Cost

| Resource | Cost |
| --- | --- |
| Idle | Zero memory, zero CPU, no container. One coarse cleanup pass per hour. |
| During a capture | Up to 512 MB of memory and 1 CPU for about 10 seconds. |
| Disk | The browser image (about 140 MB), plus roughly 40 KB per deploy for thumbnails. |

Captures are skipped, with the reason recorded, when the host has less than 768 MB of free memory or less than 2 GB of free disk.

## Privacy

A preview is a picture of what the app served, so treat it like the app.

- **Opt-in per app.** Nothing is captured for an app that has not been enabled.
- **Internal pages can leak.** If the path you pick shows internal data to anyone who can reach the app, the thumbnail shows it too. Pick a public page, such as `/`.
- **No login.** The browser sends no cookies and no tokens. A 401 or 403, or a redirect to a login page, is recorded as skipped and no image is stored.
- **Same access as the app.** The image is served only to people who can read the app, through the authenticated API. It is never public and never appears on the public status page.
- **Not a sandbox for hostile apps.** The page is loaded by a real browser next to the app. The container limits above contain a browser bug, but only capture apps you run.
- The browser's debugging port is published on the host loopback interface for the length of a capture, on a random port, so other processes on the host can reach it during that window.

## What is skipped

The deploy history shows a "No preview" note with the reason when a capture does not produce an image:

| Reason | Meaning |
| --- | --- |
| `low_ram`, `low_disk` | The host was under the free memory or disk floor. |
| `auth_wall` | The app answered 401 or 403, or redirected to a login page. |
| `http_status` | The app answered a non-2xx status. |
| `unreachable` | The app did not answer on its private network. |
| `blank_image` | The page rendered a single flat color. |
| `no_app_network` | The app has no per-app network or declares no port. |
| `remote_node` | The app runs on another node. Only the local node is supported. |
| `browser_image_unavailable` | The browser image could not be pulled. |
| `timeout`, `capture_failed`, `bad_image` | The capture itself failed. |

A failed recapture never replaces a thumbnail that already exists.

## Retention and cleanup

- The newest 5 previews of each app are kept, plus the preview of the release now serving.
- Previews older than 30 days are deleted, except the live release's.
- If all previews together pass 200 MB, the least recently viewed go first.
- Previews are deleted when the app is deleted.
- Every capture container carries a role label. On startup and every hour, any older than twice the capture timeout (left by a crash) is removed.
- The browser image is removed after 14 idle days, or at the next cleanup pass once the server switch is off, but only if this control plane pulled it and no container uses it. An image you pulled yourself is never touched.
- **Prune now** (or `levelrail preview prune <app>`) applies these rules immediately. `--all` deletes every preview of the app.

## Settings

Set these on the control plane. Unset or malformed values fall back to the default.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PREVIEW_ENABLED` | `false` | Global switch. When off, nothing is captured and manual captures are refused. |
| `APP_PREVIEW_IMAGE` | pinned `chromedp/headless-shell` | Browser image. Point it at your own digest-pinned copy if you prefer. |
| `APP_PREVIEW_TIMEOUT` | `30s` | Hard limit for one capture. |
| `APP_PREVIEW_PULL_TIMEOUT` | `5m` | Limit for the first image pull. |
| `APP_PREVIEW_MEMORY_MB` | `512` | Memory cap of the capture container. |
| `APP_PREVIEW_CPUS` | `1.0` | CPU cap of the capture container. |
| `APP_PREVIEW_VIEWPORT` | `1280x800` | Browser window size. |
| `APP_PREVIEW_THUMB_WIDTH` | `640` | Thumbnail width in pixels. |
| `APP_PREVIEW_QUALITY` | `72` | JPEG quality. |
| `APP_PREVIEW_MAX_THUMB_KB` | `200` | Largest thumbnail accepted. |
| `APP_PREVIEW_BLANK_RATIO` | `0.995` | Share of one color at which a page counts as blank. |
| `APP_PREVIEW_KEEP_PER_APP` | `5` | Previews kept per app, besides the live one. |
| `APP_PREVIEW_TTL_DAYS` | `30` | Age after which previews are deleted. |
| `APP_PREVIEW_MAX_TOTAL_MB` | `200` | Global storage budget. |
| `APP_PREVIEW_IMAGE_TTL_DAYS` | `14` | Idle days before the browser image is removed. |
| `APP_PREVIEW_MIN_FREE_RAM_MB` | `768` | Skip captures below this much free memory. |
| `APP_PREVIEW_MIN_FREE_DISK_MB` | `2048` | Skip captures below this much free disk. |
| `APP_PREVIEW_QUEUE_DEPTH` | `8` | Captures that may wait. The oldest is dropped when full. |
| `APP_PREVIEW_SWEEP_INTERVAL` | `1h` | How often cleanup runs. |

## API

| Route | Use |
| --- | --- |
| `GET /api/v1/apps/{name}/preview` | Settings, storage used and the latest result. |
| `PUT /api/v1/apps/{name}/preview` | Change `enabled`, `path` or `wait_ms`. |
| `POST /api/v1/apps/{name}/preview/capture` | Queue a recapture of the current release. |
| `POST /api/v1/apps/{name}/preview/prune` | Prune now. Body `{"all": true}` deletes everything. |
| `GET /api/v1/apps/{name}/preview/history` | One record per deployment, with status and reason. |
| `GET /api/v1/apps/{name}/deployments/{id}/preview` | The thumbnail (JPEG). |

Reading needs the app read ability. Changing settings, capturing and pruning need write. `GET /api/v1/apps/{name}/deploy-attempts` includes `preview_image_url` on each attempt that has a thumbnail.

## Limits

- Local node only. Apps on other nodes are skipped.
- Apps without a per-app network or a declared port are skipped.
- The page is requested by service name, so an app that routes by `Host` header shows its default site.
- Only the viewport is captured, not the full page. Cookie banners and late-loading content appear as the app renders them.
