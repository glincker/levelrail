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
- Do not add analytics or request capture in front of the demo. The demo only keeps aggregate counters that carry no personal data.

## Check it

- `curl -sI https://demo.example.com/` returns the security headers.
- `curl -s https://demo.example.com/api/v1/demo` reports `"enabled": true` and the next reset time.
- A write such as `curl -s -X POST https://demo.example.com/api/v1/apps` returns `403` with the code `demo_readonly`.
- To take the demo down: `systemctl stop levelrail-demo.service levelrail-demo-reseed.timer`.
