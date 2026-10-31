# ADR 058: An optional in-house mail server, wired from an existing engine

Partly superseded by [ADR 064](064-email-for-apps.md): new mail work follows ADR 064.

Status: Proposed

Date: 2026-10-29

## Context

The question: can a Levelrail operator run their own mail for many domains
and many mailboxes from the control plane, on a node they pick, with DNS
wired for them, and can Levelrail itself send its no-reply mail (alerts,
invites, password resets, status page subscribers from ADR 054) through
that server instead of a paid sending provider?

What exists today, verified by reading `origin/release/v0.8.12`:

1. **Plugins cannot host a mail server.** `docs/plugins.md` and
   `internal/plugin/manifest.go` define three kinds only
   (`notification_channel`, `panel_provider`, `dns_provider`; `Validate`
   rejects anything else). A plugin runs with a read-only root filesystem,
   every capability dropped, a process, memory and CPU limit, an
   unprivileged user, and its only mount is a private socket directory.
   With no declared outbound hosts it has no network at all, and declared
   hosts are shown at approval but not enforced per host. Plugins on
   remote nodes are listed as not supported. A mail server needs the
   opposite of every one of these: public ports 25, 465, 587 and 993, a
   persistent data volume, TLS material, outbound port 25, and placement
   on a node the operator chooses.
2. **The app catalog can run raw TCP services, with friction.**
   `internal/catalog` templates are compose files with env prompts,
   generated secrets (`EnvPrompt.Generate`, `Secret`), domain prompts,
   a healthcheck and `RecommendedMemoryBytes`. The RustDesk template in
   `templates_catalog_batch_rustdesk.go` publishes raw TCP and UDP ports;
   published ports bind to `127.0.0.1` by default (`internal/bindaddr`,
   empty value resolves to private) and the operator sets a public bind
   per port in the app's Network settings. A `stalwart` template already
   exists in `templates_selfhosted4.go`, pinned to `v0.16.23`, exposing
   only the HTTP port 8080, with a comment that the mail ports are not
   exposed. Its volume paths (`/opt/stalwart/...`) predate the v1 layout.
   It is a plain app, not a managed mail service.
3. **Placement is per service.** Services carry a `NodeID`;
   `internal/placement` has `Pinned`, `Spread`, label `Selector` and
   resource `Needs` checks. Whether the catalog install request accepts a
   target node today was not confirmed by reading; slice 1 checks it.
4. **DNS zones and a mail DNS engine.** `internal/dnszones` plans and
   applies record sets for Cloudflare, Route53 and Google Cloud DNS. The
   branch `feat/mail-dns` (three commits, in review) adds
   `internal/mailrecords` (`build.go`, `plan.go`, `check.go`,
   `presets.go`) and `POST /api/v1/mail/dns/plan` (read),
   `POST /api/v1/mail/dns/apply` (root), `GET /api/v1/mail/dns/check`
   (read) behind `APP_EXPERIMENTAL=mail`. Plans are add-only by default;
   `replace_existing` must name a record kind (MX, SPF, DKIM, DMARC,
   MTASTS, TLSRPT) to overwrite it, a foreign MX produces a refusal with a
   reason, a plan hash pins what was reviewed, and audit subjects carry
   names and types, never values. This ADR builds on that engine and does
   not redesign it. It has no PTR check today.
5. **Email sending.** `internal/email` has backends `smtp`, `ses`,
   `ses_smtp`, `resend`, `postmark`, `sendgrid` and `mailgun`, a provider
   registry (`provider.go`), a primary plus backup failover with a circuit
   breaker and pre-send error classification (`failover.go`), delivery
   recording (`dynamic.go`, `store/email_deliveries.go`) and SPF, DKIM and
   DMARC checks in `domaincheck.go`. Providers live in
   `store/email_providers.go` with roles `primary` and `backup`, their
   credentials in `internal/secrets` under `EmailProviderSecretsKey`.
   `internal/alerting/email_delivery_failing.go` already alerts on a
   failing outbound log. `internal/domaindoctor` diagnoses web domains.
6. **Backups.** Volume backups (`internal/backup`, `quiesce.go`) run a
   per volume `PreHook` and `PostHook` inside the app container and can
   pause it; policies are in `store/backup_sealed.go`. Retention is GFS.
7. **Secrets.** `internal/secrets.Manager` seals values per service and
   env key (`SetValue`, `Resolve`, `SetLocked`).

What we measured locally about the two candidate engines (Docker on arm64,
all ports on loopback, no real mail sent):

| | mox v0.0.17 | Stalwart v1.0.1 |
| --- | --- | --- |
| License | MIT | AGPL-3.0, plus an enterprise license for some features |
| Language | Go | Rust |
| Image, compressed arm64 | about 22.6 MB | about 116.5 MB |
| Idle RAM | 13.5 MiB at start, 23.6 MiB after 2 min (dev mode, a lower bound) | about 149 MiB after setup |
| Idle CPU | 0% | 0% |
| Admin surface | sherpa JSON-RPC at `/admin/api/`, about 97 functions, plus an in-container CLI over a control socket | JMAP objects at `POST /jmap`, plus a separate CLI |
| Backup | `mox backup <dir>` online consistent copy, `mox verifydata` | backend specific, live RocksDB copy is not a backup |
| Webmail | built in | not bundled as far as we saw |
| Release age | pre 1.0, last release August 2026 | 1.0 released days ago with a new config model |

mox answered an unauthenticated admin call with `missing required csrf
header`, so a script must send the header the web UI sends. The admin
function list includes `DomainAdd`, `DomainRemove`, `DomainRecords`,
`AccountAdd`, `AccountRemove`, `AddressAdd`, `AliasAdd`, `SetPassword`,
`AccountSettingsSave`, `DomainDKIMAdd`, `QueueSize`, `QueueList`,
`QueueHoldSet`, `QueueDrop`, `DMARCReports`, `TLSReports`, `Transports`
and `DomainRoutesSave` (read from the upstream API description, not
called end to end).

## Decision (recommended)

**Wire an existing open source mail engine. Do not write one.** Default
engine: **mox**. Stalwart becomes an optional, later "full groupware"
template. The feature is off by default, gated by `APP_EXPERIMENTAL=mail`
(the flag the DNS engine already uses), and installed like an app.

Why mox by default: MIT license, a single small Go binary, idle memory in
the tens of MiB so it fits on a small node next to apps, built in webmail,
DMARC and TLS report ingest, an online backup command, and an admin API
that covers domains, accounts, aliases, DKIM, DNS records, queue and
transports. Its cost is pre 1.0 churn and no downgrade path.

Why Stalwart later, not first: about six times the idle memory, a 1.0
release only days old that replaced its configuration model, and AGPL
obligations to document. It wins when an operator wants JMAP, CalDAV,
CardDAV, Sieve and POP3.

**License boundary.** We run unmodified upstream images as separate
containers and talk to them only over their network API. No engine code is
linked into or copied into the Levelrail binary. Levelrail stays
Apache-2.0. For the AGPL engine we ship the license text with the template
and link the upstream source of the exact pinned release. We never enable
enterprise-only features. Whether any specific integration pattern forms
a combined work is a question for counsel, listed below.

### 1. Architecture: two layers

**Layer A, the engine as a managed stack.** A catalog template `mail-mox`
(compose, pinned by digest) runs on a node the operator picks at install,
with operator-set CPU, memory and disk limits. It publishes 25, 465, 587
and 993 with an explicit public bind, and 443 or 80 only if the engine
serves its own autoconfig, MTA-STS policy and webmail; otherwise the
ingress serves the webmail hostname. The admin listener and the metrics
listener bind to the private network only. A named data volume holds
config and mail. The stack is labelled `levelrail.mail=engine` so the
control plane finds it and refuses a second engine per node.

**Layer B, `internal/mail` in the control plane.**

```go
type Engine interface {
    Domains(ctx context.Context) ([]Domain, error)
    AddDomain(ctx context.Context, d DomainSpec) error
    RemoveDomain(ctx context.Context, name string) error
    DNSRecords(ctx context.Context, domain string) ([]mailrecords.Record, error)
    Accounts(ctx context.Context, domain string) ([]Account, error)
    AddAccount(ctx context.Context, a AccountSpec) error
    RemoveAccount(ctx context.Context, address string) error
    SetPassword(ctx context.Context, address, password string) error
    SetQuota(ctx context.Context, address string, q Quota) error
    AddAlias(ctx context.Context, alias string, targets []string) error
    QueueStats(ctx context.Context) (QueueStats, error)
    SetSmarthost(ctx context.Context, s *Smarthost) error
    Health(ctx context.Context) error
}
```

The mox adapter (`internal/mail/mox`) calls the sherpa API over the
private address with HTTP Basic auth and the CSRF header. The admin
password is generated at install, stored through `internal/secrets` under
the engine's service name, locked, never returned by any API and never
logged. The adapter maps engine DNS output into `mailrecords.Spec` so the
DNS engine stays the single writer of mail records.

### 2. The future `service` plugin kind

The managed template is v1. The plugin contract needs these additions
before a mail engine can ship as a signed plugin:

- **Declared public ports** (protocol, number, purpose), each approved by
  the operator and shown at install.
- **Volumes with sizes** and a retention choice on uninstall (keep, back
  up then delete, delete).
- **Node placement**: a target node or selector, so plugins run on remote
  nodes (today they do not).
- **Resource limits** set by the operator, not only the manifest.
- **Outbound access** declared per port and host, with port 25 called out,
  and real per-host enforcement (today it is not enforced).
- **A mediated DNS ability**: the plugin returns desired records, the
  control plane plans and applies them through `mailrecords` and
  `dnszones`. The plugin never holds DNS credentials.
- **Health and metrics**: a `ServiceService.Health` plus queue depth and
  delivery counters through the existing RPC.
- **Image digest pinning** and a writable root or declared writable paths.

Migration: the v1 template and the v2 plugin share `internal/mail.Engine`.
An install records `engine_source=template`; moving to the plugin re-uses
the same volume and admin secret, swaps the supervisor, and is a no-op for
domains and accounts.

### 3. TLS

Recommended: **Levelrail's ACME certificates mounted read-only** into the
engine (mox supports static key and cert files in its config). One
issuer, one renewal alert path (`internal/alerting/cert_*.go`), and the
engine does not need port 80 or 443. Fallback: the engine's own ACME,
selected per install, for nodes where Levelrail's ingress does not hold
the mail hostname. **Unverified:** whether mox picks up a renewed
certificate file without a restart. Until verified, renewal triggers a
controlled engine restart in a quiet window, and a check alert fires if
the served certificate expires before the file on disk.

### 4. DNS

The DNS engine on `feat/mail-dns` applies MX, SPF, DKIM, DMARC, MTA-STS,
TLS-RPT and autoconfig records. Rules carried forward unchanged: add-only
by default, a foreign MX is never replaced without `replace_existing`
naming MX, plans are hash pinned, audit rows carry names and types only.
New for the engine: DKIM public keys come from `Engine.DNSRecords`, and a
domain is marked "mail ready" only after `check` passes.

**PTR and port 25.** Levelrail cannot set reverse DNS; it is a per
provider control panel or support ticket. We add a PTR check (forward
confirmed: the node IP's PTR resolves to the mail hostname, which resolves
back to the IP) and a guide per known host provider. We also test
outbound port 25 from the chosen node with a timed connect to a known MX.
Most clouds block it by default. When it is blocked, the install offers
**relay (smarthost) mode**: outbound goes through an authenticated SMTP
relay (any of the existing providers with SMTP credentials) via mox
transports and routes. Inbound still needs port 25 open inbound.

### 5. Mailboxes

Many domains and many mailboxes in one engine. Per mailbox: address,
display name, quota, aliases, catch-all per domain (off by default),
per-domain DKIM generated by the engine. Passwords are generated by
Levelrail (length from `APP_MAIL_PASSWORD_LENGTH`, default 24), sent to
the engine, shown once in the dashboard or CLI, and not stored by
Levelrail in any form; reset generates a new one. The dashboard shows
IMAP and SMTP client settings, a webmail link, and the autoconfig and
autodiscover hostnames the DNS engine published.

### 6. The `builtin_mail` email provider

A new backend `builtin_mail` registers in `internal/email/provider.go`
and sends over SMTP submission (587, STARTTLS required) to the engine on
the private network. Enabling it creates `no-reply@<system domain>` with a
generated password in the secrets store. It can be primary or backup, so
the existing failover works both ways: a paid provider backs up the
engine, or the engine backs up a paid provider. Rate limits: a per minute
cap for system mail (`APP_MAIL_SYSTEM_RATE_PER_MIN`, default 60) enforced
in Levelrail before submission, plus the engine's own per account limit.

Apps opt in per app. The control plane creates a per-app account (for
example `app-<name>@<domain>`) with a send quota
(`APP_MAIL_APP_SEND_PER_HOUR`, default 100) and injects `SMTP_HOST`,
`SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD` and `SMTP_FROM` as sealed env
vars. Removing the app removes the account.

### 7. Deliverability and abuse

What we promise: correct DNS, valid TLS, authenticated submission, DKIM
signing, DMARC and TLS reports visible, honest checks. What we do not
promise: inbox placement. A new IP has no reputation; large mailbox
providers may defer or spam folder mail for weeks. The dashboard says so,
recommends relay mode for system mail during warm-up, checks the node IP
against public DNS blocklists daily, and shows DMARC aggregate reports the
engine ingests.

Abuse and security: no open relay (verified by a test that submits
unauthenticated mail for a foreign domain and expects rejection);
submission only with auth on 465 and 587; per account rate limits; an
**outbound kill switch** that holds the whole queue
(`QueueHoldSet`) when the outbound rate passes
`APP_MAIL_OUTBOUND_KILL_PER_HOUR` (default 1000) or a blocklist hit
appears, with an alert and a manual release; audit rows for domain,
account, alias, password reset, quota, relay and kill switch changes;
the engine image pinned by digest and matched against the advisory feed
from ADR 056.

### 8. Resource footprint and placement

mox idles in tens of MiB, but mail volume is disk bound and the queue can
spike. Default limits: memory `APP_MAIL_DEFAULT_MEMORY_MB` (256), CPU
`APP_MAIL_DEFAULT_CPUS` (0.5), volume size warning at
`APP_MAIL_DISK_WARN_PCT` (80). Do not place the engine on the control
plane node when: the control plane serves more than a handful of apps, the
node's IP must stay clean for other traffic, or the disk is shared with
the control plane database. The install warns and requires a confirm flag
for the control plane node.

### 9. Backups

A volume backup policy is created at install with
`PreHook: mox backup /mox/data/backup-staging` and the staging path as the
backed up path, then `mox verifydata` in a scheduled drill. Restore stops
the engine, restores the volume, starts it. **mox has no downgrade:** it
upgrades its database schema on start. Every engine upgrade takes a fresh
backup first and blocks if that backup fails; rollback means restore, not
an older image.

### 10. Surface

- **API** under `/api/v1/mail/`: `engine` (GET status, POST install),
  `domains`, `domains/{d}/accounts`, `accounts/{a}/password` (POST
  returns once), `aliases`, `queue` (GET stats, POST hold or release),
  `relay`, and the existing `dns/plan|apply|check`.
- **CLI**: `levelrail mail engine|domain|account|alias|queue|dns`.
- **MCP**: two read tools only, `mail_status` (engine health, queue,
  domain readiness) and `mail_dns_check`. No write tools in v1; the token
  budget is tight.
- **Dashboard**: Mail under Settings, with Engine, Domains, Mailboxes,
  Deliverability and Relay tabs, compact cards, prefilled forms.
- **IAM**: reads need `read`; domain and mailbox writes need
  `write_sensitive`; engine install, relay, kill switch release and DNS
  apply need `root`.

## Alternatives rejected

- **Write our own MTA.** Years of protocol, spam, queue and security work.
  Not our product.
- **Only paid sending providers.** Fine for system mail and stays
  supported, but gives no mailboxes and no self-hosted path.
- **Stalwart as the default.** More features, but six times the idle
  memory, a fresh 1.0 and AGPL paperwork. Kept as phase 5.
- **Ship it as a plugin now.** The plugin sandbox is built for the
  opposite shape (no ports, no volume, local node only). Bending it now
  would weaken the sandbox for every plugin.
- **Drive mox through its CLI via `docker exec`.** Works, but couples us
  to container exec on remote nodes; the HTTP API is cleaner. Kept as a
  fallback for `backup` and `verifydata`, which run in the PreHook anyway.

## Security

- Admin API reachable only on the private network; never published.
- Admin and account credentials sealed in `internal/secrets`, never in
  API responses after creation, redacted by `RedactProviderText`.
- DNS credentials stay in the control plane; the engine never sees them.
- Kill switch, rate limits and no open relay test as above.
- Engine image digest pinned; advisory feed match per ADR 056.

## Failure modes

| Failure | Behaviour |
| --- | --- |
| Engine down | `builtin_mail` fails over to the backup provider; alert |
| Outbound 25 blocked | install detects it, offers relay mode |
| PTR missing | check warns, domain not marked mail ready for outbound |
| Queue grows | alert at `APP_MAIL_QUEUE_WARN` (default 200 messages) |
| Disk fills | warn at the threshold, engine keeps receiving until full, then defers |
| Blocklist hit | kill switch holds the queue, alert, manual release |
| Upgrade breaks data | restore from the pre-upgrade backup |
| Admin API changes upstream | adapter contract test against the pinned image fails in CI |

## What is NOT verified

- The 200 message throughput under load, for either engine.
- Scripted mox admin API calls with the CSRF header.
- mox quota configuration and smarthost (relay) configuration.
- Hot certificate reload, for either engine.
- Stalwart Prometheus metrics (endpoint and auth).

Also unverified: whether the catalog install request accepts a target
node today, and whether the volume backup PreHook can run `mox backup`
inside the pinned image as written.

## What cannot be verified locally

Real deliverability to large mailbox providers, PTR on any cloud,
outbound port 25 policy per provider, blocklist behaviour for a fresh IP.
These need a real node with a public IP and an owner-provided domain.

## Testing plan

- Unit: the mox adapter against a recorded sherpa fake (domains,
  accounts, CSRF header present, errors mapped).
- Live, opt in (`LEVELRAIL_LIVE_MAIL=1`): start the pinned mox image,
  add a domain and account, submit with auth, assert unauthenticated
  relay is refused, run `mox backup` and `verifydata`.
- `builtin_mail`: failover tests reuse `failover_test.go` patterns.
- One focused test per slice; no new guard tests across features.

## Phases

1. DNS engine (`feat/mail-dns`, in review).
2. **First slice, under 2 days**: `mail-mox` template plus
   `internal/mail` with the Engine interface and the mox adapter for
   domains and accounts.
3. Mailboxes UI: dashboard, CLI, aliases, quotas, passwords shown once,
   client settings and autoconfig.
4. `builtin_mail` provider and per-app SMTP credentials.
5. Relay mode, PTR and port 25 checks, queue and blocklist alerts, kill
   switch.
6. Stalwart template (replacing the current HTTP-only one).
7. The `service` plugin kind and migration.

## First thin slice (under 2 days)

Template `mail-mox` pinned by digest with public binds for 25, 465, 587,
993 and a private admin listener; install generates the admin password
into secrets; `internal/mail` Engine interface; mox adapter for
`Domains`, `AddDomain`, `DNSRecords`, `Accounts`, `AddAccount`,
`SetPassword`, `Health`; one API route set under `/api/v1/mail/domains`
behind `APP_EXPERIMENTAL=mail`; DKIM records fed into `mailrecords.Spec`.

## Slices and file ownership

| Slice | Files | Env |
| --- | --- | --- |
| 2 | `internal/catalog/templates_mail.go`, `internal/mail/engine.go`, `internal/mail/mox/*.go`, `internal/api/routes_mail.go` | `APP_MAIL_DEFAULT_MEMORY_MB`, `APP_MAIL_DEFAULT_CPUS` |
| 3 | `web/src/routes/settings/mail/*`, `web/src/locales/en/mail.json`, `cmd/levelrail/mail*.go` | `APP_MAIL_PASSWORD_LENGTH` |
| 4 | `internal/email/builtin_mail.go`, `internal/mail/appaccounts.go` | `APP_MAIL_SYSTEM_RATE_PER_MIN`, `APP_MAIL_APP_SEND_PER_HOUR` |
| 5 | `internal/mailrecords/ptr.go` (after `feat/mail-dns` merges), `internal/alerting/mail_queue.go`, `internal/mail/killswitch.go` | `APP_MAIL_QUEUE_WARN`, `APP_MAIL_OUTBOUND_KILL_PER_HOUR`, `APP_MAIL_DISK_WARN_PCT` |
| 6 | `internal/catalog/templates_selfhosted4.go` (Stalwart entry), `internal/mail/stalwart/*.go` | none new |
| 7 | `internal/plugin/manifest.go`, `proto/plugin/v1/plugin.proto`, `docs/plugins.md` | none new |

Shared files (`internal/api/routes.go`, `internal/email/provider.go`,
`internal/experimental`) are touched in sequence, never by two slices at
once.

## Consequences

- Operators get mailboxes and self-hosted system mail without a paid
  provider, at the cost of running a mail server they are responsible for.
- Levelrail takes on an upstream it does not control (pre 1.0 for mox);
  the adapter contract test and digest pin limit the blast radius.
- Support load rises around deliverability; the honest copy and relay
  mode are how we keep it bounded.
- The `service` plugin kind is the larger, later change; the managed
  template lets us learn first.

## Open questions

1. Is self-hosted mail in scope before 1k stars, or does it stay behind
   the experimental flag until then?
2. Do we keep, rename or replace the existing HTTP-only Stalwart catalog
   entry now, before phase 6?
3. Should `builtin_mail` ever be allowed as the only provider, or must a
   backup provider exist while the IP is warming up?
4. Which relay providers do we document first for blocked port 25?
5. Legal review of the AGPL boundary before the Stalwart template ships:
   who and when?
6. Is a controlled restart on certificate renewal acceptable if hot
   reload turns out unsupported?
7. Default for uninstall: keep the volume, or back up then delete?
