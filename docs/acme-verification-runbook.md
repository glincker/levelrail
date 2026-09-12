# Verifying real ACME issuance against a live domain

This is the one remaining check `docs/roadmap.md`'s "In progress" section
names for real public ACME: the Caddy ACME issuer, the settings toggle, and
form validation are all built, wired end to end, and covered by automated
tests (config-shape unit tests in `internal/ingress/acme_test.go`, and a
real, local-CA end-to-end issuance test in
`internal/ingress/acme_live_test.go`). What none of those can prove from a
sandbox is the thing ADR 005's own "Verified" section names explicitly:
real issuance against a real public ACME directory (Let's Encrypt or
otherwise), over a real domain, with real DNS and real port 80/443
reachability from the internet. That needs a human with a real server.

This document is that human's runbook. It assumes no prior context beyond
having Levelrail installed and running.

## Prerequisites

- A domain name you control, with DNS access (to create/edit an `A` or
  `AAAA` record).
- A server running this control plane (`levelrail.service`, via
  `install.sh`, or the Docker image), reachable from the public internet.
- That domain's DNS pointed at the server's public IP, **and already
  propagated** before you start (see the DNS failure mode below for how to
  check).
- Ports **80** and **443** open and reachable from the internet on that
  server. Let's Encrypt's HTTP-01 challenge (the only challenge type this
  platform's ACME issuer solves automatically today; DNS-01 via Cloudflare
  is separate, see `docs/roadmap.md`'s Cloudflare DNS entries) connects to
  your server on port 80 to validate domain ownership. A cloud firewall,
  a home router with no port forwarding, or `ufw` left in its default
  state will all silently break this from the outside while looking fine
  from the inside.
- Admin access to the Levelrail dashboard (or an API token with the
  `root` or ingress-settings ability).

## Steps

1. **Point DNS at the server**, if you haven't already, and wait for
   propagation. `dig +short your-domain.example.com` from a machine
   outside your own network should return the server's public IP. Don't
   proceed until it does: attempting ACME issuance before DNS has
   propagated is the single most common failure mode below, and every
   failed attempt burns a small amount of Let's Encrypt's per-domain rate
   limit.

2. **Open the dashboard**, go to **Domains** (top-level nav item, or
   `/domains` directly; the settings hub also links to it under
   "Platform ingress"). This is the `IngressSettingsCard` component
   (`web/src/components/IngressSettingsCard.tsx`), backed by
   `GET/PUT /api/v1/settings/ingress`.

3. **Set "Primary domain"** to your domain
   (e.g. `dashboard.example.com`). Once saved, a "Certificate status"
   badge and a DNS check panel appear beneath the field, driven by
   `GET /api/v1/settings/ingress/check`; use that panel now, before
   touching ACME, to confirm the platform itself sees your DNS record
   resolving correctly (it reports whether the record is inferred from
   your own connection or genuinely configured, and whether it matches
   what the server expects).

4. **Toggle "Issue real ACME certificates" on.** Two fields appear:
   - **Contact email**: required. This is the email the certificate
     authority uses to reach you about a problem with your certificates
     (e.g. an upcoming forced revocation). Use a real, monitored address.
   - **Directory URL (optional)**: leave blank for Let's Encrypt's real
     production directory. **For this first verification run, consider
     using Let's Encrypt's staging directory instead**
     (`https://acme-staging-v02.api.letsencrypt.org/directory`), to avoid
     burning production rate limits while you're still finding out
     whether DNS/ports are actually right. A staging-issued certificate is
     not trusted by real browsers (you'll see a certificate warning), but
     Caddy's logs and the dashboard's certificate status will otherwise
     behave identically to a production issuance, which is all you need
     to prove the mechanism works. Once staging succeeds, flip back to
     the blank (production) directory and save again to get a real,
     browser-trusted certificate.

5. **Save.** This does not itself trigger issuance; Caddy's own
   automatic-HTTPS reconciliation picks up the change the next time
   ingress reconciles for a route on that domain (in practice, within
   moments, since ingress reconciliation runs continuously and any
   domain configured for routing already qualifies for certificate
   management the moment ACME is enabled and a valid email is on file).

6. **Watch it happen.** See "What to check" below for exactly where to
   look and what success looks like.

## What to check to confirm success

- **Dashboard**: the "Certificate status" badge next to Primary domain on
  the Domains page should read **Healthy** (green). This comes from
  `GET /api/v1/certificates`, backed by `internal/api/certificates.go`.
  Click through (or check the full certificates list, if your dashboard
  version has one) to see the issuer field: for a real ACME certificate
  this will name your chosen CA (e.g. `Let's Encrypt` or, on the R-series
  intermediate chain, something like `R11`/`R12`), never Levelrail's own
  offline internal issuer.
- **Expiry**: `not_after` on that same certificate record should be
  roughly 90 days out for Let's Encrypt (their standard lifetime). If it
  reads a lifetime closer to a handful of hours, you're still looking at
  the platform's internal (self-signed) issuer, not a real ACME cert; the
  toggle likely didn't save, or you're looking at a stale cached page.
- **Auto-renewal state**: nothing to configure here. Caddy's own
  automatic-HTTPS management renews a managed certificate well before
  expiry (industry-standard practice is renewal starting around 30 days
  before expiry for a 90-day cert) with no separate cron job, timer, or
  operator action. The way to confirm renewal is working, short of
  waiting ~60 days, is to watch the logs (next bullet) for a
  `"renewing certificate"` or `"certificate obtained successfully"` line
  reappearing on its own before the current certificate's `not_after`.
- **Logs**: `journalctl -u levelrail -n 200 --no-pager` (or your Docker
  image's log output, if running that way) on the server. Caddy's own
  structured logs go through the same process; look for lines with
  `"logger":"tls.obtain"` and `"logger":"http.acme_client"`. A clean run
  looks like this sequence (abbreviated, matching what
  `internal/ingress/acme_live_test.go` proves against a local test CA,
  just with a real Let's Encrypt account and identifier instead of a
  sandboxed one):

  ```
  "acquiring lock" -> "obtaining certificate" -> "registering account" (first
  time only) -> "trying to solve challenge","challenge_type":"http-01" ->
  "authorization finalized","authz_status":"valid" -> "finalizing order" ->
  "successfully downloaded available certificate chains" ->
  "certificate obtained successfully"
  ```

  Any error logged between "trying to solve challenge" and "authorization
  finalized" is almost always the DNS or port-80 failure modes below, not
  a bug in the issuer code itself.

## Common failure modes and what they look like

- **DNS not propagated / wrong record**: the ACME log shows the challenge
  attempt failing with something like `"error":"... connection ...
  timed out"` or `"... no such host"` right after "trying to solve
  challenge". Fix: re-check `dig +short your-domain.example.com` from
  outside your own network, wait for propagation (can take minutes to a
  few hours depending on your DNS provider's TTL), retry.
- **Port 80 blocked** (cloud security group, `ufw`, a router with no port
  forward, or a reverse proxy in front of Levelrail eating port 80 for
  itself): the log shows the challenge request timing out or being
  refused, distinguishable from the DNS case because `dig` correctly
  resolves the domain but the ACME CA still can't reach it. Fix: open port
  80 inbound (443 too, for actually serving the certificate afterward).
  `levelrail-cli doctor` / the dashboard's "System status" page includes a
  firewall/`ufw` check that can catch this ahead of time.
- **Rate limited**: Let's Encrypt's production directory enforces real
  per-domain and per-account limits (as of this writing, roughly 5 failed
  validations per account/hostname/hour, and 50 certificates per
  registered domain per week; check Let's Encrypt's own current published
  limits, they change occasionally). The log shows an explicit `rate
  limited` or `too many` error from the CA. This is exactly why step 4
  above recommends the staging directory for your first attempt: staging
  has much more permissive limits and is meant for repeated verification
  runs like this one.
- **Missing or invalid contact email**: caught by the dashboard's own form
  validation before it ever reaches the API (`ingressSettingsSchema` in
  `IngressSettingsCard.tsx`); you shouldn't be able to save the toggle on
  without one. If you somehow got past that (e.g. via a raw API call),
  the backend's own `validateIngressSettingsRequest` rejects it the same
  way.
- **A pre-existing certificate on that domain from Levelrail's internal
  issuer**: if the domain already has a certificate (self-signed,
  internal) when you enable ACME, expect a brief window where the old
  certificate is still what's served until Caddy's automatic-HTTPS
  reconciliation replaces it. If it doesn't replace itself within a
  reasonable window (a few minutes), check the logs for the issuance
  sequence above; if nothing is being attempted at all, confirm the
  domain is actually routed to something (ACME issuance is scoped to
  domains that qualify for a certificate because they're actually in use
  by a route, not merely typed into a form).

## Once this succeeds: closing the gap

This runbook existing and being followed once, successfully, against a
real domain is what finally closes the specific gap this project has
carried since ADR 005's Phase 0 spike. When you've confirmed a real,
browser-trusted (production-directory, not staging) certificate is
healthy and auto-renewal is credible (logs show the expected sequence,
`not_after` is a normal ~90-day lifetime), update **`docs/roadmap.md`**:
move the **"Real public ACME"** bullet out of the "In progress" section
and into "Done", with a short note of what was verified and when (domain
used doesn't need to be named if it's a private/internal one; the fact
that it was a real public domain with real DNS and real port reachability
is the material fact). That bullet is currently the only remaining line
item blocking this from being a fully closed Phase 1 item.
