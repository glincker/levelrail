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

## Links and the public HTTPS port

When a proxy owns 80 and 443 and forwards to this ingress on its own ports
(for example `8088` and `8443`), the certificate lives on the proxy and clients
reach the proxy on 443. Set **Public HTTPS port** under **Domains > Platform
ingress** (or `levelrail-cli settings ingress set --public-https-port 443`) so
every link the platform builds (automatic URLs, preview URLs, emailed links)
uses the port clients actually use. `443` produces links with no port suffix,
any other value is appended, and `0` keeps the ingress listen port.

## Several instances behind one proxy

Each instance cannot own 80 or 443 and must not run ACME. Give every instance
its own ingress ports, let one proxy hold 80 and 443, and turn on **TLS is
terminated by a proxy in front** (`--tls-terminated-upstream`) on each:

```bash
levelrail-cli settings ingress set --tls-terminated-upstream --public-https-port 443
```

In this mode the instance never requests certificates (a stored ACME setting is
kept but skipped), builds `https` links with the public port (443 when unset),
and does not redirect HTTP itself because the proxy does. Set
`APP_INGRESS_TRUSTED_PROXIES` to the proxy's address range so `X-Forwarded-Proto`
and `X-Forwarded-Host` are honoured only from it, never from the open internet.
The dashboard shows "HTTPS is handled by your proxy" instead of a certificate
error. DNS for each domain still points at the proxy host.

Traefik, one host rule per domain and one backend port per instance:

```yaml
http:
  routers:
    team-a:
      rule: Host(`a.example.com`) || HostRegexp(`^.+\.a\.example\.com$`)
      entryPoints: [websecure]
      tls: { certResolver: letsencrypt }
      service: team-a
    team-b:
      rule: Host(`b.example.com`)
      entryPoints: [websecure]
      tls: { certResolver: letsencrypt }
      service: team-b
  services:
    team-a:
      loadBalancer:
        servers: [{ url: "http://127.0.0.1:8088" }]
    team-b:
      loadBalancer:
        servers: [{ url: "http://127.0.0.1:8089" }]
```

Instance A runs with `APP_INGRESS_HTTP_ADDR=:8088`, instance B with
`APP_INGRESS_HTTP_ADDR=:8089`. Forward to the HTTP port: TLS ends at the proxy.

## API

`GET /api/v1/system/reverse-proxy?domain=D&proxy=traefik&verify=true`.
The port and mode above are `public_https_port` and `tls_terminated_upstream` on
`PUT /api/v1/settings/ingress`.
The check only probes a domain that resolves to this server's own public
address.
