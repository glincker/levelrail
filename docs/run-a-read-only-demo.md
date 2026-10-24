---
title: "Run a read-only demo"
description: Run a read-only demo instance of the dashboard with fictional data, no Docker and no way to change anything.
"og:url": "https://docs.levelrail.com/run-a-read-only-demo"
---

The demo entrypoint serves the real dashboard and API over a seeded data directory of fictional apps, deploy history, databases and nodes. It never talks to Docker, never opens ingress or agent ports, and rejects every write on the server, so it is safe to put in front of strangers.

It is a separate subcommand. A normal instance ignores `APP_DEMO_MODE` and logs a warning, so a stray variable can never turn a real install into a demo.

## Seed the data

```sh
APP_DATA_DIR=/var/lib/demo-data levelrail demo seed --seed 1
```

- `--seed` picks the deterministic data set. The same seed and anchor always produce the same rows.
- `--anchor` sets the "now" the 14 days of history are counted back from (RFC 3339, default the current hour).
- `--scenario` picks the storyline: `incident` (default, the public demo with a crashlooping app, pending approvals and a firing disk alert) or `calm` (a healthy fleet with succeeded deploys only and at most one pending approval, used for marketing screenshots). Unknown values are rejected.
- Seeding refuses a data directory that already holds users, and re-running with the same options is a no-op.

To refresh the time anchor, seed into a new empty data directory and swap it in.

## Serve it

```sh
APP_DEMO_MODE=1 APP_DATA_DIR=/var/lib/demo-data levelrail demo serve
```

`serve` fails closed. It exits with an error unless all of these hold:

- `APP_DEMO_MODE` is set to a true value.
- The data directory carries the seeder's marker and holds only seeded users.
- `DOCKER_HOST`, `APP_AGENT_ADDR`, `APP_INGRESS_HTTP_ADDR` and `APP_INGRESS_HTTPS_ADDR` are all unset.
- `APP_HTTP_ADDR`, if set, is a loopback address. The default is `127.0.0.1:8080`.

Put your own reverse proxy or tunnel in front of the loopback port. For a Caddy based public host see [Host a public read-only demo](/hosted-demo).

Set `APP_DEMO_RESET_INTERVAL` (a duration such as `30m`, at least one minute) if your host re-seeds more often than daily; the banner then shows the matching next reset time.

## What visitors can do

Visitors sign in as a read-only account, either through the one-click demo session or with the credentials shown on the login form. Sessions last two hours.

- Every route that is not on a short allowlist returns `403` with the JSON code `demo_readonly`. The allowlist holds sign-in, sign-out and a few pure computations such as spec validation.
- Routes that would reach a host or a third party (terminal, exec, node provisioning, update checks, Git provider flows, exports, downloads, AI chat) return `403` with the code `demo_disabled`.
- API tokens, the auth library endpoints and WebSocket upgrades are refused.
- Every response carries `X-Robots-Tag: noindex, nofollow` and `/robots.txt` disallows everything.

`GET /api/v1/demo` reports whether demo mode is on, when the data resets, and the install command to show in a banner. It returns `{"enabled": false}` on a real instance.

## Human check

`POST /api/v1/demo/session` accepts an optional human check through the `api.HumanVerifier` interface and rate limits session creation per client IP. The built-in server ships without a verifier; enforce a challenge at your edge, or supply one when embedding the router.

## What it does not do

- It does not run the reconciler, collectors, builds, backups, ingress or the self-updater.
- It does not simulate deploys or rollbacks. Those buttons return `demo_readonly`.

## Host it on a small box

The `deploy/demo/` directory holds a hardened, generic layout. The demo binary listens on loopback only and a tunnel such as `cloudflared` dials out to it, so no inbound port is open and no reverse proxy is needed.

```sh
deploy/demo/build-demo.sh amd64        # builds dist/demo/levelrail-demo-linux-amd64.tar.gz
# copy and unpack on a fresh Debian or Ubuntu box, then as root:
./levelrail-demo/install.sh            # idempotent
```

What gets installed:

- A `levelrail-demo` system user, the binary under `/opt/levelrail-demo`, state under `/var/lib/levelrail-demo` and an env file at `/etc/levelrail-demo/env`.
- `levelrail-demo.service`, which allows only loopback traffic (`IPAddressDeny=any`), drops all capabilities, mounts the system read-only and caps memory, CPU and tasks.
- `levelrail-demo-reseed.timer`, a nightly re-seed that re-anchors the 14 days of history. It seeds into a new directory and swaps it in.
- `levelrail-demo-tripwire.timer`, hourly. It re-seeds if the apps, databases or users tables differ from the seed, or if a non-GET request succeeded outside the allowlist in the unit's journal.

Copy `cloudflared-config.yml.example` and `cloudflared-demo.service.example` for the tunnel, and replace the placeholder hostname. To take the demo down, `systemctl stop levelrail-demo`.
