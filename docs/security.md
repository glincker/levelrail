---
title: "Security overview"
description: How Levelrail handles secrets, sessions, TLS, and access control, and where to find the detailed page for each.
"og:url": "https://docs.levelrail.com/security"
---

Levelrail keeps your secrets encrypted, signs every request in with a session or a scoped token, and never reaches into your servers over SSH after setup. This page explains how those pieces fit together and links to the detailed page for each. It is a map, not a duplicate.

## Five things to do on a new install

1. **Serve the dashboard over HTTPS and set the dashboard URL.** Once an `https://` URL is set, sign-in over plain HTTP is refused. See [Domains and ingress](/domains-and-ingress).
2. **Give people the narrowest ability that works.** Most humans need `write` and `deploy`; keep `root` for a few. Give CI and AI tokens one ability, and attach a Deny for production apps. See [Identity and access](/identity-and-access).
3. **Turn on two-factor sign-in** for every account that holds `root`.
4. **Pin the control plane on every agent** by setting `APP_CA_FINGERPRINT`, so enrollment cannot be intercepted. See [Multi-node](/multi-node).
5. **Back up the master key separately from the data directory**, and keep it out of the same snapshot. See [Master key rotation](/master-key-rotation) and [Control plane backup](/control-plane-backup).

The [Security center](/security-center) in the dashboard checks most of these from your instance's real state.

For what Levelrail defends against and where it does not yet, read the [threat model](/threat-model). Fixed issues are listed on the [security advisories](/security-advisories) page.

```mermaid
flowchart TD
  MK["Master key<br/>(loaded into memory at start,<br/>stored in master.key or APP_MASTER_KEY)"]
  MK -->|wraps| D1["App A's DEK"]
  MK -->|wraps| D2["App B's DEK"]
  MK -->|wraps| D3["Backup target's DEK"]
  D1 -->|encrypts, AES-256-GCM| V1["secret: true env vars,<br/>bound to (app, key) slot"]
  D2 -->|encrypts, AES-256-GCM| V2["secret: true env vars,<br/>bound to (app, key) slot"]
  D3 -->|encrypts, AES-256-GCM| V3["storage credentials,<br/>bound to their slot"]
  V1 -.->|decrypted only| CT["container create time"]
  V2 -.->|decrypted only| CT
```

## Secrets: envelope encryption

Every owner of secrets (an app, a backup target, the email settings, and so on) gets its own random data encryption key (DEK), and each of its values (an app env var marked `secret: true`, email credentials, API tokens) is encrypted under that DEK with AES-256-GCM. Every DEK is wrapped under one master key. The control plane loads that key into memory at startup. By default the key is generated on first start and stored in `<data dir>/master.key` (mode `0600`); set `APP_MASTER_KEY` to supply it from your own secret manager instead. It is never included in a control plane backup snapshot.

<Warning>
Anyone who can read your whole data directory (a disk image, a host-level backup, a cloud snapshot) can read `master.key` next to the database and decrypt every secret. Keep that key file out of host backups and snapshots, or store it elsewhere with `APP_MASTER_KEY_FILE=/etc/levelrail/master.key` (an absolute path outside the data directory), or supply it with `APP_MASTER_KEY` from a secret store.
</Warning>

Each encrypted value is also bound to the slot it was written to: its owner and key name are sealed inside the ciphertext and checked on every read. Someone with write access to the database cannot copy one app's `DATABASE_URL` ciphertext into another app's `API_KEY` row and have it decrypt there; the read fails closed instead. Values written before this existed are "legacy" and should be bound once with `levelrail-cli secrets rebind`, see [Binding secrets to their slot](/master-key-rotation#binding-secrets-to-their-slot).

<Tip>

Rotating the master key re-wraps every DEK without ever exposing plaintext, then binds any legacy values. See [Master key rotation](/master-key-rotation) for the full procedure and failure modes.

</Tip>

Sensitive plain variables and secrets are masked on every read surface, and reading one back needs a confirmed, audited step-up. See [Environment variables and secrets](/environment-and-secrets).

Credentials for backup targets and registry integrations follow the same write-only pattern described in [Backups and storage](/backups-and-storage#credentials-are-write-only): once saved, the plaintext is never returned by the API again, only a masked placeholder.

### Wrap the master key with Vault or OpenBao

By default the master key sits in `<data dir>/master.key` (or `APP_MASTER_KEY`). To make a copy of the data directory useless on its own, wrap it with a Transit key from HashiCorp Vault or OpenBao. The control plane asks Vault to unwrap it once at startup and holds it only in memory.

| Variable | Meaning |
| --- | --- |
| `APP_KMS_PROVIDER` | `vault-transit` |
| `APP_VAULT_ADDR` | Vault URL. HTTPS is required, plain HTTP is allowed only to localhost. |
| `APP_VAULT_TOKEN` or `APP_VAULT_TOKEN_FILE` | Token allowed to `encrypt` and `decrypt` with the key |
| `APP_VAULT_TRANSIT_KEY` | Transit key name |
| `APP_VAULT_TRANSIT_MOUNT` | Mount path, default `transit` |
| `APP_VAULT_NAMESPACE` | Namespace, if you use one |

```sh
levelrail wrap-master-key           # seals master.key, verifies the round trip first
levelrail wrap-master-key --unwrap  # writes the plain key back
```

A wrapped key never falls back: if Vault is unreachable or the settings are missing, the control plane refuses to start and tells you why, instead of generating a new key. Rotating the master key re-wraps the new key automatically. A new install with these variables set is wrapped from the first start. Master key escrow in [disaster recovery](disaster-recovery.md) unwraps the key first, so the escrow bundle still holds a key you can use offline.

The Vault token is the new thing to protect: anyone holding it and the data directory can unwrap the key.

### Notification channel secrets

A notification channel's credential (its webhook URL, Telegram bot token, PagerDuty routing key, Opsgenie or Resend key, and a Slack signing secret or Discord public key for interactive approvals) is envelope encrypted like any other secret, under the channel's own slot. `alerting.db` keeps only a masked placeholder such as `https://hooks.slack.com/(hidden)`, and the API returns that placeholder to every caller, `admin` included. The value is decrypted only at the moment a notification, test message or approval request is sent, or an interactive button click is verified. An `email` channel's recipient address is not a credential and stays readable.

Channels saved by an older release are encrypted in place on the first start with a master key: each value is written as ciphertext, decrypted again to verify it, and only then is the plaintext column cleared. The step is idempotent and safe to interrupt; anything it could not finish is retried on the next start. Without a master key, channels keep working with plaintext credentials and `levelrail-cli doctor` warns with a `notification_channel_secrets` check until one is configured.

## Passkeys

Manage passkeys under Settings, Account. Each passkey shows the authenticator that stored it (iCloud Keychain, Google Password Manager, Windows Hello, a password manager or a YubiKey, resolved from its AAGUID, or plain "Passkey" when unknown), whether it is synced or device-bound, how it connects, when it was added and last used, and how many sign-ins it has made.

- **Add**: the dialog suggests a name from your device, walks through naming, the browser prompt and saving, and explains every failure (cancelled, timed out, already registered, wrong address, server rejected) with a retry.
- **Rename and remove**: rename inline. Removing your only passkey is refused when you have no password and no two-factor, so you cannot lock yourself out; add another passkey first.
- **Test sign-in**: runs a real passkey check without creating a new session and updates last used.
- **CLI**: `auth passkeys list`, `auth passkeys rename --name NAME ID` and `auth passkeys remove ID` (session only, like the rest of `auth`). Adding a passkey needs a browser.

Passkeys only work on the exact https address set as the dashboard URL.

## Sessions and tokens

- **Session cookies** are `HttpOnly`, `SameSite=Lax`, and `Secure` whenever the request arrived over HTTPS (directly or through the embedded Caddy ingress). Once an `https://` dashboard URL is set, sign-in over plain HTTP is refused (`APP_ALLOW_INSECURE_LOGIN=true` is the recovery escape hatch).
- **Cross-origin guard**: a state-changing request (POST, PUT, PATCH, DELETE) that authenticates with the session cookie and carries an `Origin` header naming a different host than the one served is rejected with 403. Bearer-token and CLI requests, which send no cookie or no `Origin`, are unaffected.
- **First admin** registration requires the one-time setup token from `<data dir>/setup-token`, so an exposed fresh install can't be claimed by a stranger.
- **Rate limits use the real client address.** Behind the embedded ingress the control plane reads the right-most untrusted hop of `X-Forwarded-For`, but only when the TCP peer is loopback or listed in `APP_INGRESS_TRUSTED_PROXIES`. Any other peer cannot spoof its address, so one visitor can no longer exhaust the login or webhook budget for everyone.
- **Token-redeeming routes are rate limited.** `POST /api/v1/auth/reset-password` and `POST /api/v1/invites/accept` are unauthenticated by design, so each gets a per-client-IP budget (`APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM`, default `10` per minute, `0` disables). Over budget returns `429` with `Retry-After`.
- **API tokens** are minted per-user, scoped by ability (a user can only mint a token holding abilities they hold themselves), and can be issued through a device-code flow for headless environments.
- **Tokens are bound to the server that issued them.** The server publishes its identity at `GET /api/v1/instance` (instance id, name, version, and a fingerprint, an HMAC of the id under a per-instance key that never leaves the database). The CLI pins that identity per profile at login and checks it before every first request, so a wrong URL, a stale tunnel or a hijacked DNS name never receives the token. The server also remembers which instance each token was first used on and refuses it on any other. A database restored onto a new host (the `instance.marker` file in the data dir is missing or different) gets a new identity, so bound tokens stop working until a root operator confirms "same instance" on **Settings, CLI access**.
- **Two-factor authentication (TOTP)** is available per user, with recovery codes for account lockout.
- **Two-factor codes are single-use.** A TOTP code that was already accepted is refused if presented again inside its validity window, and recovery codes are consumed on use.
- **Read-tier callers cannot enumerate people or credentials.** `GET /api/v1/users` returns only the caller's own record unless the caller is `root`, and notification destinations (webhook URLs, bot tokens, routing keys) are shown as `(hidden)` without `read:sensitive`, see [observability](/observability#notification-channels).

Full detail on all three: [Identity and access](/identity-and-access#principals-a-session-or-a-token).

## Authorization: abilities, roles, and IAM policies

Levelrail layers two permission models:

- **Abilities**: a flat list (`AbilityRead`, `AbilityWrite`, `AbilityDeploy`, `AbilityRoot`, and so on) attached to a user or token. Cheap to check, coarse-grained.
- **IAM policies**: AWS-IAM-shaped Allow/Deny documents scoped to a specific app or database, for when a flat ability isn't precise enough.

Evaluation order, the full ability list, and policy examples: [Identity and access](/identity-and-access#why-two-permission-models-instead-of-one).

## TLS and network exposure

- Certificates are issued and renewed automatically through embedded Caddy's ACME client. See the [ACME verification runbook](/acme-verification-runbook) if issuance fails.
- HSTS (`Strict-Transport-Security`) is opt-in, not on by default, because turning it on for a domain that later loses TLS locks users out until the header expires. See [Domains and ingress](/domains-and-ingress#tls-whats-actually-shipped-today).
- WAF mode and rate limiting are configurable per domain, with a detect-only mode for testing rules before enforcing them. See [Domains and ingress](/domains-and-ingress#opt-in-waf-and-rate-limiting).
- The node agent dials **out** to the control plane. No inbound ports need to be open on a managed server for enrollment or day-to-day operation.
- **Agent enrollment pins the control plane CA.** A join token is shown together with the agent CA's SHA-256 fingerprint; with `APP_CA_FINGERPRINT` set, the agent checks the control plane's certificate against that CA before it sends the token, so an attacker in the network path cannot capture the token or pose as the control plane. Without the fingerprint the agent refuses to enroll, so the token is never sent to an unverified server; setting `APP_ALLOW_UNPINNED_ENROLL=1` opts back in to trust on first use and logs a warning. The enrollment connection requires TLS 1.3. After enrollment every connection is mutual TLS against the saved CA.
- **Container installs bind plain HTTP to loopback.** The committed `docker-compose.yml` publishes `8080` on `127.0.0.1` only (override with `LEVELRAIL_HTTP_BIND`), see [Docker](/docker#control-plane).
- **`install.sh` fails closed on checksums.** A release without a usable `checksums.txt` aborts the install unless you pass `LEVELRAIL_SKIP_CHECKSUM=1`. Release checksums are cosign-signed; `APP_INSTALL_VERIFY=require` makes the installer insist on a valid signature, see [Verifying release binaries](/installing#verifying-release-binaries).

## Outbound requests to user-supplied URLs

Alert rules, deploy notifications, and notification channels post to URLs an operator types in. To keep those from being pointed at the control plane's own network (cloud metadata at `169.254.169.254`, the Docker API, databases on a private subnet), every notification request refuses to connect to loopback, private (RFC 1918, `fc00::/7`), link-local, CGNAT, multicast, and other reserved addresses. The check runs on the IP actually dialed, after DNS resolution and on every redirect hop, so a hostname that resolves (or later re-resolves) to an internal address is caught too. Proxy environment variables are ignored for these requests for the same reason.

If you run a notification receiver on your own network (a self-hosted Mattermost, Gotify, or ntfy on a private IP, for example), set:

```bash
APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true
```

on the control plane. This re-allows every internal address for notification requests, so only enable it when every user who can create alert rules or notification channels is trusted with access to that network.

The same check runs when you create or edit a notification channel: a URL that is not `http(s)`, or that is a loopback, private or link-local address (or `localhost`, or a hostname that resolves only to those), is refused up front with a `400` that names `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS`, instead of being saved and failing silently on every send. Set the variable before creating the channel when the receiver really is on a private network.

Email notifications are separate: recipient addresses must be a single plain address, and subjects are MIME-encoded, so user input cannot add mail headers.

## Fresh-box hardening checklist

A short list for the server itself, independent of anything Levelrail configures:

- **SSH key auth only.** Disable password login (`PasswordAuthentication no` in `sshd_config`) before exposing the box to the internet. Neither `install.sh` nor the agent touch SSH configuration; this is standard server hygiene, not something Levelrail does for you.
- **Only the control-plane node needs inbound ports.** `80/tcp` and `443/tcp` for ingress (the ACME HTTP-01 challenge plus HTTPS traffic), your SSH port, and nothing else. See [Installing: requirements](/installing#requirements) for the full list and `install.sh`'s optional `ufw` setup.
- **`levelrail-cli doctor`'s `firewall` check detects ufw, firewalld, and nftables/iptables**, whichever is actually installed on this host, and reports a concrete command to open 80/443 when it isn't. No supported tool installed is reported as informational, not a failure: this platform can't tell whether you're relying on something it can't introspect (a cloud security group, for example).
- **Agent-only nodes should accept no inbound traffic at all.** The node agent dials out to the control plane over mTLS; the control plane never initiates a connection. A server running only the agent has nothing that needs to accept a connection, so leave its firewall closed by default rather than opening anything speculatively.
- **Don't expose anything Levelrail didn't ask you to.** Docker's daemon socket, the SQLite database file, and the mesh DNS listener (`:5390` by default, see [Multi-node](/multi-node#wireguard-mesh-and-internal-dns)) are all meant to stay local to the box.
- **Keep the OS patched.** The `patch_status` alert (see [Observability](/observability)) can warn you about pending security patches on a managed node, but it only reports, it doesn't apply anything; that's still on you (`unattended-upgrades`, `dnf-automatic`, or your distro's equivalent).

## Security settings and data retention

Security protections default to on. An environment variable can only override a default, and an administrator can change any registered setting under **Settings, Security and runtime** or with `levelrail-cli settings runtime`. A saved value beats the environment variable, which beats the default. Turning a protection off needs an explicit confirmation, writes an audit row, and the page says which settings need a restart. A first-run review in the Get Started hub lists the defaults for you to confirm.

Retention is managed in one place, **Settings, Cleanup**: a retention period per data type with a minimum for security data (audit entries cannot be kept for less than 90 days, security events for less than 365), a dry-run preview before anything is deleted, a history of every run, and CSV and JSON export. See [Cleanup center](/cleanup-center).

## Audit log

Every mutating API call from an authenticated principal is recorded: who, what, when, and the outcome. The log is queryable, exportable as NDJSON or CSV, verifiable as a hash chain, and streamable to a webhook or an S3 bucket. Reads that can expose secrets or personal data, such as app and database logs, Loki queries and data exports, are recorded too. Every change to a security setting and every cleanup run is audited with counts only, never row contents or secret values. See [Identity and access](/identity-and-access#audit-log).

## Container hardening

Every container Levelrail creates (apps, databases, catalogue services, helpers) can be created with secure defaults: all Linux capabilities dropped except a minimal set, `no-new-privileges`, and a per-container process limit. It is controlled by `APP_CONTAINER_HARDENING` on the control plane and on each agent:

| Value | Behavior |
| --- | --- |
| `enforce` (default) | Applies the settings below to every container created from then on. Running containers pick them up on their next recreate (redeploy or restart of the resource). |
| `warn` | Nothing is applied to containers. `GET /api/v1/system/doctor` (`levelrail-cli doctor`) reports the exact settings `enforce` would apply. Use it to opt out while you work out which images need extra capabilities. |
| `off` | Nothing applied, and the doctor warns that Docker's default capability set is in use. |

`enforce` is the default: an image that needs a capability outside the minimal set fails when it starts, and the fix is `APP_CONTAINER_HARDENING_CAP_ADD` (or `APP_CONTAINER_HARDENING=warn` while you investigate). The stock nginx, postgres, mysql, mongo and redis images were verified to start under these settings.

What `enforce` sets:

- `CapDrop: ALL`, then `CapAdd`: `CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `KILL`, `NET_BIND_SERVICE`, `SETGID`, `SETUID`. This is what the stock nginx, postgres, mysql, redis, node and python images need for their entrypoints (chown a data dir, drop privileges to a service user, bind port 80 or 443). Docker's default set also grants `NET_RAW` (raw sockets, ping), `MKNOD`, `SETFCAP`, `SETPCAP`, `SYS_CHROOT` and `AUDIT_WRITE`, which are left out.
- Capabilities a caller already asks for (the egress sidecar's `NET_ADMIN`) are kept.
- `SecurityOpt: no-new-privileges`, so a process cannot gain privileges through setuid binaries.
- `PidsLimit` from `APP_CONTAINER_PIDS_LIMIT` (default `4096`, `0` disables), which stops a fork bomb from exhausting the host.

`APP_CONTAINER_HARDENING_CAP_ADD` (comma separated, for example `SYS_CHROOT,NET_RAW`) adds capabilities for every container on that host, for images that need more than the minimal set (SSH or FTP servers, tools using `ping`). It applies host-wide because a per-app override would need a new field carried to remote agents; a per-app `security` block in `app.yaml` is future work. Read-only root filesystems and privileged containers are never set by these defaults. The setting is read by the process that creates the container, so it applies to remote nodes when set on the agent, and the doctor reports the control plane's own value.

## Docker API guard

The control plane and each agent reach Docker through an in-process allowlisting proxy that refuses privileged containers, host namespaces, unsafe capabilities and protected host mounts. It ships in `enforce` mode, with `APP_DOCKER_GUARD=audit` and `off` as overrides. See [Docker access and the API guard](/docker-access) for the endpoint list, every rule, and its limits.

## Rootless and Podman

`GET /api/v1/system/doctor`'s `container_runtime` check (`levelrail-cli doctor`) reports which container engine, privilege mode and support level the control plane has: Docker, Podman, Docker Desktop, Colima, OrbStack, Rancher Desktop or unknown, rootful or rootless (rootful Docker on Linux is `supported`, other known engines `best-effort`, unknown `unverified`), and which setting decided the socket (`APP_CONTAINER_RUNTIME_SOCKET`, then the standard `DOCKER_HOST`, then a well-known rootless, Podman or desktop runtime socket path (`~/.docker/run`, `~/.colima/default`, `~/.orbstack/run`, `~/.rd`), then the rootful default `/var/run/docker.sock`).

Podman exposes a Docker-compatible Engine API socket, so the existing Docker Engine API client talks to it unchanged, no CLI shelling and no new client library, consistent with the rule that Levelrail never shells out to the Docker CLI. `APP_CONTAINER_RUNTIME_SOCKET` pins the connection to a specific socket (for example `unix:///run/user/1000/podman/podman.sock`) ahead of `DOCKER_HOST`, for operators who would rather not export `DOCKER_HOST` into the whole process environment.

Container hardening (above) adjusts automatically under detected rootless: `PidsLimit` is disabled by default, because rootless Docker without a delegated cgroup v2 controller rejects a pids-limit `HostConfig` at container-create time. Set `APP_CONTAINER_PIDS_LIMIT` explicitly to override this. `CapDrop`, `CapAdd` and `no-new-privileges` are unaffected: those apply inside the container's own user namespace regardless of rootless mode, so nothing needed to come off the minimal capability list.

**What is not verified:** this detection and the hardening adjustment above were tested against a fake Docker client, not a real rootless Docker or Podman installation. Known gaps to check before relying on this in production:

- **Bind-mount ownership.** A bind mount's files keep the UID/GID they have on the host; under rootless Docker/Podman that UID sits inside a remapped user namespace, so a container process expecting to own a bind-mounted path can see a permission mismatch that doesn't happen under rootful Docker. Named volumes (Levelrail's default for managed databases) aren't affected the same way.
- **Low host ports.** Rootless port publishing goes through a userspace proxy (`rootlesskit`/`slirp4netns`), which by default can't bind host ports below 1024 regardless of the container's own capabilities. The embedded ingress runs on the host directly, not in a rootless container, so this only affects an app that needs to publish a low host port itself.
- **cgroup limits beyond PidsLimit.** Memory and CPU limits also need a delegated cgroup v2 controller under rootless; unlike PidsLimit, this isn't currently detected or adjusted for `Resources.MemoryBytes`/`NanoCPUs`.

Source builds (Dockerfile, Railpack) need BuildKit inside dockerd: on Podman, image and compose deploys are expected to work and source builds are not verified. Treat single-container rootless or Podman hosts as best-effort, not fully verified, until tested against a real installation.

## Other hardening details

<AccordionGroup>
<Accordion title="CSRF">

A cookie-authenticated state-changing request is refused when its `Origin` names another host, when `Origin` is `null`, when there is no `Origin` but `Sec-Fetch-Site` says `cross-site` or `same-site`, or when there is no `Origin` and the `Referer` names another host. Requests with a bearer token or no session cookie are not subject to the check.

</Accordion>
<Accordion title="Static responses">

Redirect targets and custom error pages are written to the ingress with `{` and `}` escaped, so a placeholder such as `{env.NAME}` or `{file.path}` in operator text can never be expanded into a control plane secret.

</Accordion>
<Accordion title="Certificates">

Stored certificates from other CAs are purged only when the configured CA changes, not on every start, so a restart never deletes valid certificates or spends Let's Encrypt rate limit. The sslip.io hostname is only offered for a publicly routable address (carrier-grade NAT, benchmarking and reserved ranges are refused).

</Accordion>
<Accordion title="Bind mounts">

In addition to `/etc`, `/root` and the Docker socket, `/run`, `/dev`, the directories that hold executables (`/bin`, `/sbin`, `/lib`, `/usr/bin`, `/usr/local/bin` and similar), `/var/spool/cron`, the control plane and agent data directories (including a custom `APP_DATA_DIR`), and any parent directory that would contain a protected path (`/var`, `/var/lib`) are refused. Paths are checked lexically: a symlink on the host that points into a protected path is not followed, so do not create such symlinks inside a directory you let apps mount.



Paths that are or sit inside a credential directory (`.ssh`, `.gnupg`, `.aws`, `.kube`, `.docker`), a bare home directory such as `/home/deploy`, `/var/log` and `/usr/local/etc` are refused too. Set `APP_BIND_ALLOWED_ROOTS` (colon-separated absolute directories) to allow bind mounts only inside those directories; it is also a setting under **Settings, Security and runtime**.

</Accordion>
<Accordion title="Webhook replay protection">

A git webhook is accepted only with a valid signature. A signed body that already started a deploy is also ignored for a short window (`APP_WEBHOOK_REPLAY_WINDOW`, 10 minutes by default), even if the delivery header changes or is missing, because delivery headers are not covered by the signature. A rejected or ignored delivery does not use up the window, so a provider retry still works. The window is kept in memory and resets on restart.

</Accordion>
<Accordion title="Registry port">

The built-in registry's plain HTTP port is published on loopback only. Remote nodes pull through the TLS route. An existing container published on all interfaces is recreated.

</Accordion>
<Accordion title="Git fetches">

`repo_url` clones, branch listing and pipeline checkouts go through the same guarded HTTP client as webhooks: internal, loopback, link-local and metadata addresses are refused, including after redirects and DNS changes. A self-hosted Git server on a private address needs `APP_GIT_ALLOW_PRIVATE_NETWORKS=true`, which relaxes only git fetches. The older `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true` still works for git but also relaxes notification webhooks, so prefer the git-specific one.

</Accordion>
<Accordion title="Build logs">

Private keys, bearer tokens, token-shaped strings and credentials in URLs are masked before a build or clone log line is stored or streamed.

</Accordion>
<Accordion title="Agent enrolment">

A node name must be 1 to 63 letters, digits, dot, underscore or hyphen, and `control_plane_addr` for provisioning must be `host:port`, so neither can inject lines into the agent's environment file.

</Accordion>
<Accordion title="install.sh">

Data and install directories must be absolute, free of shell and unit metacharacters, and the data directory may not be a system directory (it is the target of `rm -rf` on `uninstall --purge`). Ports must be numbers in range and `LEVELRAIL_VERSION` is restricted to a safe character set.

</Accordion>
</AccordionGroup>

## Node trust

A new node enrolls with a join token. The control plane stores only a SHA-256 hash of it, shows the plaintext once, and the token is single use and expires after 15 minutes by default (`APP_NODE_JOIN_TOKEN_TTL`). The agent generates its own private key and sends only a certificate signing request, so the key never leaves the node. The control plane signs a client certificate, and later connections present it and are checked against the agent CA. Details and failure modes: [Multi-node](/multi-node).

## AI assistant safety

The built-in assistant cannot change anything on its own. The server, not the model or the browser, decides which tool calls pause:

- A tool call that is not marked read-only pauses for an explicit human confirmation before it runs. A tool with no annotations is treated as mutating and also pauses.
- Tools whose output can contain untrusted text (logs, remote content) mark the conversation as tainted. Once tainted, even read-only tools that reach outside the system or return sensitive data pause for confirmation.
- A confirmation belongs to one chat session and can be resolved once.

This lives in `internal/ai` (`classify.go`, `engine.go`). See [AI assistant](/ai-assistant) and [AI control](/ai-control).

## Production hardening checklist

1. Set an `https://` dashboard URL and serve it through the ingress. Sign-in over plain HTTP is then refused.
2. Keep port `8080` on loopback (the default in `docker-compose.yml`) and reach it through the ingress or an SSH tunnel.
3. Set `APP_CA_FINGERPRINT` on every agent, using the fingerprint shown with the join token.
4. Turn on two-factor sign-in for every account that holds `root`.
5. Keep the master key outside the data directory and outside host snapshots, or wrap it with Vault or OpenBao. See [Master key rotation](/master-key-rotation).
6. Back up the control plane on a schedule and test a restore. See [Control plane backup](/control-plane-backup).
7. Update on a regular cadence, because fixes ship only in the latest release. See [Upgrade safely](/upgrade-safely).
8. Open the [Security center](/security-center) and clear what it flags.
9. Read the [threat model](/threat-model), [deploy safety](/deploy-safety) and [supply chain](/supply-chain) pages for what each layer covers.

## Reporting a vulnerability

Levelrail does not yet have a dedicated security disclosure address. Until one exists, open a private security advisory on the [GitHub repository](https://github.com/glincker/levelrail/security/advisories/new) rather than a public issue. The repository's [SECURITY.md](https://github.com/glincker/levelrail/blob/main/.github/SECURITY.md) has the full policy, including what to expect after reporting.

## What this does not cover yet

- No third-party penetration test or formal certification (such as SOC 2 or ISO 27001) is claimed.
- The node agent runs as root in its container with access to the Docker socket, because it manages containers and creates the WireGuard device. A stolen node certificate, or a compromised control plane, is therefore equivalent to root on the nodes it manages. Keep node enrollment tokens short-lived and pin the agent CA fingerprint.
- One control plane runs the whole install. There is no high-availability control plane, so back it up.
- No LDAP. Sign-in is local password, passkey, email sign-in code, OAuth or OIDC (20 providers plus generic OpenID Connect, see [Identity and access](/identity-and-access)), or beta [SAML](/sso-saml) single sign-on (off by default), with beta [SCIM](/scim) provisioning. SAML and SCIM sit behind the `saml-scim` feature toggle and have not been verified against a live identity provider yet.
- [Secret scanning](/secret-scanning) checks the checked-out tree and plain env values at deploy time, but not git history or image layers.
- Dashboard sessions are stored in the database and survive a restart. Sessions minted from API-token session links are held in memory and end on restart. API tokens are unaffected.
- `GET /api/v1/system/containers` lists containers Levelrail does not manage only to `root` callers. Other callers with `read` see managed containers only.
- Container hardening is on by default but has no per-app override yet, and read-only root filesystems stay opt-in.
- Rootless Docker and Podman detection and hardening adjustment ([above](#rootless-and-podman)) exist but are not verified against a real installation; bind-mount ownership and cgroup resource limits beyond `PidsLimit` are known gaps.

## Next steps

<CardGroup cols={2}>
<Card title="Identity and access" href="/identity-and-access">

Users, tokens, roles, and IAM policies.

</Card>
<Card title="Threat model" href="/threat-model">

Trust boundaries, mitigations with file references, and known gaps.

</Card>
<Card title="Master key rotation" href="/master-key-rotation">

Rotate the key that protects all secrets.

</Card>
<Card title="Domains and ingress" href="/domains-and-ingress">

TLS certificates and domain configuration.

</Card>
</CardGroup>
