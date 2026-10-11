---
title: Run behind an existing proxy
description: "Serve apps and the dashboard through the proxy that already owns ports 80 and 443: one step managed routes for Traefik (Coolify included), or generated configuration for nginx and Caddy."
---

# Run behind an existing proxy

On a server that already runs Coolify, Dokploy or your own nginx, something else
owns ports 80 and 443. Levelrail does not fight it: it detects the holder and
generates the exact configuration to put the dashboard behind it.

## One step setup (Traefik, including Coolify)

When the proxy is Traefik with a file provider directory, Levelrail can manage
the routes itself: one file per domain in the directory Traefik watches,
rewritten on every reconcile pass and removed when the domain goes away.

```bash
levelrail-cli proxy setup             # dry run: what was detected and what would change
levelrail-cli proxy setup --confirm   # apply
levelrail-cli proxy status            # per-domain state, certificate, checklist
```

Setup, in order:

1. **Detect.** Reads the container publishing host port 80 or 443 through the
   Docker API (no `docker` CLI): its arguments, mounts, published ports and
   extra hosts. A mounted `traefik.yml` is read instead of the arguments when
   Traefik loads one. Nothing is defaulted: anything it cannot determine is
   listed and setup stops with `409` and the exact gap.
2. **TLS upstream.** Turns on `tls_terminated_upstream` and sets
   `public_https_port` to the host port the proxy publishes for HTTPS. This
   instance stops requesting certificates and stops redirecting HTTP itself.
3. **Routes.** Saves the detected values and writes the route files.
4. **Verify.** For every route: a TLS handshake to this server's public
   address with the domain as SNI (issuer and expiry of the certificate the
   proxy serves), then a request with the domain as `Host`. When Traefik's API
   is published without authentication, it also asks Traefik whether the
   router is loaded.

Running setup again changes nothing. Turn it off with `levelrail-cli proxy
disable`, which removes the files it wrote and keeps everything else.

### What is detected on Coolify

Coolify starts `coolify-proxy` with:

```text
--entrypoints.http.address=:80
--entrypoints.https.address=:443
--providers.file.directory=/traefik/dynamic/
--certificatesresolvers.letsencrypt.acme.httpchallenge=true
extra_hosts: host.docker.internal:host-gateway
volume: /data/coolify/proxy -> /traefik
```

which gives:

| Setting | Value | From |
|---|---|---|
| `dynamic_dir` | `/data/coolify/proxy/dynamic` | the file provider directory mapped through the mount |
| `entrypoint_http` / `entrypoint_https` | `http` / `https` | the entrypoints published on host ports 80 and 443 |
| `cert_resolver` | `letsencrypt` | the only certificate resolver |
| `upstream_host` | `host.docker.internal` | the extra host; its target (Docker's host gateway) is resolved to validate the listeners |

Each domain gets `/data/coolify/proxy/dynamic/levelrail-managed-<domain>.yaml`:
an HTTPS router with the certificate resolver, an HTTP router that redirects to
HTTPS, and a service pointing at the ingress HTTP port (app domains) or the
dashboard listener (the primary domain). Coolify lists these files next to its
own `coolify.yaml` and `default_redirect_503.yaml`; leave them to Levelrail.

### When the listener is on loopback

The proxy runs in a container, so it cannot reach a listener bound to
`127.0.0.1`. Setup then stops with `upstream_unreachable` and the exact
drop-in, for example:

```ini
# /etc/systemd/system/levelrail.service.d/proxy-upstream-ingress.conf
[Service]
Environment=APP_INGRESS_HTTP_ADDR=172.17.0.1:8088
```

then `systemctl daemon-reload && systemctl restart levelrail`. That address is
internal to the host, not reachable from the internet.

### Safety model

The control plane writes into a directory another product owns, so it is
strict about what it touches:

- Only files named `levelrail-managed-*.yaml` whose first line is
  `# managed by levelrail`. A file with the prefix but without the header is
  reported and never overwritten or deleted.
- Symlinks are never followed; the directory is resolved once and re-checked
  before every change, and a directory that resolves outside its parent is
  refused.
- Writes go to a temporary file in the same directory, then `rename`, mode
  `0644`, so Traefik never reads a half-written file.
- Domains are validated strictly: no wildcards, ports, paths or separators,
  and a domain that would need rewriting to become a file name is rejected.
- If a route cannot be rendered (for example the upstream became unreachable),
  the existing file is kept rather than deleted.
- Every write and removal is audited as `proxy_route.written` and
  `proxy_route.removed`.
- If the control plane cannot write the directory (another user, a read-only
  mount or a systemd sandbox), setup reports `not_writable` with the uid, the
  owner and mode of the directory, or the `ReadWritePaths=` line to add.

### Settings

`PUT /api/v1/settings/proxy-integration` sets them by hand, for a Traefik whose
directory is not on a host mount detection can see, or with a TOML static file:

```json
{ "integration": "traefik_file", "dynamic_dir": "/srv/traefik/dynamic",
  "entrypoint_http": "web", "entrypoint_https": "websecure",
  "cert_resolver": "le", "upstream_host": "172.18.0.1" }
```

`levelrail-cli proxy setup --dynamic-dir DIR` covers the common case of a
directory detection could not map.

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
levelrail-cli proxy --app web                        # snippet for an app's domain (upstream: the ingress HTTP port)
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

`GET /api/v1/system/reverse-proxy?domain=D&proxy=traefik&verify=true`, or
`?app=NAME` for an app's domain.

Managed routes: `GET /api/v1/system/proxy-integration` (detection, settings,
ingress, per-domain state and the setup checklist), `POST .../setup` with
`{"confirm": true}`, `POST .../apply`, `POST .../verify?domain=D`, and
`PUT /api/v1/settings/proxy-integration`. Reading needs the read ability,
every write needs root. `APP_PROXY_VERIFY_TIMEOUT` bounds each probe
(default `8s`).
The port and mode above are `public_https_port` and `tls_terminated_upstream` on
`PUT /api/v1/settings/ingress`.
The check only probes a domain that resolves to this server's own public
address.
