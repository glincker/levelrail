---
title: "Run a read-only demo"
description: Run a read-only demo instance of the dashboard with fictional data, no Docker and no way to change anything.
---

The demo entrypoint serves the real dashboard and API over a seeded data directory of fictional apps, deploy history, databases and nodes. It never talks to Docker, never opens ingress or agent ports, and rejects every write on the server, so it is safe to put in front of strangers.

It is a separate subcommand. A normal instance ignores `APP_DEMO_MODE` and logs a warning, so a stray variable can never turn a real install into a demo.

## Seed the data

```sh
APP_DATA_DIR=/var/lib/demo-data levelrail demo seed --seed 1
```

- `--seed` picks the deterministic data set. The same seed and anchor always produce the same rows.
- `--anchor` sets the "now" the 14 days of history are counted back from (RFC 3339, default the current hour).
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

Put your own reverse proxy or tunnel in front of the loopback port.

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
