# Verifying real ACME issuance against a live domain

The Caddy ACME issuer, settings toggle, and form validation are all built, wired end to end, and covered by automated tests. Specifically: config-shape unit tests in `internal/ingress/acme_test.go`, and a real, local-CA end-to-end test in `internal/ingress/acme_live_test.go`.

What those tests cannot prove from a sandbox is real issuance against a real public ACME directory (Let's Encrypt or otherwise), over a real domain, with real DNS and real port 80/443 reachability from the internet. ADR 005's "Verified" section names this explicitly.

This runbook is for that verification step. It assumes you have Levelrail installed and running.

## Prerequisites

- A domain name you control, with DNS access to create or edit an `A` or `AAAA` record.
- A server running the control plane (via `install.sh`, `levelrail.service`, or Docker), reachable from the public internet.
- DNS already pointed at the server's public IP and propagated (see the DNS failure mode section below for how to verify).
- Ports 80 and 443 open and reachable from the internet on that server. Let's Encrypt's HTTP-01 challenge (the only automatically-solved type today; DNS-01 via Cloudflare is separate, see `docs/roadmap.md`) validates domain ownership on port 80. Cloud firewalls, home routers without port forwarding, or `ufw` in its default state will silently break this from the outside while appearing fine from inside.
- Admin access to the Levelrail dashboard (or an API token with `root` or `ingress-settings` ability).

## Steps

1. **Point DNS at the server**, if you haven't already, and wait for
   propagation. `dig +short your-domain.example.com` from a machine
   outside your own network should return the server's public IP. Don't
   proceed until it does: attempting ACME issuance before DNS has
   propagated is the single most common failure mode below, and every
   failed attempt burns a small amount of Let's Encrypt's per-domain rate
   limit.

2. **Open the dashboard** and go to **Domains** (top-level nav item, or `/domains` directly). You can also find it in the settings hub under "Platform ingress". This is backed by the `IngressSettingsCard` component (`web/src/components/IngressSettingsCard.tsx`) and the `GET/PUT /api/v1/settings/ingress` endpoints.

3. **Set "Primary domain"** to your domain (e.g. `dashboard.example.com`).

   Once saved, a "Certificate status" badge and a DNS check panel appear below the field. Use this panel now, before enabling ACME, to confirm the platform can resolve your DNS record correctly. The check shows whether the record is inferred from your own connection or genuinely configured, and whether it matches the server's expectations.

4. **Toggle "Issue real ACME certificates" on.** Two fields appear:

   - **Contact email**: Required. This is the email address the certificate authority uses to reach you about certificate issues (e.g. forced revocation). Use a real, monitored address.
   - **Directory URL**: Optional. Leave blank for Let's Encrypt's production directory.

   ::: tip
   For your first verification run, use Let's Encrypt's staging directory instead (`https://acme-staging-v02.api.letsencrypt.org/directory`). This avoids burning production rate limits while you verify DNS and port reachability. Staging-issued certificates trigger browser warnings but work identically to production issuance for testing the mechanism. Once staging succeeds, flip back to blank and save again for a real, browser-trusted certificate.
   :::

5. **Save.** Saving does not immediately trigger issuance. Caddy's automatic-HTTPS reconciliation picks up the change the next time ingress reconciles for a route on that domain. In practice this happens within moments, since ingress reconciliation runs continuously and any routed domain qualifies for certificate management once ACME is enabled and a valid email is on file.

6. **Watch it happen.** See "What to check" below for exactly where to
   look and what success looks like.

## What to check to confirm success

**Dashboard**

The "Certificate status" badge next to Primary domain on the Domains page should read **Healthy** (green). This comes from `GET /api/v1/certificates`, backed by `internal/api/certificates.go`.

Click through to see the issuer field. For a real ACME certificate, this names your chosen CA (e.g. `Let's Encrypt` or an R-series intermediate like `R11` or `R12`), never Levelrail's internal issuer.

**Expiry**

The `not_after` field should be roughly 90 days out for Let's Encrypt (their standard lifetime). If it reads a few hours, you're still looking at the platform's internal (self-signed) issuer. The toggle likely didn't save, or you're viewing a stale cached page.

**Auto-renewal state**

Nothing to configure. Caddy's automatic-HTTPS management renews well before expiry, industry-standard practice being around 30 days before for a 90-day cert. No separate cron job, timer, or operator action needed.

To confirm renewal is working (short of waiting 60 days), watch the logs for a `"renewing certificate"` or `"certificate obtained successfully"` line appearing before the current certificate's `not_after` date.

**Logs**

Run: `journalctl -u levelrail -n 200 --no-pager` (or your Docker image's log output). Caddy's structured logs go through the same process.

Look for lines with `"logger":"tls.obtain"` and `"logger":"http.acme_client"`.

A clean run looks like this sequence (from `internal/ingress/acme_live_test.go` against a local test CA, just with real Let's Encrypt account and identifier):

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

**DNS not propagated or wrong record**

ACME logs show challenge failure with `"error":"... connection ... timed out"` or `"... no such host"` right after "trying to solve challenge".

Fix: Verify with `dig +short your-domain.example.com` from outside your network. Wait for propagation (can take minutes to hours depending on DNS provider TTL), then retry.

**Port 80 blocked**

The log shows the challenge request timing out or refused. Unlike DNS issues, `dig` correctly resolves the domain but the ACME CA cannot reach it. This happens with cloud security groups, `ufw`, routers without port forwarding, or reverse proxies eating port 80.

Fix: Open port 80 and 443 inbound. You can also run `levelrail-cli doctor` or check the dashboard's "System status" page, which includes firewall and `ufw` checks.

**Rate limited**

Let's Encrypt's production directory enforces per-domain and per-account limits. As of now, roughly 5 failed validations per account/hostname/hour, and 50 certificates per domain per week (check Let's Encrypt's current limits, they change). The log shows an explicit `rate limited` or `too many` error from the CA.

This is why step 4 recommends staging for your first run. Staging has much more permissive limits and is designed for repeated verification.

**Missing or invalid contact email**

The dashboard's form validation catches this before it reaches the API (`ingressSettingsSchema` in `IngressSettingsCard.tsx`). You shouldn't be able to save without one. If you bypass it via raw API call, the backend's `validateIngressSettingsRequest` rejects it the same way.

**Pre-existing certificate from internal issuer**

If the domain already has a self-signed certificate when you enable ACME, expect a brief window where the old cert stays served until Caddy's automatic-HTTPS reconciliation replaces it. If replacement doesn't happen within a few minutes, check logs for the issuance sequence. If nothing is being attempted, confirm the domain is actually routed to something. ACME issuance only applies to domains actively in use by a route, not those merely typed into a form.

## Once this succeeds: closing the gap

This runbook successfully run against a real domain closes the gap this project has carried since ADR 005's Phase 0 spike.

When you've confirmed a real, browser-trusted (production, not staging) certificate is healthy and auto-renewal is credible (logs show the expected sequence, `not_after` is roughly 90 days), update `docs/roadmap.md`.

Move the "Real public ACME" bullet from the "In progress" section to "Done", with a short note of what was verified and when. The domain itself doesn't need to be named if it's private or internal. What matters is that it was a real public domain with real DNS and real port reachability.

This bullet is the only remaining item blocking Phase 1 completion.
