---
title: Run behind an existing proxy
description: "Run Levelrail behind the proxy that already owns ports 80 and 443, with managed routes for Traefik (including Coolify) and Nginx."
"og:url": "https://docs.levelrail.com/behind-an-existing-proxy"
---

On a server that already runs Coolify, Dokploy or your own nginx, something else
owns ports 80 and 443. Levelrail doesn't fight it. It detects what holds the ports
and generates the configuration to put the dashboard behind it.

When the apps are staged and verified, [take over this server's ports](/migrate-same-server#4-take-over-this-servers-ports) in one guided, reversible step.

## Choose your proxy

Each proxy is a provider. Detection finds the one holding ports 80 and 443 and
`--provider auto` (the default) uses it; you can also pick one explicitly.

| Provider | What Levelrail does | Status |
|---|---|---|
| Built-in Caddy | Serves ports 80 and 443 and issues certificates itself. Nothing is written to another proxy. | built in |
| Traefik | Writes one route file per domain into the file provider directory. Traefik watches the directory, so no reload is needed. | managed |
| nginx | Writes one `.conf` per domain into `conf.d` or `sites-enabled`, checks it with `nginx -t`, then reloads nginx. See [nginx](#nginx-managed). | managed |
| Caddy (external) | Renders a Caddyfile snippet. Nothing is applied. | manual |
| HAProxy | Renders a frontend rule and a backend. Nothing is applied. | manual |

In the dashboard, open Network, then Traffic, and use the Proxy card:

1. Pick a provider. The detected one is marked.
2. Press Dry run to see the files that would be written (managed) or the
   snippet to add by hand (manual). Dry run changes nothing.
3. Press Apply and verify. Levelrail writes the files, validates and reloads
   the proxy where needed, then probes every domain over HTTPS. Manual
   providers have no Apply button: add the snippet yourself, then press
   Re-check.
4. Press Re-check any time to read the proxy again. "Fix via CLI" under the
   buttons shows the matching command.

The CLI does the same: `levelrail-cli proxy setup --provider nginx` is a dry
run, add `--confirm` to apply. `proxy status`, `proxy apply`, `proxy verify`
and `proxy disable` work for every managed provider.

## One step setup (Traefik, including Coolify)

When the proxy is Traefik with a file provider directory, Levelrail can manage
the routes itself: one file per domain in the directory Traefik watches,
rewritten on every reconcile pass and removed when the domain goes away.

```bash
levelrail-cli proxy setup             # dry run: what was detected and what would change
levelrail-cli proxy setup --confirm   # apply
levelrail-cli proxy status            # per-domain state, certificate, checklist
```

Traefik loads a written file a moment after it appears. `setup --confirm` and
`proxy verify` therefore wait (up to `APP_PROXY_VERIFY_TIMEOUT`, default `8s`)
for the router to show up, and a route that is still not loaded is reported as
the pending step "waiting for the proxy to load the route", not as an error.
Run `proxy verify` again once Traefik has reloaded.

Setup, in order:

1. Detect. It reads the container publishing host port 80 or 443 through the
   Docker API (no `docker` CLI): its arguments, mounts, published ports and
   extra hosts. A mounted `traefik.yml` is read instead of the arguments when
   Traefik loads one. Nothing is defaulted: anything it cannot determine is
   listed and setup stops with `409` and the exact gap.
2. TLS upstream. It turns on `tls_terminated_upstream` and sets
   `public_https_port` to the host port the proxy publishes for HTTPS. This
   instance stops requesting certificates and stops redirecting HTTP itself.
3. Routes. It saves the detected values and writes the route files.
4. Verify. For every route it runs a TLS handshake to this server's public
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

The proxy runs in a container, so it can't reach a listener bound to
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

## nginx (managed)

For an nginx container that owns ports 80 and 443, Levelrail writes one file
per domain and keeps your own nginx configuration untouched.

```bash
levelrail-cli proxy setup --provider nginx             # dry run
levelrail-cli proxy setup --provider nginx --confirm   # apply
```

What each file holds: an HTTP server that redirects to HTTPS (and serves the
ACME challenge from the certbot webroot when one is mounted), and a TLS server
that proxies to this instance with WebSocket upgrade, `Host` and
`X-Forwarded-*` headers.

Detection reads the container through the Docker API and never guesses. It
lists a gap, and setup stops with the exact text, when:

- neither `/etc/nginx/conf.d` nor `/etc/nginx/sites-enabled` is on a host mount
  (pass `--dynamic-dir`);
- a readable `nginx.conf` does not include that directory;
- nothing is mounted at `/etc/letsencrypt`, certbot's layout, so the certificate
  paths are unknown (pass `--cert-dir`, the directory inside the container that
  holds `<domain>/fullchain.pem` and `privkey.pem`);
- container port 80 or 443 is not published, or the way the container reaches
  this host cannot be read.

Files follow the same safety model as Traefik (header `# managed by levelrail`,
never touching a file without it), with these additions:

- After writing, Levelrail runs `nginx -t` and, only when it passes,
  `nginx -s reload`, both through the Docker Engine API (no `docker` CLI).
- If `nginx -t` fails, every change is rolled back, nothing is reloaded, and the
  API, CLI and dashboard report nginx's own error text. Saved settings and
  `tls_terminated_upstream` are left as they were.
- Turning the integration off removes the files and reloads nginx.

## When the proxy you chose is not running

Pick Traefik or nginx on Network, Proxy while nothing of that kind holds ports 80
and 443 and the page shows "Traefik is not running on this server yet" with three
choices instead of an error:

- **Install and start Traefik for me.** Shows the exact plan first (image,
  ports, mounts and the static configuration file), then starts the container
  only after you confirm. It uses the Docker Engine API, never the docker CLI.
  The container is named with the brand short name prefix, carries a label that
  marks it as managed by this control plane, restarts unless stopped and
  publishes ports 80 and 443. The route directory and `traefik.yml` live under
  the data directory (`proxy/traefik/`). A Let's Encrypt resolver is written
  only when public certificates are already enabled. Afterwards detection runs
  again and the route directory is filled in for you. Running it twice does not
  create a second container, and a container this control plane did not create is
  never stopped, removed or reused.
- **Go back to the built-in Caddy.** Switches the provider back. Nothing is
  deleted.
- **It runs somewhere else.** Shows the container name, route directory,
  entrypoints, certificate resolver and upstream host with defaults and inline
  checks, plus Re-check.

The install refuses, with a specific message and no change, when port 80 or 443 is
already in use (including by the built-in Caddy, which this switch does not hand
over), when the control plane itself runs inside Docker, or when a container with
that name already exists. Automatic install is Traefik only: nginx needs
certificates you provide.

Image: `APP_PROXY_TRAEFIK_IMAGE` overrides the pinned official image.

```bash
levelrail-cli proxy install                # dry run: the exact plan
levelrail-cli proxy install --confirm      # create and start the container
```

`POST /api/v1/system/proxy-integration/install` with `{"provider": "traefik",
"confirm": false}` returns the plan; with `confirm: true` it applies it through the
typed fix `proxy.install.traefik` (audited, root only) and answers `409` with a
`code` such as `port_in_use` when it refuses.

## From the dashboard

Open Domains, then "Put the dashboard behind your existing proxy". Enter the domain,
check the detected proxy, and copy the configuration. "Check it works"
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

## Certificates once a proxy owns TLS

When TLS is terminated by the proxy, the platform reports one certificate state
for each domain: the certificate the proxy actually serves (issuer, expiry,
"renews automatically"), read from the last route verification. Nothing that
this server stored earlier is shown or alerted on:

- A stale certificate request failure from an earlier attempt, the "another
  service answers port 80" guidance and the DNS-01 provider prompt are hidden,
  and the recorded failure is cleared when you enable proxy mode.
- The internal certificate this server stored before the proxy took over is not
  listed under **Settings > Certificates**, in the security checks or in the
  domain doctor, and `cert_expiring`, `cert_expiry` and `cert_renewal_stalled`
  alert rules ignore it.
- `GET /api/v1/domains` and `GET /api/v1/certificates` carry a `tls_owner`
  field (`acme`, `internal`, `proxy`, `custom`); `levelrail-cli domains
  certificates` prints it as the OWNER column.

If the proxy certificate has not been seen yet, run **Verify** on the proxy
page (or `levelrail-cli proxy --verify`).

## Traffic controls behind a proxy

Maintenance mode, basic auth, the auth gate, the WAF and rate limit, redirects
and custom error pages are all applied by the built-in ingress. Behind a proxy
they work only when the proxy really hands the domain's requests to that
ingress. A route file being written is not enough: another router on the same
proxy (for example a container label left over from Coolify) can win the match
and send visitors straight to an old container.

Every domain in `GET /api/v1/domains` (and every entry of `enforcement` in
`GET /api/v1/apps/{name}/environment-domains`) carries an `enforcement` block:

```json
"enforcement": {
  "mode": "proxy:traefik",
  "maintenance": false, "basic_auth": false, "auth_gate": false,
  "waf": false, "redirect": false, "error_pages": false,
  "reason": "route_unverified"
}
```

| Mode | When | Maintenance | Basic auth | Auth gate | WAF and rate limit | Redirect | Error pages |
|---|---|---|---|---|---|---|---|
| `levelrail` | The built-in ingress serves the domain directly | yes | yes | yes | yes | yes | yes |
| `proxy:traefik` | A managed route file exists and the last **Verify** proved the requests reach the built-in ingress | yes | yes | yes | yes | yes | yes |
| `proxy:traefik` | A managed route file exists but delivery is not proven (`reason: route_unverified`) | no | no | no | no | no | no |
| `proxy:external` | TLS is terminated by a proxy this server does not manage (nginx, Caddy, a hand written Traefik file) (`reason: proxy_unmanaged`) | no | no | no | no | no | no |
| `none` | The domain is not routed yet (`reason: not_routed`) | no | no | no | no | no | no |

"no" means the platform cannot confirm it, so the dashboard shows the saved
setting as **Saved, not enforced** with the reason, never as active. The
setting itself is kept and starts working as soon as delivery is proven.

To get a managed Traefik route to `yes`, open the proxy page and run
**Verify** (or `levelrail-cli proxy --verify`). Verification sends a request
through the proxy that only the built-in ingress answers; when it comes back,
the route is proven. Managed routers are written with a high priority so they
win over label routers for the same host. If verification still fails, look
for another router on the proxy that claims the host with an even higher
priority.

For nginx or Caddy in front, point the site at the ingress HTTP listener as
shown in the generated snippet. The controls then apply, but the platform
cannot verify it and keeps reporting them as not enforced.

## Links and the public HTTPS port

When a proxy owns 80 and 443 and forwards to this ingress on its own ports
(for example `8088` and `8443`), the certificate lives on the proxy and clients
reach the proxy on 443. Set **Public HTTPS port** under **Domains > Platform
ingress** (or `levelrail-cli settings ingress set --public-https-port 443`) so
every link the platform builds (automatic URLs, preview URLs, emailed links)
uses the port clients actually use. `443` produces links with no port suffix,
any other value is appended, and `0` keeps the ingress listen port.

## Several instances behind one proxy

Each instance can't own 80 or 443 and must not run ACME. Give every instance
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

## Troubleshooting

### Domains served by the proxy are not routing correctly

Requests to the domain arrive at the proxy but do not reach the right Levelrail app.

Likely cause: the proxy's route rule does not match the Host header, or Levelrail is not listening on the port the proxy is forwarding to.

How to confirm: check the proxy configuration (Traefik, nginx, Caddy) and verify it has a route for the domain pointing to the correct Levelrail port (e.g., 8088). Also check that Levelrail is actually listening on that port with `sudo ss -tlnp | grep 8088`.

Fix: verify Levelrail is running and listening on the correct port (set with `APP_INGRESS_HTTP_ADDR` or via the installer). Then add or update the proxy's route to forward the domain to that port. Refer to [Example proxy configs](/behind-an-existing-proxy#example-proxy-configs) above for the correct syntax for your proxy.

### The dashboard shows mixed HTTP/HTTPS content

The browser blocks some resources because they are loaded over HTTP while the page came over HTTPS.

Likely cause: APP_INGRESS_TRUSTED_PROXIES is not set, so Levelrail does not trust the `X-Forwarded-Proto` header from the proxy and thinks requests are coming over HTTP.

How to confirm: open the browser's developer console (F12) and check the Network tab for mixed content warnings.

Fix: open Domains, then Platform settings, then "Request limits and trusted proxies", and press the Fix button. Levelrail reads the forwarding headers of your own request to name the proxy (Cloudflare, Traefik, Coolify or another) and saves its address as the trusted proxy list; "Re-check" confirms it. The CLI equivalent is `levelrail-cli settings edge fix`. A proxy on a private or loopback address is trusted as `private_ranges`; a public address is trusted as that one address, and Cloudflare's public edge is never guessed, so enter its published ranges by hand. To do it by hand instead, set `APP_INGRESS_TRUSTED_PROXIES` to the IP range or address of your proxy; the environment variable always wins over the saved value. For example, `APP_INGRESS_TRUSTED_PROXIES=10.0.0.0/8` for a private network, or the proxy's specific IP if it is static. After setting it, restart Levelrail: `sudo systemctl restart levelrail`. Once Levelrail trusts the proxy's `X-Forwarded-Proto` header, it will serve links over HTTPS.

### The dashboard is unreachable from outside but works on the server

The proxy is not forwarding the dashboard address to Levelrail, or Levelrail is not listening on the right address.

Likely cause: the proxy's configuration does not include the dashboard domain, or Levelrail's dashboard port (default 8080) is only listening on localhost.

How to confirm: check the proxy's route configuration to see if the dashboard domain is included. Also check Levelrail's systemd unit with `systemctl cat levelrail` and look for `APP_HTTP_ADDR` (should not be `127.0.0.1` when running behind a proxy).

Fix: add a proxy route for the dashboard domain pointing to Levelrail's dashboard port (8080 by default, or the port set with `APP_HTTP_ADDR` or `LEVELRAIL_DASHBOARD_PORT` during install). Set `APP_HTTP_ADDR=127.0.0.1:8080` (local only, since the proxy is the external listener) if it is not already. The installer normally handles this for coexist mode; if you set up the proxy afterward, manually add the route.

Still stuck? Check the [Installing behind an existing proxy](/installing#server-already-runs-a-proxy) section or attach your proxy config to an issue on [GitHub](https://github.com/glincker/levelrail/issues).

## API

`GET /api/v1/system/reverse-proxy?domain=D&proxy=traefik&verify=true`, or
`?app=NAME` for an app's domain.

Managed routes: `GET /api/v1/system/proxy-integration` (detection, settings,
ingress, per-domain state and the setup checklist), `POST .../setup` with
`{"confirm": true, "provider": "nginx"}` (`provider` is `traefik`, `nginx`,
`caddy`, `haproxy` or `auto`; `cert_dir` and `dynamic_dir` fill gaps detection
reports; without `confirm` it is a dry run, and `caddy` or `haproxy` answer with
a `snippet` and `manual: true`). The GET answer lists the `providers` and the
selected `provider`. A proxy that refuses written files answers `409` with code
`proxy_rejected` and its own error text. Also `POST .../apply`, `POST .../verify?domain=D`, and
`PUT /api/v1/settings/proxy-integration`. Reading needs the read ability,
every write needs root. `APP_PROXY_VERIFY_TIMEOUT` bounds each probe
(default `8s`).
The port and mode above are `public_https_port` and `tls_terminated_upstream` on
`PUT /api/v1/settings/ingress`.
The check only probes a domain that resolves to this server's own public
address.
