---
title: Run behind an existing proxy
description: "Serve the dashboard through the proxy that already owns ports 80 and 443 (Traefik, Caddy, nginx), with generated configuration and a built-in check."
---

# Run behind an existing proxy

On a server that already runs Coolify, Dokploy or your own nginx, something else
owns ports 80 and 443. Levelrail does not fight it: it detects the holder and
generates the exact configuration to put the dashboard behind it.

## From the dashboard

**Domains > Put the dashboard behind your existing proxy.** Enter the domain,
check the detected proxy, and copy the configuration. **Check it works**
resolves the domain, confirms it points at this server, and requests
`/healthz` over HTTPS.

## From the CLI

```bash
levelrail-cli proxy                                  # who holds 80 and 443
levelrail-cli proxy --domain console.example.com     # configuration for that domain
levelrail-cli proxy --domain console.example.com --verify
```

## What it generates

| Proxy | You get |
|---|---|
| Traefik | a dynamic configuration file (Coolify's is `/data/coolify/proxy/dynamic/`) with HTTPS and a Let's Encrypt router |
| nginx | a server block with WebSocket headers, plus the certbot command |
| Caddy | a Caddyfile site block |

If the proxy runs in a container and the dashboard listens on loopback, the
guide also gives a systemd drop-in that moves the dashboard to that network's
gateway address. That address is internal to the host, not reachable from the
internet, and a drop-in survives upgrades.

## API

`GET /api/v1/system/reverse-proxy?domain=D&proxy=traefik&verify=true`.
The check only probes a domain that resolves to this server's own public
address.
