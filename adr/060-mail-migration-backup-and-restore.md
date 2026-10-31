# ADR 060: Mail migration, backup and restore

Partly superseded by [ADR 064](064-email-for-apps.md): new mail work follows ADR 064.

Status: Proposed

Date: 2026-10-29

Builds on: ADR 058 (optional in-house mail server, mox by default).

## Context

The owner's question, in their words: "lets say my Levelrail app is on VPS 1
and need to migrate to VPS 2, how will that look, considering multi-user,
where does the mail data go, backup, how does that work".

That question hides three different moves, and they must not be mixed up:

| | What moves | What stays | Downtime for mail |
| --- | --- | --- | --- |
| **A. Mail service to another node** | The mox container and its two volumes | The control plane, every other app | Minutes (fence to first accept on the target) |
| **B. Whole instance, VPS 1 to VPS 2** | The control plane database, master key, agent CA, and (if the mail node is VPS 1) also case A | Remote nodes stay where they are | None if mail runs on another node; case A's window if it runs on VPS 1 |
| **C. One mailbox or one user** | The messages of one account | The mail service and every other mailbox | None for anyone else |

And under all three sits **D, the backup model**: a migration is a restore
of the latest consistent backup onto a new place plus a short final delta.
If backups are not trustworthy, no migration is.

Every claim below is marked **VERIFIED** (with its source: code on
`origin/release/v0.8.12`, upstream docs, an RFC, or our own measurement) or
**UNVERIFIED**. The public product here serves teams and organisations on
one instance; nothing in this ADR assumes more than that.

### What exists today (VERIFIED by reading the release branch)

1. **mox runs as a catalog app**, `mail-mox` in
   `internal/catalog/templates_mail.go`, with two named volumes:
   `mail_config` at `/mox/config` and `mail_data` at `/mox/data`. The doc
   `docs/mail-template.md` says `domains.conf` belongs to the control plane
   and is never rewritten by the template, and the admin password is the
   generated sealed secret `SERVICE_PASSWORD_MAILADMIN`.
2. **There are no mail service, domain or mailbox tables yet.**
   `internal/store/mail.go` holds outbound mail templates, audiences and
   broadcasts, not mailboxes. Domains, accounts, aliases, DKIM keys and
   passwords live inside mox (in `mail_config` and `mail_data`).
   `internal/mail` defines the engine contract and `internal/mail/mox`
   implements domains, accounts, addresses, aliases, DNS records, DKIM
   keys, queue stats, queue drop and health. `SetQuota` returns
   unsupported (mox has no quota API function; measured by us: the quota
   is set in `domains.conf`, picked up without restart).
3. **Node moves** (`internal/nodemove`) are persisted, resumable, one per
   resource, with phases `preflight`, `stop_source`, `copy_volumes`,
   `place`, `verify`, `stop_old`, `finalize`, `rollback`, `done`. Stateful
   moves are stop, copy, start. Drains cordon a node and move what is left.
4. **Volume backups** (`internal/backup`) stream tar, zstd, then age
   encryption on the control plane, so the bucket only holds ciphertext.
   `quiesce.go` runs a per volume `PreHook` and `PostHook` inside the app
   container. Retention is count plus age (`retain`, `retain_days`), only
   succeeded backups are pruned, pruning runs after scheduled backups.
   `restore-to` restores into a new volume, optionally on another node,
   and never writes to an existing volume. Local folder targets are
   confined to a root (`local_files.go`, `ErrLocalPathEscapes`).
5. **Control plane backup** (`internal/cpbackup`): `VACUUM INTO` snapshots,
   integrity check, SHA-256, encrypted off-box copies, an escrow bundle
   holding the master key and the agent CA, an optional continuous replica,
   restore with manifest, schema and install id checks
   (`ForceInstallID` to accept another install). The master key is never
   in a backup. `alerting.db` (alert rules, channels, silences, maintenance
   windows) is **not** in a control plane backup today
   (`docs/control-plane-backup.md`).
6. **Agents** dial out to `APP_CONTROL_PLANE_ADDR` (`cmd/levelrail-agent`)
   and trust the control plane CA from enrollment. Without the restored
   agent CA, every agent must re-enroll (`docs/disaster-recovery.md`).
7. **Mail DNS** (`internal/mailrecords`): add-only plans, a hash that pins
   the reviewed plan, `replace_existing` naming MX, SPF, DKIM, DMARC,
   MTASTS or TLSRPT, a refusal when the domain's MX points elsewhere, audit
   subjects without values. TTL comes from the spec or `APP_DNS_DEFAULT_TTL`.
8. **Step-up auth** (`internal/api/step_up.go`) grants cover one action on
   one resource. Abilities: `read`, `write`, `deploy`, `write:sensitive`,
   `root` (`internal/api/abilities.go`). Maintenance windows and silences
   live in `internal/alerting` (`maintenance.go`, `noise.go`).

### Facts about mox that this design leans on

| Fact | Status |
| --- | --- |
| `mox backup <dir>` copies config and data with "a consistent snapshot of the databases and message files"; message files are hardlinked when possible; it never overwrites, so the destination must be empty | VERIFIED, xmox.nl/commands |
| "Simply copying files in the data directory while mox is running can result in unusable database files" | VERIFIED, xmox.nl/commands |
| `mox verifydata` checks database files, that every message has a file, and that no unknown files exist; opening the databases "may apply schema upgrades", so run it on a copy | VERIFIED, xmox.nl/commands and the mox README |
| Restoring after a failed upgrade: run `verifydata` with the original binary before starting | VERIFIED, mox README and FAQ |
| After a restore, `mox bumpuidvalidity <account>` makes IMAP clients resynchronize; it needs mox stopped or the account closed | VERIFIED, mox README and xmox.nl/commands |
| `mox export maildir` and `export mbox` export one or all mailboxes of one account; they open the database directly and may block while mox has it open; for a running instance use the web interface | VERIFIED, xmox.nl/commands |
| `mox import maildir` and `import mbox` need the running mox, read files it can access (suggested `data/import/`), keep flags, and **import duplicates when run twice** | VERIFIED, xmox.nl/commands |
| `mox admin imapserve` serves a preauthenticated IMAP session on a file descriptor, "e.g. with SSH during migrations" | VERIFIED, xmox.nl/commands |
| Queue: `list`, `hold`, `fail`, `drop`, `schedule -now`, `dump`, `holdrules add` (hold every new submission when given no flags), `retired list` | VERIFIED, xmox.nl/commands |
| `mox setadminpassword` exists | VERIFIED, xmox.nl/commands |
| Upgrades: "put a new binary in place and restart"; read release notes of every version in between | VERIFIED, mox README |
| Downgrade: mox upgrades database schemas on start; upstream documents no downgrade path, only restore from backup | VERIFIED that no downgrade is documented; that an older binary refuses or corrupts newer files is UNVERIFIED |
| mox is pre 1.0 (v0.0.17 current) | VERIFIED, measured by us |
| A certificate change needs a container restart (no `mox config reload` in v0.0.17; HUP stopped the server) | VERIFIED, measured by us |
| Transports (relay) change needs a restart; routes do not | VERIFIED, measured by us |
| Admin API is a session login (`LoginPrep`, `Login`, then cookie and `x-mox-csrf`); `QueueSize`, `QueueList`, `QueueHoldSet`, `QueueDrop` callable | VERIFIED, measured by us |
| `mox backup` talks to the running server over its control socket (so it needs mox up) | UNVERIFIED |
| A plain file copy of a **stopped** mox data dir is consistent | UNVERIFIED (follows from bbolt semantics, not stated upstream) |

### Facts from the wider mail world

| Fact | Status |
| --- | --- |
| SMTP senders retry a failed destination, "retry interval SHOULD be at least 30 minutes", "give-up time generally needs to be at least 4-5 days" | VERIFIED, RFC 5321 section 4.5.4.1 |
| Senders order MX records by preference, "MUST be able to try (and retry) each" and "SHOULD try at least two addresses" | VERIFIED, RFC 5321 section 5.1 |
| Gmail requires a PTR for the sending IP that resolves to a hostname whose A or AAAA points back to the same IP, and recommends ramping volume up gradually from a new IP | VERIFIED, Google sender guidelines (support.google.com/a/answer/81126) |
| If UIDs are lost, clients clear their cache and re-download everything; some clients keep metadata per UID and lose it | VERIFIED, Dovecot migration docs |
| IMAP-to-IMAP copy tools (imapsync style) create new UIDs on the destination, and a changed UIDVALIDITY invalidates previous sync state | VERIFIED as common practice (vendor migration guides, RFC 3501 semantics); exact behaviour per tool UNVERIFIED |
| Common practice: lower MX TTL to about 300 s a day or two ahead, pre-sync, switch, then a final sync | VERIFIED as vendor practice, not a standard |
| Another self-hosted suite documents moving to a new server as a repeated rsync plus a consistent database copy to a cold standby | VERIFIED, mailcow cold standby docs |

## Decision (recommended)

1. **One mechanism for backup and migration.** Every mail service has a
   scheduled consistent backup (`mox backup` into a staging directory,
   then the existing tar, zstd, age stream to the service's backup target).
   A migration restores the latest verified backup on the target as a
   seed, then fences the source, takes one final consistent copy, ships
   only the delta, verifies, and switches DNS.
2. **Prefer a byte-level copy over IMAP copy.** Restoring mox's own data
   keeps UIDs, UIDVALIDITY, flags, junk training, DKIM keys, account
   password hashes, aliases and the outbound queue. IMAP-style copy is the
   fallback for moving to or from a different engine.
3. **Never let two copies of one store be live.** The source is fenced
   (public ports unpublished) before the final copy, and is never
   restarted with its old data after the target accepted mail. Rollback is
   a reverse migration, never "start the old one again". This is the
   rule that prevents both loss and duplicates.
4. **No downgrade.** The target engine version must be equal or newer than
   the source. Restoring onto an older mox is refused.
5. **DNS is changed by the existing mail DNS engine** with its add-only
   rules, plus one new record kind for the mail host address and an
   explicit replace flag per kind.
6. **One migration per mail service, root plus step-up, audited,
   announced to mailbox owners, inside a maintenance window.**

## D. The backup model

### Layout

- Live store on the node's local disk: `mail_config` and `mail_data`.
  Network filesystems are not supported for the live store (bbolt and
  file locks; UNVERIFIED on NFS, assumed unsafe).
- Staging: `/mox/data/tmp/backup/<stamp>` inside `mail_data`, so mox can
  hardlink message files (VERIFIED as upstream advice). Staging costs
  roughly the size of the databases, not the mail.

### Scheduled run

1. `PreHook`: `mox backup /mox/data/tmp/backup/<stamp>` (empty dir each
   time; mox refuses to overwrite).
2. `mox verifydata /mox/data/tmp/backup/<stamp>/data` with the same image.
   A failure marks the backup failed and alerts; nothing is uploaded.
3. Stream the staging dir (not the live dirs) through the existing volume
   archive path: tar, zstd, age, to the service's backup target.
   Hardlinks are archived as regular file content, so restore does not
   depend on hardlinks. UNVERIFIED: that the volume backup can archive a
   sub path of a volume; if not, slice 1 adds a `path` to the policy.
4. `PostHook`: delete the staging dir.
5. Manifest row: engine version (from `mox version`), image digest,
   account count, message count, byte size, checksum, verify result.

### Destinations, cap, retention

- **The service's own bucket** (a backup target with sealed credentials,
  existing resource) or **a local folder** under the confined local root.
- Local folders carry a size cap, `APP_MAIL_BACKUP_LOCAL_CAP_MB`. When
  the next backup would pass it, oldest succeeded backups are pruned first;
  if the newest alone does not fit, the backup fails loudly instead of
  deleting the only good copy.
- Retention reuses `retain` and `retain_days`, defaults
  `APP_MAIL_BACKUP_RETAIN` (7) and `APP_MAIL_BACKUP_RETAIN_DAYS` (30).
  The newest verified backup is never pruned.
- Schedule defaults to daily (`APP_MAIL_BACKUP_CRON`).

### Restore drills

A scheduled drill (`drill_runner.go` pattern) restores the newest backup
into a scratch volume, starts the pinned image with no public ports, runs
`verifydata`, logs in over the admin API and compares account and message
counts with the manifest. Result shows on the service card. A failed or
missing drill older than `APP_MAIL_DRILL_MAX_AGE` (14d) raises an alert.

### How backups relate to migration

The seed for every migration (A, B, and the disaster path) is the newest
**verified** backup. The final delta is: new message files (mox message
files are written once and not changed; UNVERIFIED that mox never
rewrites one in place) plus the full database files from the final
consistent copy. The delta is small next to the mail itself.

## A. Moving the mail service to another node of the same instance

Example: mox on node `n1` (IP 203.0.113.10), target `n2` (198.51.100.20),
mail host `mail.example.com`. MX of every domain points at
`mail.example.com`, so **the MX records do not change; only the address of
the mail host does** (and SPF if it lists the IP instead of `mx`).

Steps:

1. **Plan** (`mail migrate plan --to n2`). Preflight, no changes:
   target online, not cordoned; disk free on `n2` at least source size
   times `1 + APP_MAIL_MIGRATION_DISK_HEADROOM_PCT/100` (default 20);
   outbound port 25 from `n2` (timed connect) or relay mode configured;
   PTR of the target IP forward confirmed to `mail.example.com` (blocks
   unless `--accept-ptr-warning`); target image version >= source; newest
   verified backup exists; DNS zone is managed or manual; current TTL of
   the mail host A/AAAA and MX read from the authoritative servers.
2. **Notice and window.** Plan proposes a cutover time no earlier than
   `now + current TTL + APP_MAIL_TTL_MARGIN` (default 1h), and at least
   `APP_MAIL_NOTICE_LEAD` (72h) for mailbox owner notices. A maintenance
   window and a silence for mail alerts on the service are created for the
   cutover slot.
3. **Lower TTL.** The DNS engine sets the mail host A/AAAA (and MX if it
   will change) to `APP_MAIL_CUTOVER_TTL` (300). The run waits until the
   old TTL has expired everywhere (computed, not guessed).
4. **Seed.** Restore the newest verified backup into new volumes on `n2`
   (`restore-to`, never writing an existing volume). Run `verifydata` with
   the target image. Not started, no ports.
5. **Fence** (downtime starts). Unpublish 25, 465, 587, 993 on `n1`. mox
   keeps running privately. Inbound senders get connection refused and
   retry (RFC 5321). `builtin_mail` fails over to its backup provider
   (ADR 058). Before fencing, `holdrules add` holds new submissions so the
   source queue only shrinks; the queue is not flushed or dropped.
6. **Final copy.** `mox backup` on `n1` into an empty staging dir,
   `verifydata`, ship only files missing on `n2` plus all database files.
   Stop mox on `n1`. Its volumes stay intact and untouched.
7. **Start target** on `n2` with the same config volume (same DKIM keys,
   same certificate files, same admin password hash), ports still private.
   Verify: admin login, account and message counts equal the final
   manifest, queue size equals the source's final queue size, IMAP login
   with a probe account, a test delivery from the probe to itself.
8. **Cutover.** Publish the ports on `n2`. DNS engine replaces the mail
   host A/AAAA (`replace_existing: [MAILHOST]`, plus SPF if needed), hash
   pinned. Remove the hold rule; the moved queue now delivers from `n2`.
9. **Observe.** Rollback window `APP_MAIL_ROLLBACK_WINDOW` (72h). Checks
   run on a timer (see the checklist). The source volumes are kept.
10. **Finalize.** After the window, or by operator command, delete the
    source container; source volumes are deleted only after one more
    successful backup and drill on `n2`. Restore TTL to the default.

**Rollback** before step 8: unfence `n1` (publish its ports, remove the
hold rule). Nothing diverged because `n2` never accepted public mail.
After step 8: reverse migration (fence `n2`, final copy from `n2`, ship
delta to `n1`, switch DNS back). Starting `n1` on its old data is refused
by the runner: it would lose mail received on `n2` and resend queued mail.

**Recommendation:** implement A as a new mail-specific executor behind
the `nodemove` runner, not as a plain stateful move. A plain move would
copy a live store and has no DNS step, no fence and no queue rule.

## B. Moving the whole instance from VPS 1 to VPS 2

### Where each part of the mail feature lives

| In the control plane database (moves with a cp backup) | On the mail node (moves only with case A or a volume restore) |
| --- | --- |
| The `mail-mox` app record: compose, env, inputs, node placement, published port binds | `mail_config`: `mox.conf`, `domains.conf` (domains, accounts, addresses, aliases, routes, quotas), DKIM private keys, host keys, admin password hash, certificate files |
| Sealed secrets: `SERVICE_PASSWORD_MAILADMIN`, relay password, `builtin_mail` credentials, per-app SMTP credentials | `mail_data`: per account databases (UIDs, flags, junk training), message files, `queue.db`, DMARC and TLS reports |
| Backup targets and storage destinations with sealed credentials, volume backup policies and history | Mailbox password hashes (Levelrail never stores mailbox passwords, ADR 058) |
| DNS provider credentials, mail DNS audit rows | |
| Proposed by this ADR: `mail_services`, `mail_domains`, a mailbox metadata mirror (address, owner, quota, created at; never passwords or content), `mail_migrations`, `mail_exports` | |
| **Not in a cp backup today:** `alerting.db` (maintenance windows, silences, mail alerts) and the master key | |

The control plane database never contains mail content or DKIM private
keys. A cp restore alone gives back the knowledge of the mail service,
not the mail.

### Flow when mail runs on another node (VPS 3)

1. Take a cp backup and confirm escrow (`master.key`, agent CA) is
   current. Copy `alerting.db` with the control plane stopped (today it is
   outside the backup; slice 4 adds it).
2. Install the same or newer release on VPS 2. Recover master key and
   agent CA from escrow into the data dir. Restore with `--dry-run`, then
   for real.
3. Point agents at the new address. If `APP_CONTROL_PLANE_ADDR` is a DNS
   name, change that record (lowered TTL ahead, same rule as mail). If it
   is an IP, each agent's env is edited. With the restored agent CA,
   agents reconnect without re-enrolling.
4. Mail is never down: mox on VPS 3 keeps serving. Only management calls
   fail while the control plane is away. Verify: admin login from the new
   control plane works, which proves `SERVICE_PASSWORD_MAILADMIN`
   decrypted with the recovered master key.

### Master key handling

Sealed mail secrets open only with the same master key. If the key is
lost: the admin password can be reset with `mox setadminpassword` inside
the container and resealed; relay and provider passwords must be entered
again. Mailbox passwords are unaffected (they live in mox). The UI shows
"secret unreadable" per item instead of failing the whole service.

### Flow when the mail node is VPS 1 itself (the common single box case)

Recommended order, **move mail first, the control plane last**:

1. Enroll VPS 2 as a node of the VPS 1 instance.
2. Run case A: mail service from VPS 1 to VPS 2 (fence, final copy, DNS
   cutover, observe). Move other apps with existing node moves or a drain.
3. Run the cp move above onto VPS 2, where the containers already run.
   The DR runbook verified that running containers on the restore machine
   are left alone.
4. Retire VPS 1 after the rollback window.

Why: each step has its own rollback and only one of them touches mail.
UNVERIFIED and a real risk: the restored database still names VPS 1 as
the control plane's local node and VPS 2 as a remote node; how the
restored control plane reconciles its local node identity on a different
machine is not proven (listed below).

Alternative, one downtime: restore cp on VPS 2, then restore mail volumes
from the newest mail backup plus a final delta copied over SSH from a
fenced VPS 1. Faster, but mail and control plane are down together and a
failure leaves neither side clean. Not the default.

## C. Per mailbox or per user export and import

### Export

- **Source**: never the live store. Export runs on a fresh consistent copy
  (on-demand `mox backup` into staging), then `mox export maildir` against
  that copy. This avoids the documented block on an open database and
  gives a point in time.
- **Format**: maildir by default (upstream: "Maildir is a better format"),
  mbox on request, packed as tar.zst, age-encrypted to a recipient key or
  a one-time passphrase shown once. A JSON manifest beside it lists
  addresses, aliases the account belongs to, mailbox names, counts and a
  checksum. Aliases and passwords are listed as facts, never exported as
  config.
- **Delivery**: a download link valid `APP_MAIL_EXPORT_LINK_TTL` (24h),
  single use, size limit `APP_MAIL_EXPORT_MAX_BYTES`. The staging copy is
  deleted when the link is used or expires.
- **Self service**: a mailbox owner can export their own mail from mox's
  account web page (VERIFIED as the upstream way for a running instance).
  The admin flow is for offboarding and requests.

### Import

- Into mox: the archive is placed in `data/import/` and imported by the
  running mox (VERIFIED requirement). Imports are **not idempotent**
  upstream, so Levelrail records an import id and checksum per account and
  refuses a second run of the same archive unless `--allow-duplicates`.
- Quota: the archive size is checked against the account quota first; the
  import is refused if it would pass it (`--raise-quota` to adjust in the
  same audited action).
- From another engine or provider: an IMAP copy (imapsync style) is the
  fallback. It creates new UIDs, so the user's clients re-download that
  mailbox. Documented, not automated in v1.

### The cases

| Case | Flow |
| --- | --- |
| Offboarding a team member | Export to the manager's key, disable the account, alias the address to a successor, delete after `APP_MAIL_OFFBOARD_RETAIN_DAYS` (30) |
| Move one mailbox to another mail service on the same instance | Export plus import, then the address moves (remove on source, add on target). New password shown once on the target; old app passwords do not carry over |
| Move one mailbox to an external provider while the domain stays | Export, or IMAP copy run by the user, then the address on mox becomes an alias forwarding to the external address. Split delivery of one domain is not offered |
| Move a whole domain to an external provider | Export every account, then MX replaced with `replace_existing: [MX]`, the existing foreign MX guard in reverse |
| Access request (GDPR style) | Export of that person's account, delivered to them, audited |
| Erasure request | Account removed; copies in backups expire by retention, and the request record states the date the last backup holding it is pruned |

Passwords and app passwords never move in a per-mailbox move. They do
move in A and B, because the account databases move whole.

## Safety and cutover engineering

### DNS TTL

The DNS engine reads the live TTL of the mail host A/AAAA and MX from the
authoritative servers, lowers it to `APP_MAIL_CUTOVER_TTL`, and computes
the earliest safe cutover as `lowered_at + old_ttl + APP_MAIL_TTL_MARGIN`.
For manual DNS zones, the plan prints the records and the run waits until
`dns check` sees the lowered TTL. Resolvers that ignore TTL exist; the
fence plus sender retries covers them.

### Old server during the window

Options considered:

1. **Fence, rely on retries (recommended).** Old address refuses on 25.
   Senders retry for days (RFC 5321) and re-resolve. No duplicates, one
   store. Delay is bounded by TTL plus one retry interval.
2. **Old host as a TCP forwarder to the new one.** Zero refusals, but mox
   sees the forwarder's address as the client, which breaks SPF checks
   and IP reputation on inbound unless mox supports the PROXY protocol
   (UNVERIFIED, assumed not). Opt-in only.
3. **Old host as a store-and-forward secondary MX.** A second queue to
   drain, spam checks on the wrong box, and secondary MX is a known spam
   path. Rejected.

### Queue: no loss, no duplicate

- The outbound queue lives in `queue.db` and moves with the final copy.
- Before the fence, a hold rule holds new submissions; existing entries
  keep delivering until fenced. After the final copy the source never
  runs again on that data, so every queued message has exactly one owner.
- No flush and no drop as part of a migration. `QueueDrop` is not called.
- Target verifies its queue size equals the source's final queue size
  before cutover, then removes the hold.
- Duplicates are possible only if a delivery was in flight at the stop
  (the remote accepted, the source did not record it). Receivers dedupe by
  Message-ID in most clients; we accept and document this edge.

### IMAP clients

- A byte-level move keeps UIDs and UIDVALIDITY. Clients see a dropped
  connection, reconnect after DNS, and do not re-download. Do **not** run
  `bumpuidvalidity` on a forward move.
- After a rollback that restores a state **older** than what clients saw,
  run `bumpuidvalidity` per affected account so clients resync instead of
  trusting stale UIDs.
- IMAP copy fallback (C, or engine changes): new UIDs, full re-download,
  per-UID client metadata lost (VERIFIED, Dovecot docs). The UI says so.

### PTR and IP reputation

- New node, new IP, no reputation. The plan blocks without a forward
  confirmed PTR on the target IP (Gmail requirement, VERIFIED) unless the
  operator accepts the warning.
- The target IP is checked against public blocklists before cutover.
- Warm-up: for `APP_MAIL_WARMUP_DAYS` (14) after an IP change, outbound
  system mail goes through relay mode if one is configured, and the
  per-account outbound limits are lowered, ramping up as Google advises.
- The old IP keeps its reputation; that is lost value, stated in the plan.

### Certificates

- The certificate files live in `mail_config` and move with it, so the
  same name and key are served on day one. A restart is needed for any
  certificate change (measured).
- Renewal must not depend on the mail host's A record during the window:
  DNS-01 is required for the mail host while a migration is open.
- If DANE TLSA records exist, the key must not change during a migration.

### DKIM continuity

DKIM private keys are in `mail_config`; selectors and DNS TXT records do
not change, so signatures made before, during and after the move verify.
The plan diffs `DNSRecords` of source and target and blocks on any DKIM
difference.

### DNS apply rules for the cutover

Add-only by default, hash pinned, audited without values, exactly as
today. New record kind `MAILHOST` (A/AAAA of the mail host) joins
`replace_existing`. The cutover plan names only the kinds it changes
(`MAILHOST`, and `SPF` when the SPF lists an IP). An MX change happens
only in a domain-level move and must name `MX`.

### Versions

The target image must report a mox version equal or newer than the source
(`mox version` in both before the seed). A newer target runs `verifydata`
on the seed with its own binary (may upgrade schemas, on the copy only).
Restoring a backup onto an older mox is refused, both for migrations and
for plain restores. Rollback after an upgrade-and-move reuses the newer
version on the old node.

### Verification checklist after cutover

1. `dns check` passes for every domain, mail host resolves to the new IP.
2. PTR of the new IP forward confirmed.
3. TLS on 465 and 993 serves the expected certificate.
4. Inbound probe from an outside address lands (owner's test domain).
5. Outbound probe passes SPF, DKIM, DMARC at a receiver.
6. Queue drains, no hold left, `mox_queue_hold` metric zero.
7. IMAP login for the probe account, UIDVALIDITY unchanged.
8. Account and message counts match the final manifest.
9. First backup and drill on the new node succeed.

### Rollback time limit

Rollback is offered for `APP_MAIL_ROLLBACK_WINDOW` (72h). Inside it, the
source volumes are kept and the reverse migration is one command. After it,
the migration finalizes and the old volumes follow the deletion rule.
Fenced downtime has its own limit, `APP_MAIL_FENCE_MAX` (2h): if the target
is not verified by then, the run aborts and unfences the source.

### Dry run

`plan` is always a dry run. `run --dry-run` also restores the seed into
scratch volumes, runs `verifydata`, starts the target privately, compares
counts, and deletes the scratch. No fence, no DNS write.

## State machine

```
            +---------+
            | planned |
            +----+----+
                 | run
            +----v------+   fail    +---------+
            | preflight +---------->| failed  |
            +----+------+           +---------+
                 |
            +----v-----+
            | ttl_wait |  (waits for old TTL + margin)
            +----+-----+
                 |
            +----v----+
            | seeding |  (restore newest verified backup on target)
            +----+----+
                 |                    abort here: delete seed, done
            +----v----+
            | seeded  |
            +----+----+
                 | window opens
            +----v----+
            | fenced  |  (source ports closed, hold rule set)
            +----+----+
                 |                    abort or FENCE_MAX: unfence source
            +----v-------+
            | final_copy |
            +----+-------+
                 |
            +----v------+
            | verifying |  (target private, counts, queue size)
            +----+------+
                 |   ---- point of no simple abort ----
            +----v-----+
            | cutover  |  (ports public on target, DNS replaced)
            +----+-----+
                 |
            +----v------+   rollback   +-------------+
            | observing +------------->| rolling_back|
            +----+------+   (<= 72h)   +------+------+
                 |                            |
            +----v------+              +------v------+
            | finalized |              | rolled_back |
            +-----------+              +-------------+
```

Every state is persisted before the next starts and resumes after a
control plane restart, as in `nodemove`.

## Surface

### CLI

```
levelrail-cli mail migrate plan   <service> --to <node> [--at <time>] [--accept-ptr-warning]
levelrail-cli mail migrate run    <service> --plan <hash> [--dry-run]
levelrail-cli mail migrate status <service> [--watch]
levelrail-cli mail migrate abort  <service>
levelrail-cli mail migrate rollback <service>
levelrail-cli mail backup  now|list|verify|drill <service>
levelrail-cli mail restore <service> --backup <id> [--to-node <node>] [--dry-run]
levelrail-cli mail export  <service> <address> [--format maildir|mbox] --recipient <age key>
levelrail-cli mail import  <service> <address> --file <archive> [--allow-duplicates] [--raise-quota]
```

### API (all behind `APP_EXPERIMENTAL=mail`)

| Method and path | Ability |
| --- | --- |
| `POST /api/v1/mail/services/{s}/migrations/plan` | `read` (dry run) |
| `POST /api/v1/mail/services/{s}/migrations` (body: plan hash) | `root` + step-up |
| `GET /api/v1/mail/services/{s}/migrations/{id}` | `read` |
| `POST /api/v1/mail/services/{s}/migrations/{id}/abort` | `root` |
| `POST /api/v1/mail/services/{s}/migrations/{id}/rollback` | `root` + step-up |
| `POST /api/v1/mail/services/{s}/backups`, `GET` | `write:sensitive`, `read` |
| `POST /api/v1/mail/services/{s}/restore` | `root` + step-up |
| `POST /api/v1/mail/services/{s}/accounts/{a}/export` | `root` + step-up |
| `POST /api/v1/mail/services/{s}/accounts/{a}/import` | `write:sensitive` + step-up |

`run` returns 409 when a migration, restore or import is open for that
service (one at a time, as `ErrNodeMoveInProgress`). Backups and engine
upgrades are paused while a migration is between `fenced` and `observing`.

### IAM resource names

`mail_service:<name>`, `mail_domain:<domain>`, `mailbox:<address>`,
`mail_migration:<id>`, `mail_backup:<id>`, `mail_export:<id>`.
Step-up scopes: `mail.migrate`, `mail.restore`, `mail.export` on one
`mail_service`, never wider.

### Multi-user rules

- Who: migrations, restores, rollbacks and exports of someone else's
  mailbox need `root` plus a fresh step-up. Imports need
  `write:sensitive` plus step-up. Plans are readable with `read`.
- Notices: mailbox owners get mail at plan time (lead
  `APP_MAIL_NOTICE_LEAD`), at fence, and at finalize, sent via the backup
  provider so it does not depend on the server being moved. An export of
  someone's mailbox notifies that person unless the operator records a
  legal hold reason (audited).
- Audit rows: plan, run, each state change, abort, rollback, export
  created and downloaded, import, with actor, service, node ids, counts
  and sizes. Never message content, passwords or keys.
- Maintenance windows: `run` creates a window and a silence scoped to the
  service for the fence-to-observe span, linked to the migration id and
  removed at finalize.
- Progress UI: a compact horizontal stepper on the service card, one chip
  per state (waiting, running, ok, failed) with elapsed time; Abort is
  enabled up to `verifying` and replaced by Rollback after `cutover`.

## Failure modes

| Failure | Behaviour |
| --- | --- |
| Newest backup fails `verifydata` | Plan blocked; operator triggers a new backup |
| Target disk too small | Preflight fails with the needed size |
| Target PTR missing | Plan blocked unless accepted, warning in checklist |
| Outbound 25 blocked on target | Plan requires relay mode |
| Target mox older than source | Plan refused (no downgrade) |
| TTL not yet expired | Run waits in `ttl_wait`, shows ETA |
| Final copy fails or `FENCE_MAX` reached | Abort, unfence source, no DNS change |
| Target counts or queue size differ | Stay fenced, block cutover, show diff |
| DNS apply fails mid cutover | Retry with the same hash; fall back to unfence source if the record was not changed |
| Control plane restarts mid run | Resume at the persisted state |
| Target node dies in `observing` | Reverse migration needs the target's final data; if the node is gone, restore newest target backup on the source (mail since that backup is lost; stated in the UI) |
| Master key lost on whole instance move | Reset admin password inside the container, re-enter relay secrets; mailboxes unaffected |
| Agent CA lost | Re-enroll every agent with a join token |
| Second import of the same archive | Refused unless `--allow-duplicates` |
| Export link leaked | Single use, short TTL, ciphertext only |

## What is NOT verified and must be proven on a real host

The owner's VPS and domain are the test bed. Each item is a pass or fail:

1. `mox backup` while serving real traffic, `verifydata` on it, restore on
   a second node, counts match.
2. Whether `mox backup` needs the running server, and whether a copy of a
   stopped data dir is consistent.
3. Message files never rewritten in place, so the file-level delta is safe.
4. Volume backup of a sub path (the staging dir) through the existing
   pipeline.
5. IMAP clients (Thunderbird, Apple Mail, one phone client) after a move:
   no re-download, UIDVALIDITY unchanged.
6. Real inbound during a fence: senders retry and deliver after cutover,
   nothing lost; measured delay.
7. Queue moved with held and deferred entries delivers once from the
   target.
8. PTR set on the provider, forward confirmed check, blocklist check, and
   Gmail placement during warm-up.
9. Restoring onto an older mox actually fails (to know what the refusal
   protects against).
10. PROXY protocol support in mox (decides option 2).
11. The single box flow of case B: the restored control plane's local
    node identity on VPS 2, agents reconnecting with the restored CA.
12. `alerting.db` copy by hand during a whole instance move.
13. Certificate renewal with DNS-01 for the mail host during the window.

## First slice (under 2 days)

Same host only, no DNS, no second node:

1. `internal/mail/backup.go`: run `mox backup` into an empty staging dir
   and `verifydata` inside the container (exec, as the PreHook already
   does), write the manifest (version, counts, size).
2. Ship the staging dir through the existing volume archive and age path to
   a local folder with `APP_MAIL_BACKUP_LOCAL_CAP_MB` and oldest-first
   pruning.
3. `mail restore --dry-run` and real restore into new volumes on the same
   node, `verifydata` with the target image, version guard (no downgrade).
4. `mail export` of one account from the staging copy to an age-encrypted
   maildir archive, and `mail import` with the duplicate guard.
5. One live opt-in test (`LEVELRAIL_LIVE_MAIL=1`): seed two messages,
   back up, restore into a fresh container, count, export, import into a
   second account, count.

Files: `internal/mail/backup.go`, `internal/mail/export.go`,
`cmd/levelrail-cli/mail_backup.go`, one test file. Env:
`APP_MAIL_BACKUP_LOCAL_CAP_MB`, `APP_MAIL_BACKUP_RETAIN`,
`APP_MAIL_BACKUP_RETAIN_DAYS`, `APP_MAIL_EXPORT_MAX_BYTES`.

## Later slices

| Slice | Content |
| --- | --- |
| 2 | Scheduled backups and drills, alert on missing or failed drill |
| 3 | Case A executor on the `nodemove` runner, `MAILHOST` record kind, TTL computation, fence, final copy, cutover, observe, rollback |
| 4 | `alerting.db` in the control plane backup; case B runbook with mail checks |
| 5 | Mailbox owner notices, maintenance window and silence integration, stepper UI |
| 6 | IMAP copy fallback documentation and an optional helper |

## Alternatives rejected

- **Plain stateful node move of the volumes.** Copies a live store (upstream
  warns this can break the databases), no DNS, no fence.
- **IMAP copy as the default.** Loses UIDs, junk training and the queue,
  needs every password.
- **Dual live servers with sync.** mox has no replication; two writers
  means divergence.
- **Raw volume snapshot as backup.** Same upstream warning as above.

## Consequences

- Backups and migrations share one tested path, so drills also prove
  migrations.
- Mail has a real, short downtime per move (fence to cutover). We say so.
- The mail DNS engine gains one record kind and a TTL step.
- Whole instance moves become two clean steps when mail sits on VPS 1.

## Open questions

1. Is `APP_MAIL_ROLLBACK_WINDOW` of 72h right, given senders retry for
   days and the old IP may be released by the provider sooner?
2. Should a migration be allowed with manual DNS at all, or require a
   managed zone?
3. Do mailbox owners who are not Levelrail users get notices at their
   mailbox only, or also at a recorded alternate address?
4. Do we add `alerting.db` to the control plane backup before slice 3, as
   a prerequisite for case B?
5. For erasure requests, do we offer re-encrypting or pruning specific
   backups early, or is retention expiry acceptable?
6. Do we keep the single downtime variant of case B as a documented
   option, or hide it?
7. Should the target reuse the source IP when the provider supports
   moving a floating IP, skipping PTR and warm-up entirely?
