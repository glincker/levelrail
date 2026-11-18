---
title: "Host a public read-only demo"
description: Runbook for serving the read-only demo to the internet behind Caddy on one host, with periodic resets, no Docker access and a short log retention window.
"og:url": "https://docs.levelrail.com/hosted-demo"
---

This is the reverse proxy variant of [Run a read-only demo](/run-a-read-only-demo). The demo binds loopback only and Caddy on the same host is the one public listener. The files live in `deploy/hosted-demo/`. Replace every `example.com` value before use.

## Layout

- The `levelrail demo serve` process runs as the unprivileged `levelrail-demo` user under systemd and listens on `127.0.0.1:8080`.
- Caddy terminates HTTPS for `demo.example.com` and proxies to that loopback port.
- The bind guard is unchanged: the demo refuses to start on a non-loopback `APP_HTTP_ADDR`.

## Set it up

- Create the user and directories: a `levelrail-demo` system user with no shell, the binary under `/opt/levelrail-demo/bin`, state under `/var/lib/levelrail-demo`.
- Copy `env.example` to `/etc/levelrail-demo/env` (mode `0640`, group `levelrail-demo`). Keep it free of secrets.
- Copy `reseed.sh` to `/opt/levelrail-demo/reseed.sh` and make it executable.
- Run the first seed once: `APP_DATA_DIR=/var/lib/levelrail-demo/data levelrail demo seed --seed 1`, as the `levelrail-demo` user.
- Install `levelrail-demo.service`, `levelrail-demo-reseed.service` and `levelrail-demo-reseed.timer` into `/etc/systemd/system`, then `systemctl enable --now levelrail-demo.service levelrail-demo-reseed.timer`.
- Install the `Caddyfile` as `/etc/caddy/Caddyfile` and reload Caddy.

## Reset on a schedule

- The timer runs `reseed.sh`, which seeds a fresh directory, stops the service, swaps the directory in and starts the service again.
- Set `APP_DEMO_RESET_INTERVAL` (for example `30m`) in the env file to the same cadence as the timer. The dashboard banner then shows the next reset time. Unset, it shows the daily 04:00 UTC default.
- The value is a Go duration of at least one minute and the time is aligned to UTC multiples of it.

## Harden the host

- Rate limits per client IP: the Caddyfile uses the `caddy-ratelimit` module, which needs a custom Caddy build. Without it, limit at your CDN or firewall. The demo also limits session creation per IP on its own.
- Request size limits: Caddy caps bodies at 16KB and the demo caps them at 4KB.
- Security headers: the Caddyfile sets HSTS, `nosniff`, frame denial, a no-referrer policy and a permissions policy. The demo adds `X-Robots-Tag: noindex, nofollow` itself.
- No Docker access: the service user is not in the `docker` group and the unit hides the Docker socket paths. The demo refuses to start if `DOCKER_HOST` is set.
- Outbound network: the unit sets `IPAddressDeny=any` with loopback allowed, so the process cannot call out. Keep the host firewall default-deny for outbound traffic from that user as a second layer.
- Sandboxing: the unit drops all capabilities, mounts the system read-only and caps memory, CPU and tasks.

## Logging policy

- Caddy access logs drop the client address, cookies and authorization headers, keep at most two files of 10MiB and delete them after 72 hours.
- The demo process logs to the journal. Set `SystemMaxUse` and `MaxRetentionSec=3day` in `journald.conf` so the journal follows the same window.
- Analytics are off by default, and a self-hosted instance never has them. The demo keeps aggregate counters that carry no personal data. See Analytics below for the opt-in.

## Share tags and theme

The demo page carries link-preview tags (title, description, `og:image` of `/demo-og.png`, a large Twitter card and a canonical link). Set `APP_DEMO_PUBLIC_URL` to the public https origin to change the URL used in them; it defaults to `https://demo.levelrail.com`, and a value that is not an absolute https URL is ignored with a log warning. The demo stays noindex. First-time visitors see the light theme; a stored choice of dark, OLED or system still wins.

## Analytics (opt-in)

The demo can send a few named events to a PostHog project. It stays off unless both variables are set on the demo process:

- `APP_DEMO_ANALYTICS_HOST`: the https ingest origin, for example `https://us.i.posthog.com`.
- `APP_DEMO_ANALYTICS_TOKEN`: the project token.

With both set, `GET /api/v1/demo` returns an `analytics` object and the demo CSP allows that host in `script-src` and `connect-src`. For a PostHog cloud host (`*.i.posthog.com`) the matching assets host (for example `us-assets.i.posthog.com`) is allowed too. The widened CSP is sent on every demo response, including the dashboard HTML and static assets. A real instance never returns it.

Analytics start as soon as the SPA loads on any route, including the signed-out `/login` page, so a visitor who never clicks "Explore the demo" is still counted. The page reuses the public `GET /api/v1/demo` call the shell already makes; a real instance (`enabled: false`) loads nothing and makes no extra request. Each provider is loaded once per page load, so the later sign-in does not load it again.

What is collected: the events `demo_landing_viewed` (once per page load, on the first route), `demo_session_started` (the Explore the demo click), `demo_route_viewed` (route pattern only), `demo_readonly_blocked` (HTTP method and route pattern only), `demo_install_command_copied` and `demo_github_click`, all tagged `surface=demo`, plus `utm_source`, `utm_medium` and `utm_campaign` when the landing URL has them.

What is not collected: no cookies or local storage (memory persistence), no session recording, no autocapture, no record ids or query strings. Visitors with Do Not Track or Global Privacy Control (`navigator.globalPrivacyControl`) enabled load nothing and send nothing.

IP addresses are seen by PostHog on receipt. Turn off IP capture in the PostHog project settings if you want to promise they are not stored.

### Google Tag Manager (opt-in)

Google Tag Manager is a second, independent provider. It works alone or next to PostHog. Set one variable on the demo process:

- `APP_DEMO_GTM_ID`: the container id, for example `GTM-XXXXXXX`. It must match `^GTM-[A-Z0-9]{4,12}$`; anything else is ignored with a log warning. The id is supplied at deploy time and is never committed.

### Star count

The top bar Star button shows the live GitHub star count. The demo process fetches it in the background once at start and at most every 6 hours, with a 5 second timeout. It never blocks a request and never fails the demo: on error the last good value is kept, or the count is omitted from `GET /api/v1/demo` and the button shows no number.

- `APP_DEMO_STARS_REPO`: the `owner/name` repository to read. Defaults to `glincker/levelrail`. Set it empty to disable the fetch.
- `APP_DEMO_STARS_DISABLE`: set to `true` to make no outbound request at all, for example on a host hardened to block egress.

Nothing loads unless the variable is set, and a real instance never returns it. With it set, `GET /api/v1/demo` returns `gtm: {"id": "..."}` and the demo CSP additionally allows `www.googletagmanager.com` (script, connect, image) plus `*.google-analytics.com`, `*.analytics.google.com` and `*.googletagmanager.com` (connect, image). The CSP still has no `unsafe-inline` or `unsafe-eval` for scripts.

The page loads the standard `gtm.js` from Google and pushes the same six events as `{event, surface: "demo", ...}` into `window.dataLayer`, with the route pattern only and no ids or query strings. Build tags and triggers on those event names in the GTM account that owns the container. Levelrail does not configure the container.

Google tags can set cookies and send visitor data to Google, so EU visitors may need consent. In GTM, enable Consent Mode with `analytics_storage` denied by default, and turn on IP anonymisation in GA4. Visitors with Do Not Track or Global Privacy Control enabled load nothing and send nothing. The demo banner has no place for a privacy note today, so state the use of Google tags in your own privacy notice for the demo site.

## Check it

- `curl -sI https://demo.example.com/` returns the security headers.
- `curl -s https://demo.example.com/api/v1/demo` reports `"enabled": true` and the next reset time.
- A write such as `curl -s -X POST https://demo.example.com/api/v1/apps` returns `403` with the code `demo_readonly`.
- To take the demo down: `systemctl stop levelrail-demo.service levelrail-demo-reseed.timer`.
