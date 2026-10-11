---
description: Automatic TLS, domain routing, WAF, rate limiting, and redirects, all embedded in the control plane, with no separate proxy to manage.
---

# Domains and ingress

Levelrail has nothing to install for ingress. The control plane embeds [Caddy](https://caddyserver.com/) as a Go library (`internal/ingress`) and drives it through its admin API, not a hand-written Caddyfile. Config comes from the app specs the reconciler already knows about, so there is no `caddy` binary, no separate proxy container, and no way for ingress config to drift from desired state. The reasoning is in [ADR 005](../adr/005-caddy-embedded-ingress.md) and the [comparison](comparison.md).

To serve an app on a domain, write `domains:` in its `app.yaml` (or add the domain in the dashboard) and deploy. Routing and certificates follow automatically.

<InlineToc default-open />

## How domain routing actually works

Requests to any configured domain flow through a decision chain:

```mermaid
flowchart TD
  A["Request arrives<br/>with Host header"] --> B{"Maintenance<br/>mode on?"}
  B -->|Yes| C["Serve maintenance<br/>page"]
  B -->|No| D{"Redirect<br/>configured?"}
  D -->|Yes| E["Issue redirect<br/>301/302"]
  D -->|No| F{"WAF<br/>enabled?"}
  F -->|Yes, match| G{"Mode =<br/>detect?"}
  G -->|Yes| H["Log match,<br/>allow request"]
  G -->|No| I["Block request<br/>with 403"]
  F -->|No or skip| H
  I --> J["Request ends"]
  H --> K{"Rate limit<br/>exceeded?"}
  K -->|Yes| L["Return 429"]
  K -->|No| M["Proxy to<br/>backend<br/>container"]
  C --> J
  E --> J
  L --> J
  M --> J
```

Add `domains` to a service in `app.yaml`:

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile
    domains:
      - app.example.com
    port: 3000
```

`domains` is a list of public hostnames routed to that service. Each domain can be claimed by only one service across your whole spec (see [App spec reference](app-spec-reference.md)). When you deploy, the control plane builds a Caddy route that matches incoming requests by `Host` header and reverse-proxies them to the service's container. Static sites (`build.type: static`) skip the container entirely and are served by Caddy from disk.

### Setting up DNS

Create an **A record** (or AAAA for IPv6) for the domain, pointing at your server's public IP address. This tells the internet where `app.example.com` should go. DNS resolution happens before any request reaches your server, so this is no different from any other platform.

Check that DNS has propagated:

```
dig +short app.example.com
```

Run this from a machine outside your own network. The control plane can also check for you: `levelrail-cli domains check <app> <domain>` reports whether the domain currently resolves to this control plane's advertised address. Once it resolves and the app is deployed, Caddy starts routing and issuing certificates (depending on your TLS configuration below). No manual reload is needed.

### Managing DNS records from the dashboard

If you've connected Cloudflare DNS or Route53 DNS (see [Wildcard domains](#wildcard-domains-dns-01-providers) below for how to enable either one), a domain's own Domains tab also gets a **DNS records** panel: the actual A/AAAA/CNAME/TXT/MX/SRV/CAA records in that domain's zone, listed, added, edited, and deleted without leaving the dashboard, each with a live resolved/pending/mismatch check against its configured value. This reuses the same provider credentials entered for wildcard ACME, no separate API token needed. NS and SOA records are read-only (provider-managed) and never appear in this view.

The zone shown is a best-effort guess, a domain's last two labels (`app.example.com` -> `example.com`), which is wrong for multi-label public suffixes like `.co.uk`; the panel always echoes the exact zone it queried so you can tell at a glance.

The CLI covers the same three operations:

```
levelrail-cli domains dns list <app> <domain>
levelrail-cli domains dns add <app> <domain> --type A --name www --value 203.0.113.10
levelrail-cli domains dns remove <app> <domain> --type A --name www --value 203.0.113.10
```

Editing a record in place is dashboard-only: it's a delete-then-add under the hood, and the CLI exposes those two primitives directly rather than a third verb that just composes them.

### Adding or changing domains

<Tabs :items="['Dashboard', 'CLI', 'app.yaml']">
<Tab value="Dashboard">

Open the app's **Domains** tab, add the domain inline, and view its DNS and certificate status. No redeploy is needed.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli apps domains add <app> www.example.com
levelrail-cli apps domains remove <app> www.example.com
levelrail-cli domains list
```

</Tab>
<Tab value="app.yaml">

Edit `domains:` in `app.yaml` and redeploy.

</Tab>
</Tabs>

**Infrastructure > Domains** lists every domain across every app with its certificate status, plus read-only badges for WAF, redirect, maintenance mode and basic auth. Edit those settings on the owning app's Domains tab.

![Levelrail Domains page with the platform ingress settings: primary domain, ACME certificates, and HSTS](assets/screenshots/domains-list.png)

### Guided setup: domain, DNS, port, certificate

The **Guided setup** button on an app's Domains tab walks one domain through four checks before anything is saved:

1. **Domain.** Pick the environment it belongs to, and optionally route the `www` or apex twin with a redirect to the one you typed.
2. **DNS record.** The exact A, AAAA or CNAME record to add, re-checked live. DNS changes usually appear within minutes but can take a few hours depending on the record's TTL.
3. **Container port.** Whether the running container really listens on the app's port. This reads the container's socket table, so it needs exec access enabled for the app; without it the step says it could not verify.
4. **Certificate.** Which challenge applies. HTTP-01 needs the internet to reach port 80 on this server. If this server sits on a private (LAN, NAT, NAS) address, or the domain is a wildcard, only DNS-01 can work, and the step says which provider is connected (Cloudflare, Route53 or none) and where to connect one.

When a certificate request fails, the dashboard shows the certificate authority's own error next to the one step that usually fixes it: open ports 80 and 443, fix the DNS record, wait out a rate limit, or correct a CAA record. The same reason appears on the Domains list and in `domains check`.

```bash
levelrail-cli domains check <app> <domain>   # DNS, challenge, DNS-01 provider, last CA error
levelrail-cli domains connectivity           # do ports 80 and 443 answer here, is the address private
```

`domains connectivity` dials this server's advertised address from the control plane. It cannot prove a port is open to the whole internet, but a private address is decisive: HTTP-01 cannot work there.

### One app, one domain set per environment

An app can carry a separate set of domains for each environment, so you do not duplicate the app for dev, UAT and production. The app routes the set of the environment it is tagged with. An environment without its own set falls back to the app's default domains, which is why existing apps keep working unchanged.

```bash
levelrail-cli apps domains add --env dev <app> dev.example.com
levelrail-cli apps domains add --env uat <app> uat.example.com
levelrail-cli apps domains list --env all <app>
```

`--env` takes an environment id, name or kind. In the dashboard, the Domains tab shows one tab per environment, with the active one marked. Moving the app to another environment switches the routed set on the next reconcile. A hostname still belongs to exactly one app across all sets.

Cloning an environment can copy every app's domain sets under rewritten hostnames, since a hostname cannot be shared:

```bash
levelrail-cli apps environments clone <id> --new-name uat \
  --copy-domains --domain-find example.com --domain-replace uat.example.com
```

Use `--domain-prefix uat.` instead to prepend a label to every copied hostname.

### The dashboard's own domain

**Infrastructure > Domains** sets the control plane's own `primary_domain`, the one the dashboard itself is reachable at, separate from any app's `domains:` in `app.yaml`. Give it its own dedicated subdomain rather than reusing one an app already serves, the same convention CapRover uses for its panel (`captain.<domain>`): something like `console.example.com` or `panel.example.com`.

Setting the primary domain to a domain an app already owns is rejected with a `409` naming the conflicting app, so this is a real guardrail, not just a convention. Pick a domain no app uses from the start and there's nothing to collide with later.

## Hide a domain from search engines

Consoles, staging sites and internal tools should not show up in search
results. Per domain, **Hide from search engines** (domain editor), `levelrail-cli
domains search-visibility <app> <domain> --hide`, or
`PUT /api/v1/apps/{name}/domains/{domain}/search-visibility` with
`{"hidden": true}` makes the ingress:

- send `X-Robots-Tag: noindex, nofollow, noarchive, nosnippet` on every
  response, overriding the app's own value;
- answer `/robots.txt` itself with a disallow-all file, even if the app serves
  its own.

This is a request that well-behaved crawlers honour. It is not access control:
use basic auth to keep people out. The setting takes effect on the next ingress
reconcile pass, and a crawler that already indexed the page drops it on its next
visit. Domains are open to search engines by default.

## Zero DNS setup: sslip.io hostnames and one-click HTTPS

[sslip.io](https://sslip.io) is a public DNS service that resolves any dash-encoded IP straight to that IP, so `134-209-118-96.sslip.io` is `134.209.118.96` with no record to create and nothing to wait for. Levelrail builds on it so a fresh install gets working HTTPS URLs with no DNS setup at all.

### The server's public address

At first start the control plane works out its own public IP, in this order:

1. `APP_PUBLIC_HOST`, if set (an IP, or a hostname if you only need it for DNS checks). This always wins.
2. Otherwise it asks public what-is-my-IP services (`api.ipify.org`, `icanhazip.com`, `ifconfig.me`) and takes the first public answer. Private, loopback and link-local answers are ignored.

The Domains page shows the address and how it was found (`from APP_PUBLIC_HOST`, `detected`, `detection disabled`, `not found`), and `levelrail-cli settings ingress get` prints it as `public_host`.

| Variable | Default | Effect |
|---|---|---|
| `APP_PUBLIC_HOST` | unset | Override the public address. |
| `APP_PUBLIC_IP_DETECT` | on | Set to `off` to never call the outside services (air-gapped or privacy sensitive installs). |
| `APP_PUBLIC_IP_PROBE_URLS` | the three above | Comma separated list of URLs that return the caller's IP as plain text. |
| `APP_PUBLIC_IP_PROBE_TIMEOUT` | `4s` | Overall budget for detection at startup. |

If the server is behind NAT or a load balancer, detection finds the wrong address. Set `APP_PUBLIC_HOST` to the address that actually reaches ports 80 and 443 on this box.

### One-click HTTPS for the dashboard

The Domains page opens with an **Enable HTTPS** card (the setup wizard's domain step shows the same card first). Enter a contact email and click the button. The control plane:

1. sets the dashboard's primary domain to `<dashed-ip>.sslip.io`,
2. turns on real ACME issuance and asks Let's Encrypt for a certificate over the HTTP-01 challenge,
3. shows `pending`, then `issued` with the issuer and expiry, and
4. once you are looking at the dashboard on that https address, saves it as the dashboard URL (sign-in over plain HTTP is then refused).

From the CLI:

```
levelrail-cli settings ingress https enable --email you@example.com --wait 2m
levelrail-cli settings ingress https status
```

The API is `GET` and `POST /api/v1/settings/ingress/https`.

Ports 80 and 443 must be reachable from the internet. If issuance fails, the card shows the certificate authority's own error and a localized hint:

| Hint | Meaning |
|---|---|
| `unreachable` | Let's Encrypt could not connect to port 80. Open 80 and 443 in the host firewall (`ufw allow 80,443/tcp`) and in your cloud provider's security group. |
| `rate_limited` | Let's Encrypt is throttling the hostname. Wait, and use staging while debugging. |
| `dns` | The hostname did not resolve to this server. Check the detected public address. |
| `caa` | A CAA record forbids Let's Encrypt from issuing for the hostname. |

**Staging and rate limits.** Tick "Test with Let's Encrypt staging first" (or pass `--staging`) to use the staging CA: certificates are not browser trusted, but there are no meaningful limits. Production enable attempts are capped at 5 per hour per control plane (`APP_ACME_MAX_ATTEMPTS_PER_HOUR`) so a misconfiguration cannot burn Let's Encrypt's failed-validation limit; staging is never capped. `APP_ACME_STAGING=true` makes staging the default CA, and `APP_ACME_DIRECTORY_URL` points at any other RFC 8555 CA. A request that never settles is reported as failed after `APP_ACME_PENDING_TIMEOUT` (default 2 minutes).

Switching CA (staging to production, or from the internal issuer to ACME) drops the certificates stored for the previous issuer so they are re-issued. Without this, Caddy keeps serving the old still-valid certificate and never asks the new CA.

### Automatic hostnames for apps

Deploying an app without `domains:` does not leave it reachable only at `host:port`. Every app with no domain gets:

```
https://<app-name>.<dashed-ip>.sslip.io
```

It is routed by the same ingress and gets the same TLS treatment as any other domain: a Let's Encrypt certificate once ACME is enabled (the Enable HTTPS card above turns it on for every host), Caddy's internal issuer before that.

- It appears on the app's **Network** tab, in `levelrail-cli apps network <name>`, and in `levelrail-cli domains list` (source `automatic`) and `GET /api/v1/domains` (`"automatic": true`).
- It disappears the moment you add a real domain. A configured domain always wins.
- It is generated each pass and never stored on the app, so per-domain features (basic auth, WAF, BYO certificate, maintenance mode) do not apply to it. Add a real domain for those.
- **Toggle:** the **Automatic app hostnames** setting on the Domains page, or `levelrail-cli settings ingress set --fallback-domains=false`. It is off-able per server, not per app.
- It needs a public IPv4 address (detected or `APP_PUBLIC_HOST`). With a private address, a hostname, or detection turned off there is nothing to build, and the Network tab says so.

### HTTP redirects to HTTPS

When the control plane can bind the HTTP ingress port (port 80 as root, the installer's default), every routed host answers plain HTTP with a `308` redirect to the same URL over HTTPS, and Let's Encrypt's HTTP-01 challenge is served on the same listener. A non-root development run that cannot bind port 80 skips the redirect and logs why. `APP_INGRESS_HTTP_REDIRECT=true|false` forces the choice instead of auto detecting.

## TLS: what's actually shipped today

### Default: self-signed certificates

Out of the box, every routed domain gets a certificate from Caddy's internal, self-signed issuer. This works immediately:

- No DNS propagation needed.
- No outbound connectivity to a certificate authority.
- No configuration required.

The tradeoff: browsers and HTTP clients show a trust warning until you accept the certificate or switch to a real issuer. Use self-signed for internal tools, staging environments, or first local trials, not for public-facing apps.

### Real public ACME (Let's Encrypt or RFC 8555 CA)

A Caddy ACME issuer is built and wired end-to-end. Enable it with the **Issue real ACME certificates** toggle on the Domains page, or with `levelrail-cli settings ingress set --acme-enabled --acme-email you@example.com`. The API is `GET/PUT /api/v1/settings/ingress`. An account email is required, and a directory URL is optional.

::: tip Verified against a live domain
Real Let's Encrypt issuance, HTTP to HTTPS redirect and trusted-chain handshakes were run on a public VPS (`<dashed-ip>.sslip.io`, ports 80 and 443 open). The recorded run, including what failed before it worked, is in [docs/acme-verification-runbook.md](acme-verification-runbook.md#recorded-run-2026-10-05).
:::

### HSTS (HTTP Strict Transport Security)

Turn on **Enable HSTS** on the Domains page (or `levelrail-cli settings ingress set --hsts-enabled`) to send `Strict-Transport-Security` on every https response, including the dashboard's HTML page, no restart required. It is never sent over plain HTTP. HSTS defaults to off on purpose.

Setting the `APP_ENABLE_HSTS=true` environment variable still works the same way it always has, for anyone who already relies on it. The two are additive: HSTS is sent if either the dashboard toggle or the environment variable is on, so upgrading never turns HSTS off for a deployment that already had it on.

::: warning
HSTS tells browsers to refuse plain HTTP and refuse certificate warnings on this host for 180 days. Enabling it before ACME is working (while still on self-signed certificates) can lock you out of your own dashboard. Only enable it once real, browser-trusted certificates are issuing.
:::

### Bring your own certificate

For domains ACME can't reach (internal-only hosts, externally issued wildcards, or certificates provisioned before DNS cuts over), upload a certificate and key. Caddy loads it directly and skips automatic issuance for that host.

```bash
levelrail-cli domains tls-cert set <app> <domain> --cert-file fullchain.pem --key-file privkey.pem
levelrail-cli domains tls-cert get <app> <domain>
```

The API is `PUT /api/v1/apps/{name}/domains/{domain}/tls-cert`, and the dashboard has a per-domain TLS control.

## Wildcard domains: DNS-01 providers

Wildcard domains like `*.example.com` need ACME's DNS-01 challenge (HTTP-01 cannot validate wildcards). DNS-01 works by creating a short-lived TXT record at your DNS provider, so you must grant API access.

Two providers are supported. Configure them platform-wide on the **Domains** page or with the CLI. Only one is active per reconcile pass, and Cloudflare wins if both are enabled.

<Tabs :items="['Cloudflare', 'Route53']">
<Tab value="Cloudflare">

Use an API token scoped to `Zone:DNS:Edit` for your zone, never the global API key.

```bash
levelrail-cli domains cloudflare-dns set --cf-api-token <token>
levelrail-cli domains cloudflare-dns get
levelrail-cli domains cloudflare-dns clear
```

The API is `GET/PUT/DELETE /api/v1/settings/cloudflare-dns`.

</Tab>
<Tab value="Route53">

Use an AWS IAM access key pair with `route53:ChangeResourceRecordSets`, `route53:ListResourceRecordSets` and `route53:GetChange`. The region and hosted zone ID are optional.

```bash
levelrail-cli domains route53-dns set --aws-access-key-id <id> --aws-secret-access-key <secret>
levelrail-cli domains route53-dns get
levelrail-cli domains route53-dns clear
```

The API is `GET/PUT/DELETE /api/v1/settings/route53-dns`.

</Tab>
</Tabs>

### Credential storage

Credentials are envelope-encrypted at rest and never returned by GET: responses report only whether a credential is present.

## Opt-in WAF and rate limiting

Both WAF and rate limiting are Caddy modules registered at build time, not separate containers. They are off by default and configured per domain.

- WAF: OWASP Coraza with the stock OWASP Core Rule Set (`github.com/corazawaf/coraza-caddy/v2`)
- Rate limiting: Caddy's `rate_limit` module (`github.com/mholt/caddy-ratelimit`)

### WAF modes: detect or block

| Mode | Behavior | Default | Use case |
| --- | --- | --- | --- |
| `detect` | Logs OWASP CRS matches but never rejects | Yes, new domains | Monitor for false positives first |
| `block` | Actually rejects requests matching CRS rules | No | Deploy after validating detect logs |

::: warning
The OWASP CRS can produce false positives against normal app traffic. This platform's core design principle is avoiding silent failures. Enable WAF in `detect` mode first, watch logs for a while, then switch to `block` once you're confident it isn't flagging legitimate traffic. There is no auto-promote from detect to block; you flip it yourself.
:::

### Rate limiting parameters

Rate limiting is independent of WAF. Configure per client IP with two numbers:

| Parameter | What it does | Example |
| --- | --- | --- |
| `requests/sec` | Sustained cap, averaged over 10 seconds | 20 |
| `burst` | 1-second window allowance above sustained rate | 50 |

Clients exceeding either limit get a 429 response. Set burst equal to or below requests/sec for a strict, non-bursting cap.

### Configuration

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Each domain's row in an app's **Domains** tab has an **Add WAF / rate limit** control.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli domains waf get|set|clear <app> <domain>
levelrail-cli domains waf set my-app my-app.example.com --waf --mode detect --rps 20 --burst 50
```

</Tab>
<Tab value="API">

```text
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/waf
```

</Tab>
</Tabs>

::: details Not included in v1
- No UI for custom CRS exclusion or override rules.
- No per-path or per-route rate-limit scoping (whole-domain only).
- No separate WAF/rate-limit event log (use Caddy's access log).
:::

## Domain redirects

Route a domain to a different URL instead of proxying to a container. Use cases:

- `www.example.com` to `example.com`
- `old-domain.com` to `https://newapp.example.com/promo` (during a migration)

This uses Caddy's `static_response` handler with a `Location` header and redirect status code. No new container, no new infrastructure.

### Redirect status codes

| Code | Type | Use case |
| --- | --- | --- |
| `301` | Permanent (default) | Renames, www-to-apex normalization, or expected permanent moves |
| `302` | Temporary | Redirects you plan to undo (browsers and search engines don't cache these) |

### Requirements

- The domain must already be one of the app's domains. Add it first with `levelrail-cli apps domains add <app> www.example.com`.
- Target must be an absolute URL, e.g. `https://example.com` or `https://newapp.example.com/promo`.
- Bare hostnames, relative paths, and non-HTTP(S) schemes are rejected.

A target that is only an origin, such as `https://example.com`, keeps the request path and query, so `www.example.com/pricing?plan=pro` goes to `https://example.com/pricing?plan=pro`. A target with its own path, query, or fragment always redirects to exactly that URL.

### Interaction with maintenance mode

If a domain has both a redirect and maintenance mode configured, maintenance mode takes precedence. The domain serves the maintenance response instead. Maintenance mode is an active operator decision; a redirect can be a stale leftover from an old migration. Clear maintenance mode to let a configured redirect take effect.

### Configuration

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Each domain's row in an app's **Domains** tab has an **Add redirect** control.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli domains redirect get|set|clear <app> <domain>
levelrail-cli domains redirect set my-app www.example.com --target https://example.com
```

Add `--temporary` for a `302`. The default is a permanent `301`.

</Tab>
<Tab value="API">

```text
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/redirect
```

</Tab>
</Tabs>

## Custom error pages

Replace Caddy's bare default error text, or whatever the backend returned, with your own HTML for a domain. Four status codes are supported: `404`, `500`, `502` and `503`. The original status code is kept and only the body changes. It covers both a status the backend actually returns and a proxy failure such as an unreachable container, which Caddy turns into its own 502 or 504.

- A domain can have several mappings at once, for example a custom `404` and a separate `503` "temporarily down" page.
- The HTML is served byte for byte: no templating, no variables, and no per-app default separate from the per-domain mapping.

<Tabs :items="['Dashboard', 'CLI', 'API']">
<Tab value="Dashboard">

Each domain's row in an app's **Domains** tab has an **Add error page** control. Pick a status code and paste in HTML. There is no live preview.

</Tab>
<Tab value="CLI">

```bash
levelrail-cli domains error-pages get|set|clear <app> <domain>
levelrail-cli domains error-pages set my-app my-app.example.com --code 404 --body-file 404.html
```

</Tab>
<Tab value="API">

```text
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/error-pages
```

`PUT` upserts one status code mapping per call. `DELETE` takes an optional `?status_code=` to remove one mapping, or removes all of the domain's mappings when omitted.

</Tab>
</Tabs>

## Edge limits, client IPs and failover

Every proxied domain gets the same edge policy, on by default. All of it is tuned by environment variables on the control plane (set them in the systemd unit or compose file); `APP_INGRESS_HARDENING=false` restores the bare Caddy config. `levelrail-cli doctor` shows the active policy in its `ingress_edge` check.

| Variable | Default | What it does |
| --- | --- | --- |
| `APP_INGRESS_READ_HEADER_TIMEOUT` | `10s` | Time a client gets to send its request headers. This is the slowloris limit. |
| `APP_INGRESS_READ_TIMEOUT` / `APP_INGRESS_WRITE_TIMEOUT` | `0` (off) | Whole-request deadlines. Left off so uploads, downloads and SSE streams keep working. |
| `APP_INGRESS_IDLE_TIMEOUT` | `2m` | Idle keep-alive connections are closed after this. |
| `APP_INGRESS_MAX_HEADER_BYTES` | `131072` | Largest request header block. |
| `APP_INGRESS_MAX_BODY_BYTES` | `1073741824` | Largest request body (1 GiB), answered with `413`. `0` removes the cap. |
| `APP_INGRESS_PROTOCOLS` | `h1,h2,h3` | HTTP/1.1, HTTP/2 and HTTP/3. HTTP/3 needs `443/udp` open. |
| `APP_INGRESS_RETRY_WINDOW` | `2s` | How long a request is retried when its upstream refuses connections, before the friendly 503. |
| `APP_INGRESS_DIAL_TIMEOUT` | `2s` | Time allowed to connect to a container. |
| `APP_INGRESS_PASSIVE_FAIL_DURATION` | `5s` | A replica that fails a connection is skipped for this long (pools of two or more replicas only). |
| `APP_INGRESS_UNAVAILABLE_RETRY_AFTER` | `5` | Seconds in the `Retry-After` header of the friendly 503. |
| `APP_INGRESS_GRACE_PERIOD` | `0` (`3s` with socket activation) | How long in-flight requests get to finish when the process stops. |

There is no per-IP connection limit: Caddy has none built in. Put a firewall rule or the [WAF rate limit](#opt-in-waf-and-rate-limiting) in front if you need one.

### Real client IPs behind a CDN or proxy

By default the app sees the address that connected to Levelrail, and any `X-Forwarded-For` the client sent is discarded (so it cannot be spoofed). If Levelrail itself sits behind Cloudflare, a load balancer or another proxy, tell it which peers to trust:

```bash
APP_INGRESS_TRUSTED_PROXIES=203.0.113.0/24,private_ranges
APP_INGRESS_CLIENT_IP_HEADERS=CF-Connecting-IP   # optional, defaults to X-Forwarded-For
```

`private_ranges` expands to the RFC 1918 and loopback ranges. For a trusted peer, the client address is read from the header (right to left for `X-Forwarded-For`, so a forged left-most entry is ignored), appended to `X-Forwarded-For`, and sent to the app as `X-Real-IP`. Every app receives `X-Real-IP`, trusted proxy or not. Keep the list to the proxies you actually run: anything listed can claim any client address.

### Running behind a TLS proxy

If the proxy is Traefik (Coolify's included), `levelrail-cli proxy setup` turns
this mode on and writes one route per domain into Traefik's watched directory,
kept in step with every domain you add or remove. See
[Run behind an existing proxy](behind-an-existing-proxy.md#one-step-setup-traefik-including-coolify).
The domain check then says the proxy handles certificates instead of asking
for ports 80 and 443 on this server.

If Traefik, nginx or Caddy owns 80 and 443 and forwards to this ingress, set the **Public HTTPS port** (`public_https_port`, usually `443`) so generated links omit the ingress listen port, and turn on `tls_terminated_upstream` when the proxy also holds the certificates so this instance never runs ACME. Details and a Traefik example: [Run behind an existing proxy](behind-an-existing-proxy.md#several-instances-behind-one-proxy). An ACME failure notice for non-standard ingress ports does not apply in this mode, because the proxy handles HTTPS.

### When a backend is down

A request that cannot reach its container is retried for `APP_INGRESS_RETRY_WINDOW`. With two or more replicas a refused connection moves to another replica at once and the failed one is skipped for `APP_INGRESS_PASSIVE_FAIL_DURATION`, so a killed replica costs no requests. A single-replica app, or a pool with no live replica, answers a styled `503` with `Retry-After` and a page that reloads itself, not Caddy's bare `502`. A [custom error page](#custom-error-pages) for the domain replaces it.

A domain whose app was routed in the last `APP_INGRESS_HOLD_WINDOW` (default `10m`) but has no ready container right now (a `recreate` deploy, a crash, a restart) keeps its route and its certificate and answers the same `503` instead of a TLS error or a dead connection. A domain that has never been routed since the control plane started is not held, so adding a domain before the first deploy does not start certificate issuance early.

### Surviving a control plane restart

The embedded Caddy lives in the control plane process, so a restart would close ports 80 and 443 for its length. New installs close that gap by default: systemd owns the listening sockets.

```bash
./install.sh                                          # new install, socket activation on
LEVELRAIL_SOCKET_ACTIVATION=0 ./install.sh            # new install, opt out
LEVELRAIL_SOCKET_ACTIVATION=1 ./install.sh upgrade    # switch an existing install (one short stop)
LEVELRAIL_SOCKET_ACTIVATION=0 ./install.sh upgrade    # switch back
```

An `upgrade` with the variable unset keeps whichever mode the install is already in. Docker Compose deployments are unchanged: they do not use systemd and bind the ports themselves.

The installer writes `levelrail-http.socket` and `levelrail-https.socket`. The control plane detects the inherited sockets (`LISTEN_FDS`, named `http` and `https`) on its own; no extra setting is needed. While the process is down the kernel queues new connections, and the new process serves them when it is up, so visitors see a slower response instead of a refused connection. The measured numbers are on the [resilience](resilience.md#ingress-availability-windows-measured) page, and the decision is in [ADR 025](../adr/025-ingress-restart-gap-socket-activation.md). Trade-offs: HTTP/3 and ACME HTTP-01 are not available in this mode (TLS-ALPN-01 on 443 and DNS-01 still issue certificates), and the sockets stay open while the service is stopped on purpose.

## Firewall: ports 80 and 443

For public traffic and ACME's HTTP-01 challenge to work, your server must reach the internet on **ports 80 and 443**. Port 80 is also how Let's Encrypt validates domain ownership during ACME issuance. If it's blocked, issuance fails silently even if everything else is configured correctly.

Common blockers:

- Cloud provider firewall or security group
- Home router with no port forwarding
- `ufw` in default-deny state

These block traffic from the outside while appearing fine from the server itself.

### Automatic firewall setup

`install.sh` has an opt-in flag to configure this for you:

```bash
LEVELRAIL_CONFIGURE_UFW=1 ./install.sh
```

This script will:

1. Allow SSH
2. Open `80/tcp`, `443/tcp` and `443/udp` (HTTP/3)
3. Enable `ufw` (if not already active)

If `ufw` was already active, it just adds the rules without re-enabling it.

If you don't set this flag, open these ports manually via your cloud provider's firewall, `ufw`, or `iptables`.

### Running on non-default ports

If port 443 (or 80) is already taken on the host, most often by a second control-plane instance, set the ingress listen addresses instead of fighting over the defaults:

```bash
APP_INGRESS_HTTPS_ADDR=:8443 \
APP_INGRESS_HTTP_ADDR=:8080 \
./levelrail
```

- `APP_INGRESS_HTTPS_ADDR` (default `:443`): the embedded Caddy ingress's HTTPS listener.
- `APP_INGRESS_HTTP_ADDR` (default `:80`): kept off port 80 only when real ACME (not the default self-signed issuer) is enabled for a non-wildcard domain, since that's the one path that can otherwise reach for a literal port 80 for its HTTP-01 challenge.

`GET /api/v1/system/doctor`'s port checks follow whatever you set here, so a second instance running on `:8443`/`:8080` reports those ports as owned and available, not `:443`/`:80`.

## Raw TCP streams: forwarding a non-HTTP port

Domains, WAF, redirects and error pages are all for HTTP(S). Some services are not HTTP: a Postgres instance, an SSH server, a game server. A **stream** forwards a host port to one of an app's container ports byte for byte, with no Host-header routing and no protocol awareness.

```bash
levelrail-cli apps streams create my-postgres --host-port 15432 --container-port 5432
levelrail-cli apps streams list my-postgres
levelrail-cli apps streams delete my-postgres <id>
```

The dashboard's app **Streams** tab lists, adds and removes forwards, and the API is `GET`/`POST`/`DELETE /api/v1/apps/{name}/streams`.

Streams use the same embedded Caddy through its [`layer4`](https://github.com/mholt/caddy-l4) app, not a second proxy. Creating or deleting one restarts the app automatically, because Docker cannot add a published port to a running container.

Scope: TCP only, one stream per forward, no access lists, and no TLS termination on the stream (if the backend speaks TLS, that is between the client and the backend). Multi-app and load-balanced streams are not supported.

## Traffic: routing status for every domain at a glance

**Infrastructure > Traffic** in the dashboard (`GET /api/v1/network/proxy`, `read` ability) is a flat, one-row-per-domain table: which app a domain routes to, which node that app actually runs on, whether this control plane's own embedded ingress can reach it, its port, and TLS status and issuer.

It exists for one otherwise-invisible failure: the embedded Caddy ingress reaches an app on another node only over the WireGuard mesh. If that path is down, the app's container can be perfectly healthy while its domain silently never routes. When the path is healthy the domain routes normally and shows **Reachable**. See [Multi-node: Routing to apps on remote nodes](multi-node.md#routing-to-apps-on-remote-nodes) for how the mesh bind works.

This is the fastest way to spot it. A domain in that state shows an **Unreachable** badge with the reason (with a banner at the top of the page when any exist; `levelrail-cli nodes traffic` prints the same reason) instead of only turning up as a line in `GET /api/v1/doctor`'s report. Each unreachable row carries a **Move** button straight to the same move-with-volumes flow described in [Moving an app with its volumes](multi-node.md#moving-an-app-with-its-volumes), or run the fix directly:

```bash
levelrail-cli apps set-node <app-name> <this control plane's own node id>
# or, to let auto-placement choose again:
levelrail-cli apps clear-node <app-name>
```

## Walkthrough: your first domain

This assumes the control plane is running and an app is deployed (see [Getting started](getting-started.md)).

<Steps>
<Step title="Point DNS at your server">

Create an A record for your domain pointing at the server's public IP, then check it from outside your network:

```bash
dig +short my-app.example.com
```

</Step>
<Step title="Add the domain to your app">

Run `levelrail-cli apps domains add my-app my-app.example.com`, add it on the app's **Domains** tab, or put it in `domains:` in `app.yaml` and redeploy.

</Step>
<Step title="Open ports 80 and 443">

Open them in your cloud firewall, `ufw` or `iptables`, or re-run the installer with `LEVELRAIL_CONFIGURE_UFW=1 ./install.sh`. See [Firewall](#firewall-ports-80-and-443).

</Step>
<Step title="Get a trusted certificate">

By default the domain is served over HTTPS with a self-signed certificate, so browsers warn. For a trusted one, enable ACME on the Domains page and follow the [ACME verification runbook](acme-verification-runbook.md).

</Step>
<Step title="Test it">

```bash
curl -v https://my-app.example.com
```

</Step>
</Steps>

## Next steps

<CardGroup :cols="2">
<Card title="ACME verification runbook" href="/acme-verification-runbook">

Issue and verify a real Let's Encrypt certificate.

</Card>
<Card title="Load balancing" href="/load-balancing">

Balance traffic across an app's replicas.

</Card>
<Card title="App spec reference" href="/app-spec-reference">

Configure domains in `app.yaml`.

</Card>
<Card title="Multi-node" href="/multi-node">

How ingress reaches apps on worker nodes.

</Card>
</CardGroup>
