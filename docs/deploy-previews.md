---
description: Thumbnails of each deploy in three tiers, from free (page metadata, no browser) to a real screenshot from a short-lived browser container. What each tier costs, what it can leak, how it is cleaned up and every APP_PREVIEW_* setting.
---

# Deploy previews

When a deploy goes live, the control plane can save a small thumbnail of the running app and show it in the app's deploy history. There are three modes per app, and the default costs about nothing.

| Mode | What it does | Cost |
| --- | --- | --- |
| `off` | No preview. | None. |
| `metadata` (default) | One small request to the app, reads its title and social image (`og:image` or `twitter:image`). Uses that image as the thumbnail, or a text card when there is none. | No browser, no container, no image download. One request of at most 5 seconds and 512 KB, plus at most one 2 MB image. |
| `screenshot` | A real screenshot from a short-lived browser container. | Up to 512 MB of memory and 1 CPU for about 10 seconds per deploy, and a browser image of about 143 MB downloaded once. |

New apps start in the server's default mode, `metadata`. Nothing runs while idle in any mode: there is no resident browser, and a screenshot capture starts a container, takes one picture and removes the container again.

## Choose a mode

Open the app, then **Deploy settings**, then **Deploy previews**, and pick a mode. Or use the CLI:

```bash
levelrail preview enable web --mode metadata
levelrail preview enable web --mode screenshot --path /pricing
levelrail preview disable web
levelrail preview status web
levelrail preview capture web
```

`preview enable` without `--mode` means `screenshot`, as `enabled: true` always did. The next successful deploy gets a thumbnail. **Recapture** in deploy history (or `levelrail preview capture`) redoes the current release on demand. Each thumbnail carries a badge saying where it came from: Screenshot, Site image or Card.

## What the metadata mode does

- Requests the configured path (default `/`) on the app itself, through the app's published loopback port, the same route the ingress uses. It never contacts the public internet for the page, and you never supply a URL, only a path.
- Sends no cookies or credentials. A 401 or 403, or a redirect to a login page, is recorded as skipped with a reason. Redirects that leave the app are refused, and at most 3 are followed.
- Reads at most 512 KB of HTML with a real HTML tokenizer and keeps the title, `og:title`, `og:description`, `og:image`, `twitter:image`, `theme-color` and icon links.
- If there is an `og:image` (or `twitter:image`), it is resolved against the page, must be an `http` or `https` URL, and is downloaded through the same outbound guard the webhooks use: loopback, private, link-local and metadata-service addresses are refused at connect time, including after redirects. The app's own origin and its own domains are read from the app itself. Limit 2 MB.
- JPEG and PNG images are supported. The image is decoded, scaled down to fit 640 by 400 (never up) and centered on a neutral background, then stored as a JPEG. Images under 120 pixels on a side and single-color images are rejected.
- When there is no usable image, the preview is a card: the deploy history composes it from the page title, description and theme color, with the app name and commit. A card stores about 100 bytes of text and no image file.

Because the page is read as an anonymous visitor would see it, an app whose home page shows a login gets no preview, not a picture of the login.

## What a screenshot capture does

- Pulls the browser image the first time it is needed (`docker.io/chromedp/headless-shell`, pinned to a version, about 140 MB compressed).
- Starts one container with a 512 MB memory cap, 1 CPU, a read-only root filesystem, a small tmpfs, all capabilities dropped, a non-root user and a process limit.
- Attaches it to the app's own Docker network and nothing else. It reaches the app by service name and port. You never supply a URL, only a path on the app.
- Loads the page at 1280x800 without cookies or credentials, waits for the page to settle (at most about 10 seconds), takes a screenshot, and stops the container. The whole capture has a 30 second limit.
- Shrinks the screenshot to a 640 pixel wide JPEG, usually 20 to 60 KB.

A capture takes 2 to 15 seconds after the first one. It runs one at a time across all apps. A burst of deploys queues, and a newer deploy of an app replaces its older queued capture.

## Cost

| Resource | `metadata` | `screenshot` |
| --- | --- | --- |
| Idle | Zero memory, zero CPU. One coarse cleanup pass per hour. | Same. No container, no resident browser. |
| Per deploy | One HTTP request to the app (5 second limit) and at most one image download (2 MB cap). | Up to 512 MB of memory and 1 CPU for about 10 seconds. |
| Download | None. | The browser image, about 143 MB, once. |
| Disk | About 50 KB or less per deploy for a site image, about 100 bytes for a card. | The browser image, plus roughly 40 KB per deploy. |

Screenshot captures are skipped, with the reason recorded, when the host has less than 768 MB of free memory or less than 2 GB of free disk. The metadata mode is not gated on either, because it needs neither.

### Screenshot browser benchmark

The screenshot tier uses `chromedp/headless-shell` (pinned). To see whether a smaller browser was worth shipping, a Chromium-only route was tried on 2026-09-26 as a time-boxed experiment, on Docker Desktop for macOS (Linux arm64 VM, 8 GB) with one trivial local page:

- The Brotli `chromium` build from the `Sparticuz/chromium` releases (v153.0.0, arm64) is a 68,249,600 byte download that unpacks to a 199 MB binary, plus about 25 MB of SwiftShader libraries and 4 MB of support libraries.
- It runs in a `debian:12-slim` image. It needed `socat` (the new headless mode ignores `--remote-debugging-address`, so the debug port is only on the container's loopback) and its bundled font config points at AWS Lambda paths, so no text rendered until the fonts were copied to `/opt/fonts`. Both are fixed in the test image.
- With those fixes the existing CDP client took a correct screenshot. Start to screenshot took 1.2 seconds against 1.7 seconds for `headless-shell` (one run each, so treat the gap as noise).
- Docker reported the image at 133,293,451 bytes against 146,147,349 for `headless-shell` on the same machine: about 9 percent smaller, not the 50 to 65 MB the binary alone suggests, because the base image, libraries and fonts make up the rest.

That is not enough to justify a second browser code path, an extra image build to maintain and the Lambda-specific workarounds, so it was not shipped. To use your own smaller image, point `APP_PREVIEW_IMAGE` at it: it must expose the DevTools port on 9222 like `headless-shell` does. Memory was sampled once with `docker stats` after the screenshot (about 53 MiB for the Chromium route, about 85 MiB for `headless-shell`), which is not a peak, so no memory claim is made.

## Privacy

A preview is a picture of what the app served, so treat it like the app.

- **Metadata is on by default, screenshots are opt-in.** The default reads only the public head of your page and, at most, one image the page itself names. A screenshot is never taken unless you choose it for that app.
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
| `not_html` | The page answered something other than HTML (metadata mode). |
| `redirect` | The page redirected away from the app (metadata mode). |
| `timeout`, `capture_failed`, `bad_image` | The capture itself failed. |

A failed recapture never replaces a thumbnail that already exists, and a card never replaces a site image or screenshot.

## Retention and cleanup

These rules apply to every source alike: screenshots, site images and cards.

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
| `APP_PREVIEW_ENABLED` | `true` | Global switch. When off, nothing runs in any mode and manual captures are refused. |
| `APP_PREVIEW_DEFAULT_MODE` | `metadata` | Mode of an app nobody has configured: `off`, `metadata` or `screenshot`. |
| `APP_PREVIEW_META_TIMEOUT` | `5s` | Limit for each request of the metadata mode. |
| `APP_PREVIEW_META_MAX_HTML_KB` | `512` | Most HTML read from a page. |
| `APP_PREVIEW_META_MAX_IMAGE_KB` | `2048` | Largest social image downloaded. |
| `APP_PREVIEW_META_MAX_REDIRECTS` | `3` | Redirects followed, all on the app itself. |
| `APP_PREVIEW_META_MIN_IMAGE_PX` | `120` | Smallest side of a social image worth using. |
| `APP_PREVIEW_THUMB_HEIGHT` | `400` | Height of the canvas a social image is fitted onto. |
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
| `PUT /api/v1/apps/{name}/preview` | Change `mode` (`off`, `metadata`, `screenshot`), `path` or `wait_ms`. `enabled` is still accepted: `true` means `screenshot`, `false` means `off`. `mode` wins when both are sent. |
| `POST /api/v1/apps/{name}/preview/capture` | Queue a recapture of the current release. |
| `POST /api/v1/apps/{name}/preview/prune` | Prune now. Body `{"all": true}` deletes everything. |
| `GET /api/v1/apps/{name}/preview/history` | One record per deployment, with status, reason, `source` (`screenshot`, `og_image` or `card`) and, for cards, `meta`. |
| `GET /api/v1/apps/{name}/deployments/{id}/preview` | The thumbnail (JPEG). Cards have no image, only `meta`. |

Reading needs the app read ability. Changing settings, capturing and pruning need write. `GET /api/v1/apps/{name}/deploy-attempts` includes `preview_image_url` on each attempt that has a thumbnail.

## Limits

- Local node only. Apps on other nodes are skipped.
- Screenshots skip apps without a per-app network or a declared port. Metadata needs only a running container with a published port.
- WebP and other formats are not decoded for site images, only JPEG and PNG.
- The page is requested by service name, so an app that routes by `Host` header shows its default site.
- Only the viewport is captured, not the full page. Cookie banners and late-loading content appear as the app renders them.
