# ADR 062: Mail server certificates

Partly superseded by [ADR 064](064-email-for-apps.md): new mail work follows ADR 064.

Status: Proposed

Date: 2026-10-30

## Context

SMTP on 25 (STARTTLS), submission on 587 (STARTTLS required before AUTH),
submissions on 465 and IMAPS on 993 all need a certificate that is valid
for the mail host name (`MAIL_HOSTNAME`, default `mail.<domain>`). Mail
clients refuse anything else, and a sending server that honours the
domain's MTA-STS policy in `enforce` mode (ADR 059, section 2.10) will
not deliver to a host whose certificate does not match the MX name.

What exists today, verified by reading `origin/release/v0.8.12`:

1. **The template makes a self-signed certificate.**
   `internal/catalog/templates_mail.go` runs `mox localserve -initonly`
   on first start when neither certificate input is set, copies
   `localhost.crt` and `localhost.key` into `config/certs/selfsigned.pem`
   and `.key` (key `chmod 640`), and drops a `config/certs/.selfsigned`
   marker. The certificate is for `localhost` only. `docs/mail-template.md`
   says so and tells the operator to replace it.
2. **A user can supply files.** `MAIL_TLS_CERT_FILE` and
   `MAIL_TLS_KEY_FILE` are paths under `/mox/config`. Existing files are
   never overwritten; if only one is set, or a file is missing, the start
   script exits with one line naming the missing file. The generated
   `mox.conf` lists them under `public.TLS.KeyCerts`.
3. **mox cannot run its own ACME here.** The platform's embedded Caddy
   (ADR 005) owns ports 80 and 443 on every node that runs the ingress,
   so mox cannot answer HTTP-01 or TLS-ALPN-01. The template publishes
   neither port for mox.
4. **mox does not reload certificates.** ADR 059 measured on v0.0.17:
   replacing the `KeyCerts` files was not picked up, there is no
   `mox config reload`, and HUP stopped the server. ADR 060 records the
   same fact. A certificate change means a container restart.
5. **The ingress already issues certificates.** Caddy's certmagic
   storage is backed by SQLite (`internal/ingress/certstorage.go`,
   `SQLiteStorage`), and every leaf `.crt` write emits an audit row. A
   DNS-01 path exists for wildcards (`DNSChallengeConfig`,
   `DNS01Provider`, Cloudflare and Route53 wire types in
   `internal/ingress/config.go`, Google Cloud in `dns_googlecloud.go`;
   `requireWildcardDNS01` in `internal/api/dns_zones_wildcard.go` refuses
   wildcards when no provider is active). Operator uploaded certificates
   for app domains go through `internal/secrets` under
   `domain-tls-cert/<domain>` (`internal/store/domain_tls_cert.go`), so
   that path is sealed at rest.
6. **Certificate alert policy is already env driven.**
   `internal/alerting` reads `APP_DEFAULT_ALERT_CERT_DAYS`,
   `APP_ALERT_CERT_RENEW_RATIO`, `APP_ALERT_CERT_LAST_RESORT_HOURS` and
   `APP_ALERT_CERT_REMINDER_INTERVAL`.
7. **The agent can write into a service volume.**
   `internal/agent/volume_files.go` exposes `VolumeList`, `VolumeRead`,
   `VolumeWrite` and `VolumeRemove` over the exec frame stream (and a
   `Local` variant for the control plane's own node). Size caps come
   from the agent's env, so the control plane cannot raise them.
   `internal/volumefiles.Store.Write` writes to a temp file and renames
   it, so a reader never sees half a file. **Gap:** a new file gets mode
   `0644`; only an existing file keeps its mode. A private key written
   this way today would be world readable inside the volume.
8. **Volumes are per service.** `internal/deploy/translate.go`
   `volumeName` returns `"app-" + serviceName + "-" + logicalName`, so the
   mail config volume is `app-<service>-mail_config` and no other service
   can mount it.

## Decision

**Recommended: option A.** The control plane obtains the certificate for
the mail host name through the existing ingress ACME, delivers the chain
and key into the mail service's config volume through the node agent,
and restarts mox in a controlled way. Renewal is driven by the control
plane. User supplied files (option C) stay as the override.

### A.1 Issuing

- The mail host name is added to the ingress as a certificate only
  name: managed by certmagic, no HTTP route. Whether the current config
  builder can express this (Caddy's `tls.certificates.automate` list) was
  not confirmed while writing this ADR; slice 1 checks it first.
- Challenge choice, in order:
  1. **DNS-01** when the zone of the mail host is served by a DNS
     provider already connected (the same provider the mail DNS engine
     in `internal/mailrecords` uses through `dnszones.Provider`). This
     works wherever the mail node is and during a migration. No wildcard
     is requested: one name, one certificate.
  2. **HTTP-01** only when the mail host's A and AAAA records point at a
     node whose ingress answers port 80. On a dedicated mail node with
     no ingress (the advised layout in ADR 059, 2.1) this fails, and the
     status page says so and asks for a DNS provider.
- Key type follows the ingress default. If TLSA records are published
  (ADR 059, DANE), renewal must reuse the key; whether certmagic's key
  reuse is exposed through our config is UNVERIFIED.

### A.2 Delivery

1. Read chain and key from certmagic storage in the control plane.
2. Validate before touching the node: key matches the leaf, a SAN covers
   `MAIL_HOSTNAME`, chain verifies, `NotAfter` is later than the file
   currently on disk.
3. Through `VolumeWrite`, write to `app-<service>-mail_config`:
   `certs/levelrail.pem` (mode `0644`) and `certs/levelrail.key` (mode
   `0600`, owner `1000:1000`, the uid the start script already chowns
   to). The previous pair is kept as `certs/levelrail.prev.pem` and
   `.prev.key` for rollback. This needs a mode option on
   `volumefiles.Store.Write` (or a create with mode then write), since
   new files default to `0644` today.
4. The first delivery switches the app's `MAIL_TLS_CERT_FILE` and
   `MAIL_TLS_KEY_FILE` inputs to the new paths and removes the
   `.selfsigned` marker. User supplied paths are never touched (see C).

### A.3 Restart

- mox does not reload certificates (measured, v0.0.17), so delivery is
  followed by a restart of the mail container through the normal deploy
  restart path, never by `docker kill` or HUP.
- The restart runs inside `APP_MAIL_RESTART_WINDOW` (already named in
  ADR 059) unless the served certificate expires within
  `APP_MAIL_CERT_FORCE_RESTART_HOURS`, in which case it runs now.
- Before the restart the controller waits up to
  `APP_MAIL_CERT_DRAIN_SECONDS` for the submission and SMTP connection
  gauges from `/metrics` to reach zero.
- After the restart it probes 25 and 587 (STARTTLS), 465 and 993
  (implicit TLS) from the agent and compares the served serial with the
  delivered one. If mox does not become healthy, or serves the old
  serial, it restores the `.prev` pair, restarts again and raises an
  alert.

### A.4 Renewal

- Renewal is certmagic's own timer for the name; the mail controller
  reacts to the audited `.crt` write for the mail host (the hook in
  `SQLiteStorage.Store`) and to a periodic reconcile every
  `APP_MAIL_CERT_RECONCILE_INTERVAL`.
- The controller delivers a new certificate when the served one is
  within `APP_MAIL_CERT_RENEW_BEFORE_DAYS` of expiry or differs from the
  stored one. Alert thresholds reuse `APP_DEFAULT_ALERT_CERT_DAYS`. No
  value is hardcoded; missing env keeps a documented default in one
  config struct.

### A.5 Failure behaviour

- Issue fails: nothing on the node changes. mox keeps serving the old
  certificate. Alert per the existing cert alert policy; retry with
  backoff capped by `APP_MAIL_CERT_RETRY_MAX_INTERVAL`.
- Delivery fails (agent offline, volume write refused): same, plus the
  status shows "certificate issued, not delivered".
- Restart fails: roll back to `.prev`, alert, keep the new certificate
  in storage for the next window.
- The mail status card (ADR 059, section 6) shows "certificate expires in
  N days", the source (platform ACME, uploaded, self-signed), the
  challenge used, and a warning when served and stored certificates
  differ.

## Alternatives rejected

- **B. mox runs its own ACME with DNS-01.** It would need DNS provider
  credentials inside the mail container, the most exposed workload on
  the instance (public 25, 465, 587, 993). Rejected unless the
  credentials are sealed, scoped to one TXT name, and never visible in
  the container env. Whether mox v0.0.17 supports DNS-01 at all is
  UNVERIFIED; its documented ACME paths need 80 or 443, which the
  platform owns.
- **C. User supplied certificates only.** Kept as the override, not the
  default: most operators will not renew by hand every 60 to 90 days,
  and a missed renewal silently breaks inbound mail under MTA-STS
  enforce. When `MAIL_TLS_CERT_FILE` points somewhere other than
  `certs/levelrail.pem`, the controller only reports expiry and never
  writes. `mail cert upload` stores an uploaded pair sealed through
  `internal/secrets` (like `domain-tls-cert/<domain>`) and delivers it
  the same way as A.2.
- **D. A sidecar certificate service.** Volumes are per service
  (`volumeName` in `internal/deploy/translate.go` prefixes the service
  name), so a sidecar cannot mount `app-<mail>-mail_config`. Making it
  possible would weaken the per service isolation ADR 059 relies on, and
  the sidecar would still need port 80 or DNS credentials.
- **E. Pass TLS-ALPN-01 through the ingress to mox.** `internal/ingress`
  has a layer4 path, but routing the mail host on 443 to mox couples the
  ingress to mail, keeps ACME state inside the mail container, and fails
  on a dedicated mail node anyway.

## Security

- The private key is never logged, never placed in env, never returned
  by the API after upload, and never written to audit. Audit subjects
  carry service id, host name, serial, `NotAfter` and the SHA-256 of the
  leaf (all public).
- At rest in the control plane: uploaded keys are sealed through
  `internal/secrets`. Whether ACME keys in `cert_storage` are encrypted
  at rest was not confirmed while writing this ADR; if not, this is a
  gap shared with every app domain, tracked as an open question rather
  than solved only for mail.
- In the volume: key `0600`, owner `1000:1000`, directory `0700`.
- The agent writes only to the mail service's own config volume, with
  the agent side size cap, over the existing mTLS channel.
- Per service isolation (ADR 059): the key exists in the control plane
  store and in one volume. It is never shared with another service.

## IAM and audit

Resource `mail:service:<id>` as in ADR 059, 2.6 (ADR 060 writes
`mail_service:<name>`; see open questions).

| Action | Ability | Step-up |
| --- | --- | --- |
| `mail cert status` | `read` | no |
| `mail cert renew` (force issue and deliver now) | `write:sensitive` | no |
| `mail cert upload` | `write:sensitive` | yes (`manage`) |
| Switch the source between platform ACME and uploaded | `root` within the team | yes |

Audit rows: `mail.cert.issued`, `mail.cert.delivered`,
`mail.cert.restarted`, `mail.cert.rolled_back`, `mail.cert.uploaded`,
`mail.cert.failed` (reason code only).

## Surface sketch

All behind `APP_EXPERIMENTAL=mail`. Paths are a sketch.

- CLI: `levelrail-cli mail cert status`, `mail cert renew [--now]`,
  `mail cert upload --cert <file> --key <file>`.
- API: `GET /mail/services/{id}/cert`, `POST /mail/services/{id}/cert/renew`,
  `PUT /mail/services/{id}/cert`.
- UI: the certificate row on the mail status card, a "Renew now" action,
  and an upload form that validates the pair in the browser before
  sending.

## Interaction with ADR 060 (migration)

- The certificate files live in `mail_config` and move with it, so the
  new node serves the same certificate on day one.
- While a migration is open, renewal for the mail host uses DNS-01 only,
  because the A record is in flux. If no DNS provider is connected and
  the certificate expires within the migration's rollback limit, the
  migration preflight refuses to start.
- After cutover the controller re-delivers to the new node's volume
  (`app-<service>-mail_config` on the new node) and verifies the served
  serial there.
- If TLSA records exist, the key does not change during the window.
- When the whole instance moves (ADR 060, case B), certmagic storage
  moves with the control plane database.

## Env settings

| Name | Purpose |
| --- | --- |
| `APP_MAIL_RESTART_WINDOW` | Quiet window for the restart (from ADR 059) |
| `APP_MAIL_CERT_RENEW_BEFORE_DAYS` | Deliver when the served cert is this close to expiry |
| `APP_MAIL_CERT_FORCE_RESTART_HOURS` | Ignore the window below this many hours left |
| `APP_MAIL_CERT_DRAIN_SECONDS` | Wait for open connections before restart |
| `APP_MAIL_CERT_RECONCILE_INTERVAL` | Periodic check of served versus stored |
| `APP_MAIL_CERT_RETRY_MAX_INTERVAL` | Backoff cap after a failure |
| `APP_DEFAULT_ALERT_CERT_DAYS` | Existing alert thresholds, reused |

## UNVERIFIED, to prove on a real host

1. Whether a newer mox release reloads `KeyCerts` without a restart.
2. Whether a restart loses inbound mail. Expected: a sender mid DATA sees
   a dropped connection and retries later, and queued mail is in
   `queue.db` on the data volume, but this has not been measured.
3. Whether a restart drops IMAP IDLE clients in a way that clients
   recover from without prompting for a password.
4. Whether the ingress config builder can hold a certificate only name.
5. DNS-01 issuance for the mail host through each connected provider.
6. Key reuse across renewals for TLSA.
7. Whether `cert_storage` values are encrypted at rest.

## Testing plan

- Unit: pair validation (mismatched key, wrong SAN, expired, older than
  disk), window and env parsing, rollback decision, a fake
  `VolumeFileOps` that records paths and modes, and an audit test that
  asserts no key bytes appear in any row.
- One focused test for the new `volumefiles` mode option.
- Real host: staging CA, DNS-01, issue and deliver, `openssl s_client
  -starttls smtp` on 25 and 587, implicit TLS on 465 and 993, serial
  match, forced restart failure to exercise rollback, a send from an
  outside server during the restart.

## Slices

1. **First slice, under 2 days.** DNS-01 issue for the mail host, A.2
   delivery with modes, A.3 restart and probe, and `mail cert status` and
   `mail cert renew --now` in the CLI and API. No timer yet.
   Files: new `internal/mailcert/` (issue, deliver, probe, config from
   env), `internal/volumefiles/volumefiles.go` (mode option, shared:
   sequence it), `internal/api/mail_cert.go`,
   `cmd/levelrail-cli/mail_cert.go`.
2. Renewal reconcile, quiet window, drain, alerts. Files:
   `internal/mailcert/reconcile.go`, the scheduler wiring in `cmd/levelrail`.
3. HTTP-01 fallback and upload (sealed). Files: `internal/mailcert/upload.go`,
   `internal/api/mail_cert.go`.
4. UI row and upload form on the mail status card. Files: the mail area
   in the dashboard only.
5. ADR 060 hooks: DNS-01 only during a migration, preflight refusal,
   re-delivery after cutover. Files owned by the migration controller.

## Consequences

- Mail gets real certificates with no extra credentials in the mail
  container and no extra ports.
- Every renewal costs a mox restart until mox reloads certificates, so
  there is a short window per renewal where inbound connections fail
  and senders retry.
- The control plane becomes the single place that must be healthy for
  renewal. If it is down past the alert thresholds, mail keeps serving
  the old certificate until it expires.
- `volumefiles` gains a mode option, which other secret file writes can
  reuse.

## Open questions

1. Should a missing DNS provider block the platform ACME path for mail,
   or is HTTP-01 on a node running the ingress a supported layout?
2. Are ACME keys in `cert_storage` encrypted at rest today, and if not,
   is fixing that for every domain in scope before this ships?
3. Which IAM naming wins: `mail:service:<id>` (ADR 059) or
   `mail_service:<name>` (ADR 060)?
4. What default restart window and drain time are acceptable, given that
   a restart has not been measured against live inbound mail?
5. Should the self-signed first start remain once platform ACME works, or
   should the template wait for the first issued certificate instead?
