# Domains and ingress: you don't set up a reverse proxy

If you're coming from a platform that makes you install and wire up your
own Traefik or nginx container, read this first: with Levelrail, you don't.
There is nothing to install for ingress and nothing separate to keep
running.

## Why there's no proxy to install

Levelrail's control plane binary embeds [Caddy](https://caddyserver.com/)
directly as a Go library (`internal/ingress`) and drives it in-process
through Caddy's admin API. Config goes in as a JSON document built from
the app specs the reconciler already knows about, never a hand-written
Caddyfile on disk. There is no `caddy` binary anywhere in the codebase and
no separate ingress container to start, restart, or misconfigure.

This is a deliberate architectural choice (see [ADR 005](../adr/005-caddy-embedded-ingress.md)
and [docs/comparison.md](comparison.md)), and it's a real point of
difference from Coolify and Dokploy, which both run ingress (Traefik) as
its own long-lived container alongside the control plane. That separation
is a second process with its own config surface, its own restart
semantics, and its own way to drift out of sync with what the platform
thinks is deployed. Levelrail's embedded approach removes that failure
class structurally: there is one process and one source of desired state
(the reconciler and its database), so ingress config can't fall out of
sync with a separate proxy's own copy of it.

The practical upshot for you as an operator: you write `domains:` in your
app's `app.yaml`, deploy, and the routing and certificate work happens
automatically. There's no proxy config file to write, no container to add
to your stack, and nothing extra to monitor for that specific piece.

## How domain routing actually works

Domain routing starts with the `domains` field on a service in
`app.yaml`:

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

`domains` is a list of public hostnames routed to that service. A domain
can only be claimed by one service across your whole spec (see
[docs/app-spec-reference.md](app-spec-reference.md) for the full field
reference). When you deploy, the control plane's ingress controller
builds a Caddy route that matches incoming requests by `Host` header and
reverse-proxies them to that service's running container. Static sites
(`build.type: static`) skip the container hop entirely and are served
directly by the embedded Caddy from disk.

What you need to do on the DNS side is the same as with any other
platform: create an **A record** (or AAAA for IPv6) for the domain,
pointing at your server's public IP address. That's what tells the
internet "requests for `app.example.com` go to this machine"; nothing
about Caddy being embedded changes that requirement, since DNS resolution
happens before a request ever reaches your server. Once that record has
propagated (check with `dig +short app.example.com` from a machine
outside your own network) and the app is deployed, Caddy starts routing
and, depending on your TLS configuration below, issuing a certificate for
it automatically. No manual reload or restart is needed on your end.

You can add or change a domain after the first deploy too, either by
editing `domains:` in `app.yaml` and redeploying, or from the dashboard's
per-app **Domains** tab, which lets you add a domain, see its DNS record
status inline, and see its certificate status once it's routed.
`levelrail-cli domains list` shows every domain currently routed across
your apps.

## TLS: what's actually shipped today

Be clear-eyed about where this stands, because it's easy to overstate:

- **Default: Caddy's internal issuer.** Out of the box, every routed
  domain gets a certificate from Caddy's internal, fully offline,
  self-signed issuer. This works immediately, requires no DNS
  propagation, no outbound connectivity to a certificate authority, and
  no configuration. The tradeoff is exactly what you'd expect from a
  self-signed certificate: browsers and most HTTP clients will show a
  trust warning until you either accept the certificate or switch to a
  real issuer below. This is fine for internal tools, staging
  environments, or a first local trial; it's not what you want for a
  public-facing app.

- **Real public ACME (Let's Encrypt or any RFC 8555 CA): built, not yet
  spot-checked by this project on a live domain.** A real Caddy ACME
  issuer, a settings toggle (`ACMEEnabled` under **Settings > Domains**,
  backed by `GET/PUT /api/v1/settings/ingress`), and form validation for
  the account email and an optional directory URL override are all built
  and wired end to end. It's covered by config-shape unit tests and a
  local-CA end-to-end issuance test, but as of this writing it has not
  been verified issuing a real certificate against a real domain over the
  public internet by this project's own team. If you want real public
  certificates now, or you're in a position to be the first to verify
  this against a live domain, follow
  [docs/acme-verification-runbook.md](acme-verification-runbook.md) step
  by step. Don't take "toggle exists" as "proven to work at internet
  scale" until that runbook (or your own experience) confirms it.

- **Bring your own certificate.** If ACME can't reach a domain (an
  internal-only host, an externally issued wildcard, a cert already
  provisioned before DNS cuts over), you can upload your own
  certificate and key for a specific domain
  (`PUT /api/v1/apps/{name}/domains/{domain}/tls-cert`,
  `levelrail-cli domains tls-cert set`, or the dashboard's per-domain TLS
  control). Caddy loads it directly and skips automatic issuance for that
  host.

## Opt-in WAF and rate limiting

Since Caddy already ships embedded in the control plane binary, adding a
Web Application Firewall and rate limiting is two more Caddy modules
registered at build time, not a new container or service: OWASP Coraza
(`github.com/corazawaf/coraza-caddy/v2`, running the stock OWASP Core
Rule Set) for the WAF, and Caddy's own `rate_limit` module
(`github.com/mholt/caddy-ratelimit`) for rate limiting. Both are off by
default for every domain and are configured per domain, not platform-wide.

- **WAF modes: `detect` (default) or `block`.** `detect` runs the full
  OWASP CRS rule set against every request and logs matches, but never
  rejects anything. `block` actually rejects a request that matches a
  CRS rule. **`detect` is the default for a newly enabled domain on
  purpose:** OWASP's own CRS documentation is explicit that the rule set
  can produce false positives against a given application's normal
  traffic, and this project's central risk (see the root `CLAUDE.md`'s
  "main risk" section) is exactly the class of failure where a new
  safety control silently breaks something that used to work. Turning
  the WAF on in `detect` mode first, watching logs for a while, and only
  then switching to `block` once you're confident it isn't flagging your
  own legitimate traffic is the safer rollout path. There's no
  autopromote from detect to block; you flip it yourself once you trust
  it.

- **Rate limiting is independent of the WAF.** You can rate limit a
  domain without enabling the WAF, or enable the WAF without rate
  limiting, or both. Rate limiting is keyed per client IP and takes two
  numbers: **requests/sec** (a sustained cap, averaged over a 10 second
  window) and **burst** (a short, 1 second window allowance for values
  above the sustained rate; set it equal to or below requests/sec for a
  strict, non-bursting cap). A client that exceeds either gets a 429.

- **Where to configure it:**
  - Dashboard: each domain's row in an app's **Domains** tab has an "Add
    WAF / rate limit" control with the WAF toggle, mode selector, and the
    two rate-limit fields.
  - API: `GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/waf`.
  - CLI: `levelrail-cli domains waf get|set|clear <app> <domain>`, e.g.
    `levelrail-cli domains waf set my-app my-app.example.com --waf --mode detect --rps 20 --burst 50`.

- **What this doesn't do.** There's no UI for authoring custom CRS
  exclusion or override rules, no per-path or per-route rate-limit
  scoping (it's whole-domain), and no WAF/rate-limit event log separate
  from Caddy's own access log. All of that is a real gap, not a hidden
  default; it may get filled in a later phase, but the on/off-plus-
  threshold surface here is deliberately the whole v1 scope.

## Firewall: ports 80 and 443

For real public traffic and ACME's HTTP-01 challenge to work, your server
needs ports **80** and **443** reachable from the internet. Port 80 is
also how Let's Encrypt validates domain ownership during ACME issuance;
if it's blocked, issuance fails even though everything else is configured
correctly. A cloud provider's firewall, a home router with no port
forwarding, or `ufw` left in its default-deny state will all block this
silently from the outside while looking fine from the server itself.

`install.sh` has an opt-in flag for this: set `LEVELRAIL_CONFIGURE_UFW=1`
before running it, and the script will allow SSH first, then open
`80/tcp` and `443/tcp`, and only then enable `ufw` if it wasn't already
active (if `ufw` was already active, it just adds the rules without
re-enabling anything). If you don't set that flag, `install.sh` doesn't
touch your firewall at all, and opening those ports is on you, whether
that's your cloud provider's security group, `ufw`, or `iptables`
directly.

## Walkthrough: your first domain, from install to HTTPS

This picks up right after [docs/getting-started.md](getting-started.md)'s
"Deploy your first app" section, assuming you already have the control
plane running and an app deployed.

1. **Point DNS at your server.** Create an A record for the domain you
   want to use (e.g. `my-app.example.com`) pointing at your server's
   public IP. Confirm it's resolving with `dig +short
   my-app.example.com` from a machine outside your own network before
   moving on.

2. **Add the domain to your app.** Either add it to `domains:` in
   `app.yaml` and redeploy:

   ```yaml
   services:
     web:
       domains:
         - my-app.example.com
       port: 3000
   ```

   ```
   APP_API_TOKEN=dev-root-token ./levelrail-cli apps deploy your-app --file app.yaml
   ```

   or open the app's **Domains** tab in the dashboard and add it there
   directly, no redeploy needed.

3. **Open your firewall**, if you haven't already: either re-run
   `install.sh` with `LEVELRAIL_CONFIGURE_UFW=1`, or manually confirm
   ports 80 and 443 are reachable from the internet.

4. **Check certificate status.** By default your app is now reachable
   over HTTPS with a self-signed certificate from Caddy's internal
   issuer, browsers will warn until you trust it or switch to ACME. If
   you want a real, browser-trusted certificate, go to **Settings >
   Domains**, enable ACME, and follow
   [docs/acme-verification-runbook.md](acme-verification-runbook.md) for
   the full verification steps (DNS check panel, rate-limit
   considerations, what to do if issuance fails).

5. **Confirm it works.** Visit `https://my-app.example.com` in a browser,
   or `curl -v https://my-app.example.com` from outside your network, and
   confirm you're hitting your app.

That's the whole flow. No proxy container to add anywhere in it.
