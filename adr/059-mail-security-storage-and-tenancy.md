# ADR 059: Mail security, storage and team ownership

Partly superseded by [ADR 064](064-email-for-apps.md): new mail work follows ADR 064.

Status: Accepted (owner defaults recorded, see Decisions at the end)

Date: 2026-10-29

Builds on ADR 058 (optional in-house mail server, mox by default), ADR 034
(plugin contract) and ADR 056 (signed runtime advisory feed). It does not
change the ADR 058 engine choice. It decides how the mail engine is secured,
where mail data lives, who owns it on an instance with several teams, and
what the dashboard shows.

## Context

The first question every adopter of a self-hosted mail server asks is "how
secure is it". The second is "where is my mail and how do I get it back".
This ADR answers both, plus how mail fits teams and organisations on one
instance.

### What exists today (verified by reading `origin/release/v0.8.12`)

- `internal/catalog/templates_mail.go` ships a `mail-mox` template pinned
  to `r.xmox.nl/mox:v0.0.17@sha256:8889e7b4...`. Its generated mox config
  puts `AdminHTTP`, `AccountHTTP`, `WebmailHTTP`, `WebAPIHTTP` and
  `MetricsHTTP` on one `internal` listener bound to `0.0.0.0` inside the
  container, published as host ports 8025 and 8010 (private bind by
  default through `internal/bindaddr`). The admin password arrives as the
  env var `MAIL_ADMIN_PASSWORD` and is re-applied with
  `mox setadminpassword` on every start. The compose sets no `cap_drop`,
  no `no-new-privileges` and no `read_only`. The default TLS path writes
  a mox ACME block but publishes neither 80 nor 443.
- `internal/catalog/catalog.go:75` appends `mailTemplates` without
  checking `APP_EXPERIMENTAL=mail`, so the template is visible in the
  public catalog.
- `internal/mail` defines an `Engine` interface with `DropQueue` (drops
  every queued message) and no hold or release. `internal/mail/mox` logs
  in with the session flow (`LoginPrep`, `Login`, then the
  `x-mox-csrf` header). ADR 058 says Basic auth; that is wrong and this
  ADR corrects it.
- `internal/mailrecords` plans add-only mail DNS with plan hashes and
  refuses a foreign MX.
- `internal/backup` already streams volume backups as tar, zstd, then age
  (`seal.go`, `Sealer.Seal`, `OpenSealed`), so buckets hold ciphertext.
- Backup targets (`store.BackupTarget`: ID, Name, Provider, Endpoint,
  Region, Bucket, EncryptDumps) are **instance global**: no team or
  project field. Credentials live in `internal/secrets` under
  `backup-target/<id>` and are write-only through the API.
- Storage destinations have a connection test that puts, gets and
  deletes a probe object under `.probe/` with stable failure reasons
  (`invalid_credentials`, `access_denied`, `bucket_not_found`, ...).
- Step-up auth exists: `POST /api/v1/auth/step-up` with actions
  `reveal`, `export` and `manage` (`internal/api/step_up.go`).
- IAM resources today are `app:`, `database:`, `node:`, `environment:`
  and `environment-kind:`. Policies have no conditions. Teams own
  projects; apps and databases belong to a project and so to a team
  (`docs/teams-and-roles.md`).
- `internal/authengine` wraps the theauth Go library (passkeys, TOTP,
  OAuth and SAML SSO, sessions, tokens) for Levelrail users.

### What we measured (mox v0.0.17, Docker on arm64, loopback only, no real mail sent)

- Idle RAM 13.5 MiB at start, about 14 to 24 MiB idle, 36.4 MiB after 50
  authenticated submissions over one connection (mean 6.9 ms per message
  into the queue), CPU 0.01%.
- Admin API is sherpa JSON-RPC at `/admin/api/`; Basic auth does not
  work; session login plus CSRF header does. HTTP status is 200 even for
  errors, so the adapter must read the `error` object.
- Admin, account, webmail and webapi sit on the internal HTTP listener;
  SMTP and IMAP on the public listener; metrics on 8010.
- No API function sets the mailbox quota. `QuotaMessageSize` is set in
  `domains.conf`, which mox picked up with no restart.
  `AccountSettingsSave` covers `MaxOutgoingMessagesPerDay` and
  `MaxFirstTimeRecipientsPerDay`.
- Replacing `KeyCerts` files was not picked up within a few seconds; a
  restart is needed. There is no `mox config reload` in v0.0.17.
- A wrong password on submission returned `535 5.7.8 bad user/pass`.
- `/metrics` returned about 550 lines including `mox_queue_hold`,
  `mox_authentication_total` (with a result label, so failures count)
  and `mox_smtpserver_submission_total`.
- mox always queues submissions, even to local recipients.

### Shipped open source mail servers we learned from

| Project | What we take | Status |
| --- | --- | --- |
| Mail-in-a-Box | A single status checks page that re-checks DNS and emails the admin when a check changes; encrypted backups, local by default or to S3 compatible storage with no local copy kept; the admin must copy the backup encryption key off the box | VERIFIED, mailinabox.email/guide.html. Uses duplicity: UNVERIFIED here |
| Mailcow | Backup script per component (`vmail`, `crypt`, `redis`, `rspamd`, `postfix`, `mysql`), local folder `mailcow_DATE`, `--delete-days` pruning, no S3 and no encryption described | VERIFIED, docs.mailcow.email backup page. Admin UI TOTP and WebAuthn exist as guides: VERIFIED by sidebar only |
| Mailu | Durable state is an admin SQLite volume plus the Maildir volume; the queue and caches are treated as ephemeral | Secondary source only (a community recipe): UNVERIFIED against Mailu docs |
| Stalwart | Separate blob store, S3 compatible including R2, MinIO, B2, Wasabi, with a `keyPrefix` so several servers share a bucket; bucket must already exist; `verifyAfterWrite` on by default | VERIFIED, stalw.art/docs/storage/backends/s3 |

### mox facts checked against upstream docs

| Claim | Status |
| --- | --- |
| Each listener enables `AdminHTTP`, `AccountHTTP`, `WebmailHTTP`, `WebAPIHTTP` (each with `Enabled`, `Port`, `Path`, HTTPS variants) and `MetricsHTTP` (`Enabled`, `Port`, default 8010) independently | VERIFIED, xmox.nl/config |
| `QuotaMessageSize` global and per account; `MaxOutgoingMessagesPerDay` (default 1000) and `MaxFirstTimeRecipientsPerDay` (default 200) per account in `domains.conf` | VERIFIED, xmox.nl/config |
| No TOTP, no two-factor and no app password settings; closest are `NoCustomPassword` and `LoginDisabled` per account | VERIFIED, xmox.nl/config |
| No login attempt limit setting in the config reference | VERIFIED, xmox.nl/config. Whether mox throttles auth failures internally: UNVERIFIED |
| `DataDir` holds queue, accounts, messages and ACME material; each account has its own directory with messages and an index database | VERIFIED, xmox.nl/config |
| No S3 or object storage option | VERIFIED, xmox.nl/config (absence in the reference) |
| `mox backup <destdir>` makes a consistent snapshot copy of config and data, hard linking message files, and refuses to overwrite; copying a live data dir can give unusable databases | VERIFIED, xmox.nl/commands |
| `mox verifydata` checks databases and message files and may apply schema upgrades, so run it on a copy | VERIFIED, xmox.nl/commands |
| `mox export maildir|mbox <dst> <accountpath> [mailbox]` opens the account database directly and may block while mox runs; on a live instance use the account page or webmail | VERIFIED, xmox.nl/commands |
| `mox import maildir|mbox` exists; the account page accepts zip or tgz uploads | VERIFIED, xmox.nl/commands |
| `domains.conf` reloads on change; `mox.conf` changes need a restart | VERIFIED, xmox.nl/commands |
| Encryption at rest is not implemented; on the roadmap as encrypted storage with per account keys | VERIFIED, xmox.nl/features |
| OAuth2 single sign on is a roadmap item, not available | VERIFIED, xmox.nl/features |
| mox serves MTA-STS policies, implements DANE for outbound when the recipient zone is DNSSEC signed, REQUIRETLS, and inbound and outbound TLS reporting | VERIFIED, xmox.nl/features |
| Webmail is "still in early stages" | VERIFIED, xmox.nl/features |

## Decision

Eight recommendations, then the detail.

1. **Mail runs on its own node by default**, as a hardened container
   (all capabilities dropped except bind, no new privileges, read-only
   root, secrets as files). Placing it on the control plane node needs an
   explicit confirm and is labelled not recommended.
2. **The Levelrail dashboard is the only admin UI.** mox `AdminHTTP` runs
   on its own listener bound to the node's mesh address, reachable only
   from the control plane. mox's own admin web page is never routed by
   the ingress. Break-glass access is a short-lived tunnel opened through
   the control plane after step-up.
3. **Mailbox identities are separate from Levelrail users**, owned by a
   team, optionally linked to a team member. Removing the member suspends
   or transfers the mailbox. Mailbox users sign in to webmail and IMAP
   with generated passwords per device; Levelrail never stores them.
4. **New IAM resources and abilities**: `mail:service:<id>`,
   `mail:domain:<d>`, `mail:mailbox:<address>`, with step-up for
   destructive actions and audit rows that never contain bodies,
   passwords or full message headers.
5. **Live mail stays on the mail node's local volume**, size capped,
   because mox has no object storage backend. Object storage is for
   encrypted backups, age based archive and export. A stateless mail node
   is a later Stalwart option, not a promise now.
6. **Each mail service brings its own bucket**: a mail storage connection
   owned by that mail service, independent of instance backup targets,
   sealed per service, invisible to other teams, with prefix
   `<service-id>/`, a full put, get, list, delete probe on connect, and
   age encryption before upload.
7. **Zero cost when off**: with the flag off or no mail service
   installed the control plane starts no goroutine, reads no mail table
   and polls nothing. Mail metrics come from mox's Prometheus endpoint
   through the existing node-local telemetry path.
8. **An honest public security statement** ships with the docs: what is
   protected (transport, admin plane, backups) and what is not (mail at
   rest on the node is plaintext on disk unless the operator encrypts the
   volume).

### 1. Threat model

**Assets**, highest value first:

1. The control plane master key and the secrets it unseals (DNS provider
   credentials, backup and mail bucket keys, the mox admin password).
2. The Docker socket and agent on every node (root equivalent there).
3. Mail content on the mail node: messages, index databases, the queue.
4. Mail credentials: mailbox passwords (hashed by mox), DKIM private
   keys, TLS private keys, the mox admin password.
5. The node's IP reputation and the operator's domains (spoofing and
   abuse cost).
6. Backups and archives in object storage.

**Attackers**:

- **Internet** at large: scanners, credential stuffing on IMAP,
  submission and webmail, malformed MIME aimed at the parser, spammers
  looking for an open relay.
- **A mailbox user** (or whoever stole their password): tries to send
  spam, read other mailboxes, reach the admin API or other services on
  the node.
- **A team member with limited abilities**: tries to read another team's
  mailboxes, buckets or audit log.
- **A compromised app on the same node**: tries to reach the mox admin
  listener or the mail volume.
- **A compromised upstream image**: a malicious or vulnerable mox release.
- **The bucket provider**: can read or delete objects in the bucket.

**Trust boundaries**:

```
 Internet ──25,465,587,993──▶ [ mail node: mox container ]
    │                              │  admin + metrics listener
    │ 443 (webmail host)           │  bound to mesh IP only
    ▼                              ▼
 [ ingress ] ──mesh──────▶ [ webmail listener ]
                                   ▲
 [ control plane: master key ] ────┘ WireGuard mesh, admin session,
        │ agent mTLS                 metrics scrape
        ▼
 [ agent + Docker socket on mail node ]
        │ age ciphertext only
        ▼
 [ team's bucket ]
```

- Boundary A, internet to mail node: only SMTP, submission, IMAPS and
  the webmail hostname cross it.
- Boundary B, mail node to control plane: the mail node is assumed
  hostile to the control plane. It never holds the master key, DNS
  credentials or bucket credentials at rest. The control plane reaches
  the admin listener over the mesh; the mail node cannot call the
  control plane API with any credential.
- Boundary C, team to team: enforced by IAM on every mail route and on
  the storage connection.
- Boundary D, node to bucket: only ciphertext crosses.

### 2. Layered controls

#### 2.1 Dedicated node: enforced versus advised

| Control | Enforced or advised |
| --- | --- |
| Install refuses the control plane node unless `--allow-control-plane-node` (API field `allow_control_plane_node`) is set, with a warning | Enforced |
| One mail engine per node (label `levelrail.mail=engine`) | Enforced |
| The mail node runs no other team's apps (placement refuses apps from another team onto a node labelled for mail) | Enforced when `APP_MAIL_NODE_EXCLUSIVE=true` (default true) |
| Separate public IP from the control plane and from web apps | Advised; the install shows whether the IP matches the control plane |
| Disk encryption on the mail volume (LUKS or provider encrypted disk) | Advised; we cannot verify it, the status page shows "unknown" |
| PTR set, port 25 open outbound | Checked, shown, not enforced (relay mode exists) |

#### 2.2 Container hardening (template v2)

- `cap_drop: [ALL]`, `cap_add: [NET_BIND_SERVICE]`, `security_opt:
  [no-new-privileges:true]`, `read_only: true` with writable mounts only
  for the config and data volumes and a small `tmpfs` for `/tmp`.
- Runs as uid 1000 (mox refuses root with `$MOX_SOCKETS`, measured by
  us); the init step that chowns volumes is a separate one-shot
  container, so the long-running one never starts as root. Whether the
  pinned image starts cleanly this way is UNVERIFIED.
- No Docker socket, no host network, no privileged mode.
- The admin password is a file in a secrets mount (`0400`, owned by
  1000), written by the agent from the sealed value, never an env var, so
  `docker inspect` and `/proc/1/environ` do not show it.
- Memory, CPU and pids limits from `APP_MAIL_DEFAULT_MEMORY_MB`,
  `APP_MAIL_DEFAULT_CPUS`, `APP_MAIL_DEFAULT_PIDS`.
- The template is filtered out of the catalog unless
  `APP_EXPERIMENTAL=mail` is set (fixes the ungated entry).

#### 2.3 Listeners: three, not one

| Listener | Binds to | Enables | Reached by |
| --- | --- | --- | --- |
| `public` | node public IP | SMTP 25, Submission 587 (STARTTLS required), Submissions 465, IMAPS 993 | Internet |
| `webmail` | mesh IP | `WebmailHTTP`, `AccountHTTP` (with `Forwarded: true` so rate limits see client IPs) | Ingress only, on its own hostname such as `webmail.<domain>` |
| `admin` | mesh IP, separate port | `AdminHTTP`, `WebAPIHTTP` off unless used, `MetricsHTTP` | Control plane only |

mox enables these per listener (VERIFIED, xmox.nl/config). IMAP 143 and
plain POP are off. The admin listener port is never published on any
interface other than the mesh, and the agent's firewall rules drop
traffic to it from any mesh peer except the control plane (UNVERIFIED
that the agent can express a per peer rule today; slice 1 checks).

**mox's own admin UI** is disabled from every route the ingress serves.
Break-glass: `levelrail-cli mail admin-tunnel <service>` after step-up
opens a 15 minute (`APP_MAIL_BREAKGLASS_MINUTES`) local port forward
through the control plane, audited at open and close.

#### 2.4 Mailbox client access

- TLS only for every client protocol. Submission on 587 requires
  STARTTLS before AUTH; 465 is implicit TLS.
- **Generated passwords, no app passwords yet.** mox has no app password
  feature (VERIFIED). Every account is created with
  `NoCustomPassword: true`, so only Levelrail generated passwords
  (length `APP_MAIL_PASSWORD_LENGTH`) are used. Honest limit: as far as
  we know mox holds one password per account, so "revoke one device"
  means rotate and re-issue to every device. UNVERIFIED whether mox
  supports several credentials per account; we do not claim per device
  revocation until it does.
- **Auth failure limits.** mox has no configurable lockout (VERIFIED by
  absence). We add a control plane watcher that reads
  `mox_authentication_total{result="..."}` and the mox log, and after
  `APP_MAIL_AUTH_FAIL_LIMIT` failures per account in
  `APP_MAIL_AUTH_FAIL_WINDOW` sets `LoginDisabled` on that account for
  `APP_MAIL_AUTH_LOCKOUT_MINUTES`, with an alert. Per IP blocking is done
  at the node firewall by the agent. Whether mox has built in auth
  throttling: UNVERIFIED.
- **TOTP:** mox has none for mailbox users (VERIFIED). Webmail is
  therefore protected by a strong generated password and the lockout
  above. Optional: put the webmail hostname behind Levelrail sign-in at
  the ingress (forward auth with authengine, passkey or TOTP) for teams
  whose mailbox users are all Levelrail users. Off by default because it
  breaks external mailbox users.
- **theauth fit.** theauth (the library behind authengine) covers
  Levelrail users: passwords, passkeys, TOTP, SSO, SCIM, an OAuth 2.1
  authorization server. It has no IMAP or SMTP SASL integration, and mox
  has no OAuth sign in today (roadmap only, VERIFIED). So theauth fits
  the dashboard side and the optional webmail forward auth, and does not
  fit IMAP and SMTP authentication. If mox ships OAuth2, theauth's
  authorization server could issue tokens for it; that is a later ADR.

#### 2.5 Identities, ownership and offboarding

- A **mailbox** is its own record (`mail_mailboxes`: service, domain,
  address, team, optional `linked_user_id`, state `active|suspended`).
  It is not a Levelrail user and cannot sign in to the dashboard.
- A **mail domain** belongs to one team (fixes "domains are instance
  global"). A second team cannot add the same domain.
- **Team member mailbox**: a team admin picks a member; the address is
  prefilled from the member's name and the team's default mail domain;
  the password is shown once to the admin to hand over, or emailed to the
  member's verified address through the normal email provider as a one
  time link (UNVERIFIED that the email path supports one time secret
  links today).
- **Offboarding**: removing a member from a team, or disabling the user,
  finds linked mailboxes and applies the team's offboarding rule
  (`suspend` default, `transfer` to a named member, or `forward` to an
  alias). Suspend sets `LoginDisabled` and keeps receiving. Delete is
  never automatic; it needs step-up and a retention window.

#### 2.6 Abilities and IAM resources

New resource prefixes for the existing evaluator: `mail:service:<id>`,
`mail:domain:<domain>`, `mail:mailbox:<address>`. A mailbox also carries
its domain and service resource, the same way an app carries its
environment, so `Deny mail:domain:example.com` hides every mailbox there.

| Action | Ability | Step-up |
| --- | --- | --- |
| View service health, domains, mailbox list, queue counts | `read` | no |
| View queue entry details (sender, recipient domain, last error) | `read:sensitive` | no |
| Add mailbox, alias, change quota, reset password | `write:sensitive` | reset: yes (`manage`) |
| Add or remove domain, apply mail DNS | `root` within the team, or `write:sensitive` plus an Allow on the domain | yes |
| Delete mailbox, delete service, delete bucket data, restore over live data | `root` | yes, plus typed confirm |
| Hold or release queue, outbound kill switch release | `write:sensitive` | release: yes |
| Export a mailbox | `read:sensitive` | yes (`export`) |
| Connect or change the mail storage connection | `root` within the team | yes |
| Break-glass admin tunnel | `root` (instance) | yes |

The DNS plan route moves from `read` to `write` because it returns
existing record values.

#### 2.7 Audit

Every mail write is audited through the shared gate. Subjects carry
service id, domain and address; never passwords, message ids, subjects,
bodies or bucket keys. Extra rows: lockouts, kill switch trips, backup
and restore runs, exports (who, which mailbox, size), break-glass open
and close.

#### 2.8 The control plane never reads message bodies

The adapter calls only management functions. Queue views show sender
account, recipient domain, attempts, next attempt and last error; never
subject or body. No API route proxies webmail. Backups and exports are
streamed and sealed on the mail node (see 4), so plaintext never passes
through the control plane. Mail content is never written to SQLite.

#### 2.9 Supply chain

Image pinned by digest; a bump is a reviewed change that runs the
adapter contract test against the new image. The ADR 056 advisory feed
matches the pinned mox version; a matching advisory raises an alert and
blocks new installs of that digest, never auto-upgrades. Every upgrade
takes a fresh backup first and blocks if it fails (mox has no
downgrade).

#### 2.10 TLS policy

- Certificates from Levelrail's ACME, written as files into the config
  volume by the agent; renewal triggers a controlled restart in a quiet
  window (`APP_MAIL_RESTART_WINDOW`) because mox does not reload them
  (measured). The status page alerts when the served certificate expires
  sooner than the one on disk.
- MTA-STS: published by the DNS engine, policy served through the
  ingress at `mta-sts.<domain>`, mode `testing` first, `enforce` after
  the operator confirms reports are clean.
- TLS-RPT published; mox ingests reports (VERIFIED).
- DANE: outbound DANE is mox's (VERIFIED). Inbound DANE (TLSA records)
  only when the zone is DNSSEC signed and the DNS provider supports TLSA;
  the status page says when it is not possible.
- Submission requires TLS before AUTH; plaintext AUTH is never offered.

#### 2.11 DMARC and outbound abuse controls

- SPF, DKIM and DMARC published per domain; DMARC starts at `p=none`
  with reports to a team mailbox, and the status page suggests
  `quarantine` after `APP_MAIL_DMARC_PROMOTE_DAYS` of clean reports.
- Per account send caps at creation from `APP_MAIL_MAX_OUT_PER_DAY` and
  `APP_MAIL_MAX_FIRST_TIME_RCPT_PER_DAY`, written to
  `MaxOutgoingMessagesPerDay` and `MaxFirstTimeRecipientsPerDay`
  (VERIFIED settings, mox defaults 1000 and 200).
- Queue hold and release replace `DropQueue` in the `Engine` interface
  (`HoldQueue`, `ReleaseQueue`, both by filter). Drop stays behind root
  plus step-up.
- Outbound kill switch: holds the whole queue when the outbound rate
  passes `APP_MAIL_OUTBOUND_KILL_PER_HOUR` or a blocklist hit appears;
  manual release with step-up. The default threshold is lowered to 200
  per hour for a new service, since a fresh IP should not send at 1000.
- No open relay test runs after install and after every upgrade.

#### 2.12 Encryption at rest: what we do and do not do

- **We do**: seal every credential with the master key; encrypt every
  backup, archive and export with age before it leaves the node;
  encrypt in transit everywhere.
- **We do not**: encrypt the live mail store. mox writes messages and
  index databases as plain files (encryption at rest is on its roadmap,
  VERIFIED). Anyone with root on the mail node, or the disk, can read
  mail. We advise an encrypted disk and say we cannot verify it.

### 3. Mail management: separate port or dashboard

**Decision: the existing dashboard, Levelrail IAM and authengine.** No
separate management listener or UI.

Why: one sign-in with passkeys, TOTP and SSO already hardened; one IAM
evaluator with deny wins; one audit trail; step-up already exists; no
second admin password for operators to protect. A separate admin UI
(mox's) would add an internet facing surface with a single shared admin
password and no 2FA.

What stays separate on purpose: the **mailbox user** surface. Webmail and
account settings live on their own hostname and listener, because
mailbox users are not Levelrail users.

### 4. Where mail data lives

mox stores everything on local disk: config, accounts, messages, index
databases, queue (VERIFIED). It cannot use S3 or R2 (VERIFIED by
absence). Stalwart can put message blobs in S3 compatible storage,
including R2, with a key prefix (VERIFIED); its metadata store is
separate (RocksDB, Postgres and others).

**Design**:

1. **Live store**: a named volume on the mail node, with a size cap
   (`APP_MAIL_VOLUME_MAX_GB`, enforced through the per mailbox quotas
   summed against the cap, warned at `APP_MAIL_DISK_WARN_PCT`).
2. **Backups**: `mox backup` into a staging dir on the same volume, then
   the agent streams it as tar, zstd, age to the mail storage connection.
   Staging needs free space about equal to the changed data; message
   files are hard linked (VERIFIED), so the extra space is mostly the
   databases. Then `mox verifydata` on a restored copy in the drill.
3. **Archive old mail**: per team policy, messages older than
   `APP_MAIL_ARCHIVE_AFTER_DAYS` in chosen mailboxes are exported to an
   encrypted mbox per month in the bucket and then deleted from the live
   store, or simply deleted (policy `archive|delete|keep`, default
   `keep`). Requires the mox API to delete by age; UNVERIFIED, may need
   IMAP from the node with an admin delegated login. Not in the first
   slice.
4. **Export**: per mailbox through the account export (live instance) or
   `mox export` on a backup copy (VERIFIED that live `mox export` may
   block), sealed with the team's age recipient, downloadable once
   through a signed link with expiry `APP_MAIL_EXPORT_LINK_MINUTES`.

**Honest tradeoffs**: the mail node is stateful; losing it without a
recent backup loses mail received since that backup. Recovery point is
the backup interval (`APP_MAIL_BACKUP_SCHEDULE`, default every 6 hours;
each run is a full `mox backup`, since mox offers no incremental mode
that we found). Disk size bounds the service.

**Stateless node option**: would need Stalwart with S3 blobs plus an
external metadata store (Postgres), which brings the AGPL, memory and
maturity costs ADR 058 recorded. What we say now: "mail lives on the
mail node; backups and archives live in your bucket". We do not promise
a stateless mail node.

### 5. Bring your own bucket

A **mail storage connection** is a new resource owned by one mail
service. It is not a backup target and never appears in
`/api/v1/backup-targets`, storage destination lists, protect-all or any
other team's pickers.

- **Record**: `mail_storage_connections` (id, service id, team id,
  provider, endpoint, region, bucket, prefix, age recipient, retention
  days, created by). Secrets under `mail-storage/<service-id>` in
  `internal/secrets`, write-only, never echoed, never resolvable by
  other code paths (the resolver checks the service id matches).
- **IAM**: reading or using it needs an ability on `mail:service:<id>`;
  team A cannot list, select or read team B's connection. The route
  refuses a service id whose team the caller cannot see, with the same
  404 as a missing one.
- **Layout**: `<service-id>/backups/<rfc3339>.tar.zst.age`,
  `<service-id>/archive/<mailbox-hash>/<yyyy-mm>.mbox.zst.age`,
  `<service-id>/exports/<id>.zip.age`, `<service-id>/.probe/`. Mailbox
  addresses are hashed in keys so object listings do not leak addresses.
- **Least privilege, AWS S3** (prefix only):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:DeleteObject"],
      "Resource": "arn:aws:s3:::BUCKET/SERVICE_ID/*"
    },
    {
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::BUCKET",
      "Condition": { "StringLike": { "s3:prefix": ["SERVICE_ID/*"] } }
    }
  ]
}
```

- **Least privilege, Cloudflare R2**: an R2 API token with "Object Read
  and Write" scoped to the one bucket. R2 tokens scope to buckets, not to
  prefixes (UNVERIFIED that no prefix scoping exists today), so we
  recommend one bucket per mail service on R2.
- **Validation on connect**: the existing storage probe (put, get,
  list, delete under `<service-id>/.probe/`) with its stable failure
  reasons, plus a check that a write outside the prefix is refused when
  the operator ticked "prefix scoped key" (a warning, not a block, if it
  is allowed).
- **Create the bucket automatically** only when the supplied key can:
  on S3 a `CreateBucket` with the key, falling back to "bucket must
  exist" on `AccessDenied`; on R2 bucket creation needs an account level
  token or the Cloudflare API, which a bucket scoped token cannot do, so
  the form asks for an existing bucket on R2. The form never asks for
  account wide credentials just to create a bucket.
- **Client-side encryption**: age on the mail node by the agent, to the
  team's mail recipient key. The private key is shown once at setup and
  must be stored off the instance, the same rule as volume backups. The
  bucket never holds plaintext.
- **Lifecycle**: retention by count and days
  (`APP_MAIL_BACKUP_RETAIN`, `APP_MAIL_BACKUP_RETAIN_DAYS`) pruned by the
  control plane; the dashboard suggests a bucket lifecycle rule as a
  second line, not the only one.
- **Uninstall**: default keeps the bucket data. Options: keep, delete
  after a retention period (`APP_MAIL_UNINSTALL_RETENTION_DAYS`, default
  30) or delete now with step-up and a typed confirm of the service name.
  Volume default on uninstall: keep.
- **Local folder fallback**: when no bucket is connected, backups go to a
  node directory capped by `APP_MAIL_LOCAL_BACKUP_MAX_GB`, pruned oldest
  first, keeping at least the newest backup, with a persistent warning
  that a backup on the same machine is lost with it. "Export as backup"
  downloads the newest sealed backup to the operator's machine.

### 6. Dashboard Mail area

Compact, horizontal cards, forms prefilled from the team, the domain
and env defaults. Every action also has a CLI command and an API route.

Tabs: **Status**, **Domains**, **Mailboxes**, **Aliases**, **Queue**,
**Storage**, **Security**, **Activity**.

- **Status checks** (Mail-in-a-Box style, one list, re-run button, alert
  on change): engine up, version and advisory status, certificate expiry
  served versus on disk, ports 25 inbound and outbound, PTR forward
  confirmed, blocklist check, no open relay test, disk use, last backup
  age, last restore drill.
- **Domains**: DNS status per record from the mail DNS engine (MX, SPF,
  DKIM, DMARC, MTA-STS, TLS-RPT, autoconfig, TLSA when possible), "plan"
  and "apply" with the diff.
- **Mailboxes**: address, linked member, quota and usage bar, state,
  last login (when mox exposes it, UNVERIFIED), reset password, suspend.
- **Aliases**: alias, targets, who may send.
- **Queue**: counts by state, rows without content, hold, release, kill
  switch state.
- **Storage and backup**: connection, last backup time and size, next
  run, retention, restore (to a new volume first), export.
- **Security**: TLS versions seen, auth failures per hour, lockouts,
  break-glass sessions.
- **Activity**: audit rows for this service.

Main screen:

```
+-----------------------------------------------------------------------+
| Mail  [team: Payments v]  mail-1 on node fra-2   v0.0.17  [Healthy]    |
+-----------------------------------------------------------------------+
| Status | Domains | Mailboxes | Aliases | Queue | Storage | Security | Log|
+-----------------------------------------------------------------------+
| [OK] Engine  [OK] TLS 61d  [WARN] PTR  [OK] No relay  [OK] Backup 2h   |
| [OK] Port 25 in  [FAIL] Port 25 out -> relay mode on  [OK] Not listed  |
+--------------------+--------------------+-----------------------------+
| Domains   3        | Mailboxes 12       | Queue  4 queued, 0 held     |
| 3 ready, 0 issues  | 4.1 / 20 GB used   | 312 sent today, cap 1000    |
+--------------------+--------------------+-----------------------------+
| Mailboxes                                       [+ Add mailbox]        |
| ana@example.com   Ana R.   [======    ] 1.2/5 GB  active   ...         |
| ops@example.com   (shared) [==        ] 0.4/2 GB  active   ...         |
| raj@example.com   Raj K.   [=         ] 0.1/2 GB  suspended ...        |
+-----------------------------------------------------------------------+
```

Storage panel:

```
+-----------------------------------------------------------------------+
| Storage and backup                                                     |
+-----------------------------------+-----------------------------------+
| Bucket  r2: mail-payments          | Last backup  2h ago, 3.8 GB, OK   |
| Prefix  svc_7f3a/   Key: bucket    | Next run     in 4h (every 6h)     |
| Encryption  age, key fingerprint   | Retain  7 backups / 30 days       |
| [Test connection] [Change]         | Last restore drill  5d ago, pass  |
+-----------------------------------+-----------------------------------+
| Live volume  4.1 / 20 GB [====      ]  warn at 80%                     |
| Archive  off   [Set policy: archive mail older than ___ days]          |
| [Back up now] [Restore to new volume] [Export mailbox v] [Download]    |
+-----------------------------------------------------------------------+
```

### 7. Performance and footprint

- **Zero when off**: the mail package registers no routes, starts no
  goroutines and reads no table unless `APP_EXPERIMENTAL=mail` is set
  and at least one mail service exists. The watcher starts on the first
  install and stops on the last uninstall. A test asserts the goroutine
  count and SQL calls at startup with the flag off.
- **Mail node idle RAM**: target under 40 MiB for mox at idle with a few
  mailboxes (measured 14 to 36 MiB on arm64); budget limit 256 MiB.
- **Polls from env**: health `APP_MAIL_HEALTH_INTERVAL` (default 60s),
  status checks `APP_MAIL_CHECK_INTERVAL` (default 1h), blocklist daily.
- **Metrics**: the node agent scrapes mox's `/metrics` on the admin
  listener into its local store, queried through the existing federated
  telemetry path. No central push. Whether the agent can scrape a
  container Prometheus endpoint today: UNVERIFIED.
- **No mail content in SQLite**; only service, domain and mailbox
  records, counters and audit rows.
- **Backups stream** from staging through tar, zstd, age to the bucket
  with bounded buffers; no full copy in memory or a second temp file.
- **Measure and publish**: `docs/benchmarks.md` gets a mail row (idle
  RAM, RAM after 1000 submissions, backup time and peak RAM per GB, on a
  real 1 vCPU node), re-run per pinned mox bump.

## Public security statement (for the docs)

> **How secure is mail on Levelrail?**
>
> Protected: mail between servers and clients is encrypted in transit
> (TLS required for client sign in and submission, MTA-STS and TLS
> reports published, DANE where your DNS supports it). The mail engine's
> admin interface is never on the internet; only the control plane
> reaches it over the private mesh, and every change goes through
> Levelrail sign in, permissions, step-up for destructive actions and the
> audit log. Credentials are sealed with the control plane master key.
> Backups, archives and exports are encrypted on the mail node with your
> key before they reach your bucket, so the bucket holds no readable
> mail. Each team's mail domains, mailboxes and bucket are invisible to
> other teams on the same instance. The control plane never reads
> message content.
>
> Not protected: mail stored on the mail node is not encrypted by the
> mail engine. Anyone with root on that machine, or its disk, can read
> it. Use an encrypted disk. Mailbox users sign in with a password
> (no two-factor in the mail engine today). We cannot guarantee inbox
> placement, and a stolen mailbox password can send mail until the
> lockout or send cap stops it. The mail engine is third party software
> pinned to a reviewed version; we alert on known advisories but do not
> audit its code.

## Alternatives rejected

- **mox's own admin UI as the management surface.** One shared admin
  password, no 2FA, a second audit trail. Kept only as audited
  break-glass.
- **A separate Levelrail mail admin port.** Duplicates sign in, IAM and
  audit for no gain.
- **Mailboxes as Levelrail users.** Gives every mailbox holder a
  dashboard account and ties mail sign in to the control plane being up.
- **Reusing instance backup targets for mail.** They are instance
  global; any team with backup access could select or read another
  team's mail backups.
- **Stalwart now for a stateless node.** Memory, AGPL and a 1.0 only
  days old; reconsidered when ADR 058 phase 6 ships.
- **Encrypting the live store ourselves (a FUSE or loop layer).** Adds a
  moving part under a database we do not own; disk encryption at the host
  is the standard answer.

## Risks

- mox is pre 1.0 with no downgrade; a bad upgrade means restore.
- A pre 1.0 internet facing parser; dedicated node and hardening limit
  the blast radius, they do not remove it.
- The lockout watcher depends on metrics or log shape; an upstream change
  silently disables it. The contract test must cover it.
- One password per mox account makes "per device" credentials weaker
  than users expect; copy must be honest.
- Deliverability and support load (ADR 058) remain the biggest product
  risk.

## Open questions for the owner

1. Accept "dedicated node by default, control plane node only with an
   explicit confirm", or block the control plane node entirely?
2. Default offboarding rule: suspend (proposed) or transfer to the team
   owner?
3. Offer webmail forward auth through Levelrail sign in in v1, or later?
4. Backup default every 6 hours: acceptable recovery point?
5. On R2, require one bucket per mail service, or allow a shared bucket
   with a prefix and a warning?
6. Kill switch default 200 per hour for a new service: acceptable?
7. Does ADR 058 Q1 (in scope before 1k stars) still gate all of this?

## First slice (under 2 days, after the template hardening branch)

- Catalog filter for `mail-mox` behind `APP_EXPERIMENTAL=mail`.
- Template v2: three listeners (public, webmail on mesh, admin on mesh),
  admin password as a file, `cap_drop`, `no-new-privileges`, read-only
  root, limits from env.
- `Engine`: replace `DropQueue` with `HoldQueue` and `ReleaseQueue`; set
  send caps at account creation from env.
- `mail_domains.team_id` and `mail_mailboxes` with team ownership; IAM
  resource prefixes `mail:service:`, `mail:domain:`, `mail:mailbox:` in
  the evaluator and the resource picker.
- The DNS plan route moves to `write`.
- One focused test: a team B token gets 404 on team A's domain and
  mailbox routes.

Next slices: mail storage connection with probe and age backups; status
checks page; lockout watcher; offboarding hook; archive policy.

## UNVERIFIED claims to prove on a real host

1. The hardened container (read-only root, caps dropped, uid 1000, admin
   password file) starts and serves with the pinned mox image.
2. The agent can restrict the admin listener to the control plane peer
   on the mesh.
3. mox has or lacks built in auth failure throttling; the lockout
   watcher works through `LoginDisabled` without a restart.
4. Several credentials per mox account in any release.
5. Delete by age through the mox API, for the archive policy.
6. The agent can scrape a container's Prometheus endpoint today.
7. `mox backup` staging space and duration per GB on a real disk, and a
   restore drill with `verifydata` on the copy.
8. Last login time is exposed by mox for the mailboxes list.
9. R2 tokens cannot be scoped to a prefix.
10. Inbound TLSA publishing through each supported DNS provider.
11. One time secret links through the existing email path.
12. Mail-in-a-Box uses duplicity; Mailu admin TOTP and token support.
13. Certificate renewal restart in a quiet window keeps mail flowing
    (senders retry), with no lost inbound mail.

## Decisions

The open questions are settled as follows. Each can be revisited with a new ADR.

1. Mail runs on a dedicated node by default. Using the control plane node needs an explicit confirmation and is never the default.
2. Offboarding a team member suspends their mailbox. Transfer or forward are explicit options.
3. Webmail forward auth is not in the first release.
4. A backup recovery point of up to 6 hours is acceptable for the first release.
5. On R2, one bucket per mail service is required. A shared bucket with a prefix is allowed on S3 compatible stores with a warning.
6. A new mail service starts with a send cap of 200 messages per hour per account, read from `APP_MAIL_DEFAULT_SEND_CAP_PER_HOUR`.
7. Mail stays behind the experimental `mail` flag until the real host run in ADR 060 is recorded.
