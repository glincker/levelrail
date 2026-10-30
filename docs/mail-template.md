---
title: "Mail server template"
tag: "Experimental"
description: Install the mox mail server as a one-click template, with private admin ports, your own certificate, optional relay mode and backup notes.
"og:url": "https://docs.levelrail.com/mail-template"
---

<Note>This template appears in the catalog, and can be installed, only when `APP_EXPERIMENTAL=mail` is set on the control plane. With the flag off it is hidden from the listing and refused at install.</Note>

The `mail-mox` template runs the official [mox](https://www.xmox.nl/) image (MIT) as a normal app. It is experimental: the control plane manages it from Settings, Mail only with `APP_EXPERIMENTAL=mail` (see [Experimental features](/experimental-features)). The template is named "Mail server (mox, experimental)" and is shown only when that flag is on. Read [Deliverability](#deliverability-reality) before you move real mail onto it.

Stalwart stays the separate, optional groupware template (`stalwart`, with JMAP and its own management console). This template does not replace or change it.

## Install

One command, with your mail domain as a setting:

```bash
levelrail-cli templates install mail-mox --env MAIL_FIRST_DOMAIN=example.com
```

In the dashboard, open the template and fill in "Mail domain". Everything else is optional and has a default, so the server comes up with a self-signed certificate and no further steps. `templates deploy mail-mox` stops with a message that points to `templates install`, because the domain cannot be guessed. If the domain is missing, the container logs one line saying so and exits.

## What it installs

One container, pinned by image digest, with two named volumes: `mail_config` at `/mox/config` and `mail_data` at `/mox/data`. On first start it runs `mox quickstart` non-interactively for your hostname and admin address, sets the web admin password from a generated secret, and writes `mox.conf` from your inputs. Later starts rebuild only `mox.conf`; `domains.conf` belongs to the control plane and is never rewritten.

mox starts as root only to bind its ports, then runs as numeric uid 1000. The config and data volumes are owned by that uid.

| Input | Meaning |
| --- | --- |
| `MAIL_FIRST_DOMAIN` | Required (install setting "Mail domain"). The first mail domain, for example `example.com`. |
| `MAIL_HOSTNAME` | Optional. The FQDN other servers connect to. Defaults to `mail.` plus the first domain. |
| `MAIL_ADMIN_EMAIL` | Optional. Defaults to `postmaster@` the first domain. |
| `MAIL_TLS_CERT_FILE`, `MAIL_TLS_KEY_FILE` | Optional. Certificate and key paths under `/mox/config`. Existing files win; when absent a self-signed pair for localhost is made (see below). |
| `MAIL_SMARTHOST_HOST`, `_PORT`, `_USER`, `_PASSWORD` | Optional relay. The password is a secret input. Port defaults to 587. |

The admin password is the generated secret `SERVICE_PASSWORD_MAILADMIN` on the app. The admin API listens on container port 8080 and is not published on the host. The webmail and account UI are published as 8025 (container port 8081). The control plane builds the adapter address from the service's container address on the app network and port 8080 (override with `APP_MAIL_ADMIN_PORT`). Queue operations are hold and release.

## Ports

| Port | Use | Exposure |
| --- | --- | --- |
| 25 | Inbound SMTP | Private until you set a public bind |
| 465, 587 | Submission (TLS, STARTTLS) | Private until you set a public bind |
| 993 | IMAP over TLS | Private until you set a public bind |
| 8025 (8081 in the container) | Account UI, webmail, webapi | Private by default. Has no admin pages |
| 8080, 8010 (container only) | Admin UI and API, metrics | Never published. Reach them over the app network only |

The template declares 25, 465, 587 and 993 as published host ports with a private bind ([App ports and exposure](/app-exposure)), so right after install they answer on 127.0.0.1 only. Each port has its own bind: making the mail ports public never publishes the web listener.

The journey is:

1. Install the template.
2. Open the mail ports: `levelrail-cli mail ports open --confirm`, or **Open mail ports** on the Mail page. This sets a public bind on exactly 25, 465, 587 and 993, adds a host firewall allow rule for each, recreates the container and is audited. It asks you to confirm it's you, like other exposure changes. Running it again changes nothing. `levelrail-cli mail ports` shows the bind and firewall state of each port.
3. Set up DNS ([Mail DNS](/mail-dns)).

Before step 2, replace the self-signed certificate with a real one and harden the server. Many providers block port 25 by default: ask yours to allow it. Plain IMAP on 143 is off and not published. mox's own web server and ACME ports (80 and 443) are not published either: you supply the certificate. The Autoconfig and MTA-STS listeners exist in the container so domain configs load, but port 443 is not reachable. Serve the MTA-STS policy and autoconfig hostnames through your ingress if you want them.

## Resources

The container is capped at `APP_MAIL_DEFAULT_MEMORY_MB` (default 256) MiB and `APP_MAIL_DEFAULT_CPUS` (default 1) CPUs, read by the control plane when it loads the catalog. An idle instance uses tens of MiB.

## Pick a node

The template has no node field of its own. Choose the node in the deploy dialog (or the node option of the deploy command) like for any app. Pick a node whose provider allows outbound port 25 and lets you set reverse DNS. See [Multi-node](/multi-node) for placement.

## TLS and restarts

The first start works with no manual steps. mox makes a self-signed certificate itself in the config volume (`certs/selfsigned.pem`) and the log says so. It is valid for `localhost` only, so it is for testing: mail clients and other servers will not trust it for your mail host name. Replace it before sending real mail.

If you set only one of the two certificate inputs, or point them at files that do not exist, mox exits with one line saying which files to add.

To use a real certificate, put the chain and key in the config volume (for example `certs/cur.pem` and `certs/cur.key`), set the two inputs and restart. Existing files are never overwritten. mox has no reload: restart after every renewal. The built-in ACME client is not offered because ports 80 and 443 are not published.

Check the state with `levelrail-cli mail status` (installed, container, admin API, certificate kind and the next step).

### Check and upload a certificate

- `levelrail-cli mail cert status` connects to the mail server's private submission port (465) and shows the served certificate: subject, issuer, expiry, days left, and whether it is self-signed. It sends no credentials and only probes the container's private address.
- `levelrail-cli mail cert upload --cert chain.pem --key key.pem` installs your own certificate. The chain must start with the leaf, cover the mail host name and be valid now; the key must match and must not be encrypted. It asks for your password.
- The key is sealed at rest in the control plane and is never shown again, logged or written to audit. In the config volume the pair lands as `certs/managed.pem` and `certs/managed.key` (mode `0600`); the previous managed pair is kept as `certs/managed.prev.pem` and `.prev.key`.
- mox is then restarted through the normal restart path and probed until it serves the new serial. If it does not come back, or still serves the old certificate after `APP_MAIL_CERT_VERIFY_SECONDS` (default 90), the previous files and settings are restored and mox restarts again.
- With `APP_MAIL_RESTART_WINDOW` set (for example `02:00-05:00`, UTC), uploads outside the window are refused unless you pass `--restart-now`.
- If `MAIL_TLS_CERT_FILE` points at your own files, upload refuses and never touches them. Clear both inputs first.
- `mail cert obtain` (a certificate through the platform's ACME) is not yet implemented and says so.

The node agent must be current: an older agent cannot write the key with mode `0600`, and the upload then stops and removes the key.

## Relay mode

Set `MAIL_SMARTHOST_HOST` to send through a provider. The template writes a `relay1` transport (`Submissions` for port 465, `Submission` otherwise, with `Auth` when a user is set) and a default route to it in `domains.conf` if no route exists yet. Adding or changing a transport needs a container restart.

## Backup

Back up both volumes together. For a consistent copy run `mox backup <dir>` inside the container and copy the result; the per-volume pre-backup hook is a policy set per app with `levelrail-cli backups volumes policy set <app> <volume> --pre-hook CMD`, not part of a template, so set it yourself. A raw volume copy of a busy mailbox store can be inconsistent. Restoring across a mox upgrade is one way: mox has no downgrade path.

## Deliverability reality

Running your own mail server is mostly DNS and reputation work. You need a static IP with a clean history, reverse DNS that matches `MAIL_HOSTNAME`, outbound port 25 open, and the MX, SPF, DKIM, DMARC, MTA-STS and TLS-RPT records for each domain (see [Mail DNS](/mail-dns)). Large providers may still junk or delay mail from a new IP. Relay mode is the safer start for outbound mail.

## Security

What the template does:

- The admin UI, admin API and metrics sit on their own listener (container ports 8080 and 8010) that is never published on the host. Webmail, the account UI and the web API sit on a separate listener (8081), which serves no admin pages: `/admin/` answers 403 there. Reaching webmail does not expose the admin login.
- The TLS certificate and key are required. There is no fallback to a built-in certificate client that could never succeed.
- mox starts as root only to bind ports and then runs as uid 1000.
- The image is pinned by digest and the container is capped on memory and CPU.
- The platform drops all Linux capabilities from every app container and keeps a small set (CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, NET_BIND_SERVICE, SETGID, SETUID), and sets no-new-privileges. mox starts and drops to uid 1000 under that policy; the template cannot ask for fewer, because the compose layer rejects `cap_add`, `cap_drop`, `read_only` and `tmpfs`.

What it does not cover:

- The admin password reaches the container as an environment variable, so anyone with Docker access on the node (`docker inspect`) can read it. Secrets cannot be delivered as files by a template today. Keep Docker access on the node limited to operators.
- The root filesystem stays writable, and the platform keeps a wider capability set than mox needs. Tested with mox v0.0.17: it runs with only CHOWN, DAC_OVERRIDE, FOWNER, NET_BIND_SERVICE, SETGID and SETUID, and also with a read-only root filesystem and a tmpfs at `/tmp`, but a template cannot request either today.
- Ports 25, 465, 587 and 993 serve the internet once you make them public, with the usual exposure of any mail server. Keep the admin and metrics ports private and firewalled.
- The relay password, when set, is also an environment variable with the same exposure.

Guidance: run mail on a dedicated node for production, so a compromise of another app cannot reach the mail volumes and the mail host's IP reputation is not shared with other workloads.

## Local demo mode

To try the Mail page without a mail container, DNS provider or domain, run `scripts/dev-local.sh`. It sets `APP_MAIL_DEMO=1` next to `APP_DEV_MODE=1`; set `LOCAL_MAIL_DEMO=0` to opt out.
Demo mode needs both variables. `APP_MAIL_DEMO` alone is ignored with one warning in the log, and release builds never enable it.
The page shows a DEMO DATA badge. A fake mail server holds two domains, five mailboxes, aliases and a queue with a failing message.
Plan and apply write the real planned records (the same builder as production) into an in-memory zone, never a provider.
The check resolves against that zone: SPF and DMARC fail first, and each record turns green after apply.
Opening the mail ports only flips a fake flag. Use Reset demo on the badge to return to the seeded state.
The default domain is `demo.example`, the mail IP is a documentation address, and every secret is an obviously fake string.
Nothing demo ever reaches a real DNS provider or mail engine.
