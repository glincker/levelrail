---
title: "Cloudflare Tunnel"
description: "Expose the control plane through a Cloudflare Tunnel with no open inbound port. Experimental: how to enable it, set the token, check status and remove it."
"og:url": "https://docs.levelrail.com/cloudflare-tunnel"
---

Cloudflare Tunnel runs the official `cloudflared` container next to the control plane. The container dials out to Cloudflare with a tunnel token from your own account, so visitors reach you through Cloudflare without any inbound port open on the server.

<Warning>
Experimental. It is built and covered by reconciler and API unit tests, but it has not been proven on a real host with a real Cloudflare account. Try it on a test server first. It stays experimental until a real run is recorded, see [Experimental features](/experimental-features).
</Warning>

## What it does

- Starts one container named `<brand>-cloudflared` (for example `levelrail-cloudflared`) from `cloudflare/cloudflared`, running `tunnel --no-autoupdate run`. The image tag is pinned and can be overridden with `APP_CLOUDFLARED_IMAGE`.
- Passes the token to the container as the `TUNNEL_TOKEN` environment variable. The token is stored encrypted in the platform's secrets store and is never returned by the API, CLI or MCP.
- Keeps the container converged: it creates it when enabled, restarts it if it stopped, replaces it when the image changes, and removes it when you disable or disconnect.
- Does not configure Cloudflare for you. Which hostname goes to which service is set on Cloudflare's side, in the Zero Trust dashboard.

It is a different credential from the DNS-01 token in `domains cloudflare-dns`, and it does not replace [Run behind Cloudflare](/behind-cloudflare), which covers the orange cloud proxy.

## Turn the feature on

Switch it on in Settings under beta features, or with `levelrail-cli settings beta-features set cloudflare-tunnel on`. It applies at once, with no restart: the reconciler is always registered and checks the switch on every pass. `APP_EXPERIMENTAL=cloudflare-tunnel` still works as a seed. With the switch off the API answers `404` with `experimental_feature_disabled`.

Switching it off stops reconciling and leaves any running container untouched. To remove the container, disconnect first (it deletes the token and removes the container), then switch the feature off.

The control plane also needs its secrets master key. Without one, `PUT` and `DELETE` answer `501`.

## Set it up

1. In the Cloudflare Zero Trust dashboard, create a tunnel and copy its token.
2. In the same dashboard, add a public hostname for the tunnel and point it at a service the `cloudflared` container can reach, such as the address of your Levelrail ingress. The container uses the default Docker network, so use an address reachable from there, not a platform container name.
3. Give the token to Levelrail, in the dashboard under Settings, Cloudflare Tunnel, or with the CLI:

```
levelrail-cli cloudflare-tunnel set --cf-tunnel-token TOKEN
levelrail-cli cloudflare-tunnel get
```

`--cf-tunnel-token` is required the first time you enable the tunnel. Later calls may omit it to keep the stored token. `--enabled=false` stops the container and keeps the token. The same settings are available over `GET`, `PUT` and `DELETE /api/v1/settings/cloudflare-tunnel`, and as a read-only `get_cloudflare_tunnel_status` MCP tool.

## Check status

`cloudflare-tunnel get` returns `enabled`, `has_token`, `status` and a `message` when something is wrong.

| Status | Meaning |
| --- | --- |
| `connected` | The `cloudflared` container is running. This reports the container, not Cloudflare's view of the connection, so confirm the tunnel is healthy in the Zero Trust dashboard too. |
| `error` | Enabled but not converging, for example no token yet or the container failed to create or start. The message has the reason. |
| `disconnected` | Disabled or never configured. |

## Security notes

- The token lets anyone who holds it run a connector for your tunnel. Treat it like a password and rotate it in Cloudflare if it leaks, then run `set --cf-tunnel-token NEW`.
- The token is visible in the container's environment to anything with access to the Docker socket on the host, like other credentials the platform injects into containers.
- Changing settings needs the root ability. Reading status needs read access.
- The tunnel exposes only the hostnames you route in Cloudflare. Levelrail does not add any. Add Cloudflare Access policies for anything that should not be public, and keep your own login in front of the dashboard.
- Traffic reaches the server from the `cloudflared` container, so client addresses are not the visitor's. See [Run behind Cloudflare](/behind-cloudflare) for real client IP handling.

## Remove it

```
levelrail-cli cloudflare-tunnel disconnect
```

This disables the tunnel, deletes the stored token and removes the container. It is safe to run twice. Then delete the tunnel and its public hostnames in Cloudflare, otherwise the DNS records keep pointing at a tunnel with no connector.

## Known gaps

- Not proven on a real host or a real account.
- Status reflects the local container only.
