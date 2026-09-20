# Domains and ingress: you don't set up a reverse proxy

If you're coming from a platform that requires you to install and wire up your own Traefik or nginx container, stop: Levelrail works differently. There is nothing to install for ingress and nothing separate to keep running.

## Why there's no proxy to install

Levelrail's control plane embeds [Caddy](https://caddyserver.com/) directly as a Go library (`internal/ingress`). It drives Caddy through its admin API, not via a hand-written Caddyfile. Config comes from the app specs the reconciler already knows about.

There is no `caddy` binary and no separate ingress container to start, restart, or misconfigure.

### How this differs from competitors

Coolify and Dokploy both run Traefik as a separate, long-lived container alongside the control plane. That separation means:

- A second process with its own config surface and restart semantics.
- Risk that ingress config drifts out of sync with the platform's desired state.

Levelrail removes that failure mode by design. One process, one source of desired state (the reconciler and database), and no way for ingress config to diverge from it. See [ADR 005](../adr/005-caddy-embedded-ingress.md) and [docs/comparison.md](comparison.md) for the full reasoning.

### What you actually do

Write `domains:` in your app's `app.yaml` and deploy. Routing and certificates happen automatically. No proxy config file, no extra container, nothing extra to monitor.

## How domain routing actually works

Add the `domains` field to a service in `app.yaml`:

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

`domains` is a list of public hostnames routed to that service. Each domain can only be claimed by one service across your whole spec (see [docs/app-spec-reference.md](app-spec-reference.md)). When you deploy, the control plane builds a Caddy route that matches incoming requests by `Host` header and reverse-proxies them to the service's container. Static sites (`build.type: static`) skip the container entirely and are served by Caddy from disk.

### Setting up DNS

Create an **A record** (or AAAA for IPv6) for the domain, pointing at your server's public IP address. This tells the internet where `app.example.com` should go. DNS resolution happens before any request reaches your server, so this is no different from any other platform.

Check that DNS has propagated:

```
dig +short app.example.com
```

Run this from a machine outside your own network. Once it resolves and the app is deployed, Caddy automatically starts routing and issuing certificates (depending on your TLS configuration below). No manual reload needed.

### Changing domains after deploy

Add or change a domain either by:

- Editing `domains:` in `app.yaml` and redeploying, or
- Using the dashboard's per-app **Domains** tab to add a domain inline and view DNS and certificate status

List all domains currently routed:

```
levelrail-cli domains list
```

## Zero-config URL: no domain, no DNS record, still HTTPS

Deploying an app without `domains:` doesn't leave it reachable only at `host:port`. When `APP_PUBLIC_HOST` is set to your server's real, publicly routable IP address (not a private/LAN address), every app with no domain gets an automatic [sslip.io](https://sslip.io) hostname:

```
<app-name>.<ip-with-dots-as-dashes>.sslip.io
```

sslip.io is a public DNS service that resolves any dash-encoded IP straight to that IP. No DNS record to create, nothing to wait for.

This gets the same TLS treatment as any other domain (see below): Caddy's internal issuer by default, or a real Let's Encrypt certificate once ACME is enabled. There's no separate toggle, and the security surface is no larger than `host:port` already exposed.

### Finding and using the zero-config URL

View your app's fallback URL on the **Network** tab or via:

```
levelrail-cli apps network <name>
```

It disappears the moment you add a real domain. A configured domain is always preferred over the synthetic one.

If `APP_PUBLIC_HOST` is not set, is a private IP, or is a hostname rather than a public IP literal, this feature is skipped entirely.

## TLS: what's actually shipped today

### Default: self-signed certificates

Out of the box, every routed domain gets a certificate from Caddy's internal, self-signed issuer. This works immediately:

- No DNS propagation needed.
- No outbound connectivity to a certificate authority.
- No configuration required.

The tradeoff: browsers and HTTP clients show a trust warning until you accept the certificate or switch to a real issuer. Use self-signed for internal tools, staging environments, or first local trials, not for public-facing apps.

### Real public ACME (Let's Encrypt or RFC 8555 CA)

A Caddy ACME issuer is built and wired end-to-end. Enable it under **Settings > Domains** (`ACMEEnabled`, backed by `GET/PUT /api/v1/settings/ingress`). Form validation for account email and optional directory URL are included.

::: warning
This feature is built and unit-tested, but NOT verified issuing a real certificate against a real domain over the public internet yet. If you want real public certificates now or are willing to be the first to verify this, follow [docs/acme-verification-runbook.md](acme-verification-runbook.md) step by step. Don't assume "toggle exists" means "proven at internet scale" until confirmed.
:::

### HSTS (HTTP Strict Transport Security)

Set `APP_ENABLE_HSTS=true` on the control plane to send `Strict-Transport-Security` on every response. HSTS defaults to off on purpose.

::: warning
HSTS tells browsers to refuse plain HTTP and refuse certificate warnings on this host for 180 days. Enabling it before ACME is working (while still on self-signed certificates) can lock you out of your own dashboard. Only enable it once real, browser-trusted certificates are issuing.
:::

### Bring your own certificate

For domains ACME can't reach (internal-only hosts, externally issued wildcards, or certs provisioned before DNS cuts over), upload your certificate and key:

```
PUT /api/v1/apps/{name}/domains/{domain}/tls-cert
levelrail-cli domains tls-cert set
# or via the dashboard's per-domain TLS control
```

Caddy loads it directly and skips automatic issuance for that host.

## Wildcard domains: DNS-01 providers

Wildcard domains like `*.example.com` need ACME's DNS-01 challenge (HTTP-01 cannot validate wildcards). DNS-01 works by creating a short-lived TXT record at your DNS provider, so you must grant API access.

Two providers are supported. Configure them platform-wide under **Domains** in the dashboard or via CLI. Only one can be active per reconcile pass; if both are enabled, Cloudflare takes precedence.

### Cloudflare

Use an API token scoped to `Zone:DNS:Edit` for the zone containing your wildcard domains. Never use the global API key.

```
GET/PUT/DELETE /api/v1/settings/cloudflare-dns
levelrail-cli domains cloudflare-dns get|set|clear
```

### Route53

Use an AWS IAM access key pair scoped to:

- `route53:ChangeResourceRecordSets`
- `route53:ListResourceRecordSets`
- `route53:GetChange`

Apply the scopes to the target hosted zone. Region and hosted zone ID are optional; the AWS SDK resolves these from its default chain and by matching the domain against your zones.

```
GET/PUT/DELETE /api/v1/settings/route53-dns
levelrail-cli domains route53-dns get|set|clear
```

### Security and extensibility

Credentials are envelope-encrypted at rest (`internal/secrets`) and never returned in plaintext by GET. Settings responses only report whether a credential is present, not its value. The provider abstraction (`internal/ingress.DNS01Provider`) supports additional providers beyond these two.

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

**Dashboard:** Each domain's row in the **Domains** tab has an "Add WAF / rate limit" control.

**API:**

```
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/waf
```

**CLI:**

```
levelrail-cli domains waf get|set|clear <app> <domain>
levelrail-cli domains waf set my-app my-app.example.com --waf --mode detect --rps 20 --burst 50
```

### Not included in v1

- No UI for custom CRS exclusion or override rules.
- No per-path or per-route rate-limit scoping (whole-domain only).
- No separate WAF/rate-limit event log (use Caddy's access log).

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

- Target must be an absolute URL, e.g. `https://example.com` or `https://newapp.example.com/promo`.
- Bare hostnames, relative paths, and non-HTTP(S) schemes are rejected.

### Interaction with maintenance mode

If a domain has both a redirect and maintenance mode configured, maintenance mode takes precedence. The domain serves the maintenance response instead. Maintenance mode is an active operator decision; a redirect can be a stale leftover from an old migration. Clear maintenance mode to let a configured redirect take effect.

### Configuration

**Dashboard:** Each domain's row in an app's **Domains** tab has an "Add redirect" control.

**API:**

```
GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/redirect
```

**CLI:**

```
levelrail-cli domains redirect get|set|clear <app> <domain>
levelrail-cli domains redirect set my-app www.example.com --target https://example.com
```

## Custom error pages

Replace Caddy's bare default error text, or whatever the backend itself
returned, with your own HTML for a domain, for a fixed set of status
codes: `404`, `500`, `502`, and `503`. No new container, no new infra
dependency: this is `reverse_proxy`'s own `handle_response` mechanism
(catches a status the backend actually returns) plus a wrapping
`subroute`'s error handling (catches a real proxy failure, e.g. the
container being unreachable, which Caddy turns into its own 502/504
before a response from the backend ever exists to match against).
Either way the original status code is preserved; only the body changes.

- **A domain can have more than one mapping at once**: e.g. a custom
  `404` and a separate custom `503` "this app is temporarily down" page,
  configured independently.
- **Only these four codes are supported.** This is deliberately not a
  generic arbitrary-status-code system: 404, 500, 502, and 503 cover the
  cases an operator actually wants a custom page for (a real not-found,
  an application error, and the container being unreachable).
- **Where to configure it:**
  - Dashboard: each domain's row in an app's **Domains** tab has an "Add
    error page" control, letting you pick a status code and paste in
    HTML.
  - API: `GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/error-pages`.
    PUT upserts one status-code-to-body mapping per call; DELETE takes an
    optional `?status_code=` to remove a single mapping, or removes every
    mapping for the domain when omitted.
  - CLI: `levelrail-cli domains error-pages get|set|clear <app> <domain>`,
    e.g. `levelrail-cli domains error-pages set my-app my-app.example.com --code 404 --body-file 404.html`.

- **What this doesn't do.** There's no live preview in the dashboard, no
  templating or variable interpolation inside the HTML you provide (it's
  served byte-for-byte), and no per-app default separate from the
  per-domain mapping. All of that is a real gap, not a hidden default;
  the small, fixed-status-code surface here is deliberately the whole v1
  scope.

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
2. Open `80/tcp` and `443/tcp`
3. Enable `ufw` (if not already active)

If `ufw` was already active, it just adds the rules without re-enabling it.

If you don't set this flag, open these ports manually via your cloud provider's firewall, `ufw`, or `iptables`.

## Walkthrough: your first domain, from install to HTTPS

This assumes you already have the control plane running and an app deployed (see [docs/getting-started.md](getting-started.md)).

1. **Point DNS at your server.**

   Create an A record for your domain pointing at your server's public IP. Verify it resolves from outside your network:

   ```bash
   dig +short my-app.example.com
   ```

2. **Add the domain to your app.**

   Option A: Edit `app.yaml` and redeploy:

   ```yaml
   services:
     web:
       domains:
         - my-app.example.com
       port: 3000
   ```

   ```bash
   APP_API_TOKEN=dev-root-token ./levelrail-cli apps deploy your-app --file app.yaml
   ```

   Option B: Open the app's **Domains** tab in the dashboard and add it directly (no redeploy needed).

3. **Open ports 80 and 443.**

   Either re-run `install.sh` with `LEVELRAIL_CONFIGURE_UFW=1`:

   ```bash
   LEVELRAIL_CONFIGURE_UFW=1 ./install.sh
   ```

   Or manually open ports 80 and 443 via your cloud provider's firewall, `ufw`, or `iptables`.

4. **Check certificate status.**

   By default, your app is now reachable over HTTPS with a self-signed certificate. Browsers will warn until you trust it. For a real, browser-trusted certificate, go to **Settings > Domains**, enable ACME, and follow [docs/acme-verification-runbook.md](acme-verification-runbook.md).

5. **Test the connection.**

   ```bash
   curl -v https://my-app.example.com
   ```

   Or visit it in a browser from outside your network.

Done. No proxy container to configure anywhere.
