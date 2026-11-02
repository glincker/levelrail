# ADR 066: Built-in dynamic DNS updater

Status: Proposed

Date: 2026-11-01

## Context

Home-lab operators run the control plane on a line whose public IPv4 address
changes. They want a stable hostname (a DuckDNS name, a record in a Cloudflare
zone, or a name at any provider with an HTTP update URL) to follow that address
so apps and certificates keep working.

What exists today, checked in code:

- `internal/ingress/publicip.go` (`ResolvePublicHost`) decides the public
  address once. `APP_PUBLIC_HOST` wins, `APP_PUBLIC_IP_DETECT=off` disables
  probing, otherwise it asks the URLs in `APP_PUBLIC_IP_PROBE_URLS` (three
  what-is-my-IP services by default) with a timeout from
  `APP_PUBLIC_IP_PROBE_TIMEOUT`, IPv4 first. The only boot call site is
  `cmd/levelrail/boot_env.go`, so the value is fixed until restart.
  `DetectPublicIPs` is also called on demand by `internal/api/dns_provider.go`.
- Cloudflare zone and record management already exists
  (`internal/dnszones/cloudflare_records.go`, `internal/dnsrecords`), with a
  stored API token, record planning (`PlanRecord`, `TargetFor`) and a conflict
  rule. No updater runs it on a timer.
- No dynamic DNS client exists for any provider.
- `kit/netguard` provides an SSRF-guarded client (`NewClient`,
  `NewClientAllowing`, `ValidateURL`) that refuses private, loopback and
  link-local destinations at dial time.
- `internal/secretmask` holds a process-wide registry that masks registered
  values in every slog record (`secretmask.Handler` wraps the root logger in
  `cmd/levelrail/main.go`).
- Certificates are issued by embedded Caddy (HTTP-01 for regular hosts,
  DNS-01 for wildcard hosts only). `internal/certflow/watchdog.go` retries
  stuck renewals with timings from `APP_CERT_WATCHDOG_INTERVAL` and
  `APP_CERT_WATCHDOG_BACKOFF`.

Today an operator works around this with a separate updater container and a
restart on every change, and the dashboard shows a stale public address until
then.

## Decision

Build a small, opt-in updater inside the control plane. It is off unless at
least one target is configured.

1. **Detection loop.** A background loop re-runs the existing probe logic on
   an interval read from `APP_DDNS_INTERVAL`. It reuses the probe URL list and
   timeout from `ResolvePublicHost` so there is one probe configuration. To
   avoid flapping on one bad answer, a change is accepted only when
   `APP_DDNS_PROBE_QUORUM` probes agree (a quorum of one keeps today's
   first-answer behavior). The loop is disabled when `APP_PUBLIC_IP_DETECT` is
   off, since the operator has said the address is fixed.
2. **Live public address.** The detected address replaces the boot-time value
   behind an accessor, so domain checks, the settings view and record
   auto-create see the current address without a restart. When
   `APP_PUBLIC_HOST` is set to a hostname, that hostname stays the public host
   and the updater only keeps its record pointed at the new IP.
3. **Providers, first version.**
   - DuckDNS: one GET to its documented update endpoint with domain, token and
     IP. Token stored as a secret.
   - Cloudflare: reuses the stored zone token and the existing record code to
     upsert an A record (and AAAA when enabled). No new module.
   - Generic HTTP update URL: an operator-supplied URL with an `{ip}`
     placeholder (and `{ipv6}` when enabled), GET or POST, covering providers
     that publish a single update URL. The whole URL is a secret because it
     usually carries a token.
   Other providers with their own APIs are out of scope until a user asks; the
   generic URL covers most of them.
4. **When it writes.** Only on a confirmed change, plus a periodic re-assert at
   `APP_DDNS_REASSERT_INTERVAL` to repair records edited elsewhere. Writes are
   spaced by at least `APP_DDNS_MIN_UPDATE_GAP` to respect provider limits.
   IPv4 only by default; AAAA updates need `APP_DDNS_IPV6=on`.
5. **Egress.** Probe and update requests use the netguard client: https only
   (http allowed for the generic URL only with
   `APP_DDNS_ALLOW_INSECURE_HTTP=on`), no redirects to another host, private
   destinations refused unless `APP_DDNS_ALLOW_PRIVATE` is set, timeout from
   `APP_DDNS_TIMEOUT`. A detected address that is private, CGNAT range or
   otherwise non-public is never written; the loop records a warning instead,
   because writing it would break every client.
6. **Secrets.** DuckDNS tokens and generic URLs go through `internal/secrets`
   envelope encryption, are added to the `secretmask` registry on load, and are
   never returned by the API, CLI or MCP (host plus a configured flag only).
   Errors from net/http are scrubbed before logging so a URL with a token is
   never echoed. Audit entries record that a target changed, never the value.
7. **ACME interaction.** A certificate is bound to the hostname, not the IP,
   so an IP change never triggers reissue. Renewal over HTTP-01 fails while
   the record is stale, so the updater runs before the renewal watchdog
   matters: on a confirmed change it updates the record first, then asks the
   watchdog to retry any domain that failed in the last window. `<dashed-ip>.sslip.io`
   hosts change name when the IP changes; the updater does not rewrite them.
   Instead the settings view and domain doctor tell the operator to move to a
   dynamic DNS hostname. The updater does not change challenge type.
8. **Failure alerts.** After `APP_DDNS_FAIL_ALERT_AFTER` consecutive failed
   writes (or a detected address that disagrees with the record for that
   long), the loop raises an internal alert through the existing alerting
   path, which reaches configured channels. Recovery resolves it. Status
   (last check, last change, last error host and class) shows in
   `settings ingress` on CLI, API, MCP and dashboard.
9. **Scope.** Control plane only. Worker nodes keep their own addressing;
   per-node records are a later decision.

Defaults for every env value above are listed once in
`docs/environment-variables.md` and in the config reader, not scattered through
logic.

## Alternatives rejected

- **Docs only (run a separate updater container and restart).** Leaves the
  dashboard wrong until restart and gives no alert. Kept as the documented
  path until slices 1 to 3 ship.
- **DNS-01 provider modules for DuckDNS and other hosts.** Needs `go.mod`
  additions (an owner decision) and solves certificates, not addressing.
  Tracked separately.
- **Polling the router (UPnP or NAT-PMP).** Not reachable from a container on
  many setups, and a second detection path to secure.

## Security

- Two new outbound paths (probe and update), both through netguard with no
  new bypass.
- Tokens and generic URLs are secrets end to end: encrypted at rest, masked in
  logs, absent from exports and API reads.
- A compromised probe service could report a wrong address. The quorum setting
  limits this, and the non-public address check stops the worst case. The
  operator can point `APP_PUBLIC_IP_PROBE_URLS` at services they trust.

## Failure modes

- All probes down: keep the last known address, alert after the threshold.
- Probes disagree: no write, warning in status.
- Provider rejects the token: write marked failed, alert after the threshold,
  no retry storm (minimum gap applies).
- Address flaps between two values: the minimum gap bounds writes.
- Behind CGNAT: the detected address is the carrier's, inbound traffic still
  fails. The doctor already explains CGNAT; the updater does not hide it.

## What cannot be verified locally

- Real DuckDNS and Cloudflare writes and how fast resolvers see them.
- Behaviour on a real line whose address changes (needs a home host or a cloud
  VM with a released and re-attached address).
- A renewal that happens shortly after a change.

## Testing plan

- Unit: quorum logic, change detection, minimum gap and re-assert timing with
  an injected clock; non-public address refused; placeholder substitution and
  URL validation; errors never contain the token (captured log buffer).
- Provider fakes: DuckDNS and generic URL against `httptest` servers;
  Cloudflare through the existing record fakes in `internal/dnszones`.
- Real host, once: change the VM's public address, observe the record update,
  then force a renewal and record the result in the verification runbook.

## Slices

| # | Slice | Files it owns | Effort | Model | Real host | Secret-sensitive |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Detection loop, quorum, live public address accessor, env reader | `internal/ingress/publicip.go`, new `internal/ingress/publicip_watch.go`, `cmd/levelrail/boot_env.go` | 1 day | Sonnet | No | No |
| 2 | Updater core: DuckDNS, generic URL, Cloudflare via existing record code, netguard egress, minimum gap | new `internal/ddns/` | 1 day | Sonnet | No | Yes |
| 3 | Secret storage, API fields with masked reads, audit, CLI `settings ingress` flags, MCP field | `internal/api/` ddns handler file, `cmd/levelrail-cli/settings_ingress.go`, `internal/mcptools` ingress tool, store migration | 1 day | Opus review of secret paths, Sonnet builds | No | Yes |
| 4 | Alerts and status, watchdog nudge after a change | `internal/ddns/`, `internal/certflow/watchdog.go` (one hook) | 0.5 day | Sonnet | No | No |
| 5 | Dashboard card and locale keys | `web/src` ingress settings card, `web/src/locales/en/*.json` | 0.5 day | Sonnet | No | No |
| 6 | Docs: env rows, dynamic DNS page, domain doctor hint for sslip.io users | `docs/environment-variables.md`, new `docs/dynamic-dns.md`, `docs/domains-and-ingress.md` | 0.5 day | Haiku draft, Sonnet review | No | No |
| 7 | Real-host run and runbook entry | `docs/acme-verification-runbook.md` | 0.5 day | Sonnet | Yes | No |

Slices 1 and 2 can run in parallel (no shared files). 3 needs 2. 4 needs 1 and 2.

New env: `APP_DDNS_INTERVAL`, `APP_DDNS_PROBE_QUORUM`, `APP_DDNS_REASSERT_INTERVAL`,
`APP_DDNS_MIN_UPDATE_GAP`, `APP_DDNS_TIMEOUT`, `APP_DDNS_FAIL_ALERT_AFTER`,
`APP_DDNS_IPV6`, `APP_DDNS_ALLOW_PRIVATE`, `APP_DDNS_ALLOW_INSECURE_HTTP`.

## Consequences

- Home-lab users with port forwarding get a working path without a sidecar.
- The public address becomes mutable at runtime; every reader must go through
  the accessor. Slice 1 owns that change.
- One more background loop and a small number of outbound requests per
  interval.
- No `go.mod` change.

## Open questions

- Should a confirmed change also update app records the platform auto-created
  in a connected Cloudflare zone, or only the control plane hostname? Proposed:
  only the control plane hostname in the first version.
- Should per-node records for multi-node home setups come next, or wait for
  demand?
