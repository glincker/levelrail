---
title: "Mail server template"
tag: "Experimental"
description: Install the mox mail server as a one-click template, with private admin ports, your own certificate, optional relay mode and backup notes.
"og:url": "https://docs.levelrail.com/mail-template"
---

The `mail-mox` template runs the official [mox](https://www.xmox.nl/) image (MIT) as a normal app. It is experimental: the control plane manages it from Settings, Mail only with `APP_EXPERIMENTAL=mail` (see [Experimental features](/experimental-features)). The template itself is in the catalog either way, named "Mail server (mox, experimental)". Read [Deliverability](#deliverability-reality) before you move real mail onto it.

Stalwart stays the separate, optional groupware template (`stalwart`, with JMAP and its own management console). This template does not replace or change it.

## What it installs

One container, pinned by image digest, with two named volumes: `mail_config` at `/mox/config` and `mail_data` at `/mox/data`. On first start it runs `mox quickstart` non-interactively for your hostname and admin address, sets the web admin password from a generated secret, and writes `mox.conf` from your inputs. Later starts rebuild only `mox.conf`; `domains.conf` belongs to the control plane and is never rewritten.

mox starts as root only to bind its ports, then runs as numeric uid 1000. The config and data volumes are owned by that uid.

| Input | Meaning |
| --- | --- |
| `MAIL_HOSTNAME` | Required. The FQDN other servers connect to, for example `mail.example.com`. |
| `MAIL_FIRST_DOMAIN` | Required. The first mail domain. |
| `MAIL_ADMIN_EMAIL` | Optional. Defaults to `postmaster@` the first domain. |
| `MAIL_TLS_CERT_FILE`, `MAIL_TLS_KEY_FILE` | Optional. Certificate and key paths under `/mox/config`, mapped to `TLS: KeyCerts` on the public listener. |
| `MAIL_SMARTHOST_HOST`, `_PORT`, `_USER`, `_PASSWORD` | Optional relay. The password is a secret input. Port defaults to 587. |

The admin password is the generated secret `SERVICE_PASSWORD_MAILADMIN` on the app. The admin API base URL is `http://<private address>:8025/admin/api/`.

## Ports

| Port | Use | Exposure |
| --- | --- | --- |
| 25 | Inbound SMTP | Private until you set a public bind |
| 465, 587 | Submission (TLS, STARTTLS) | Private until you set a public bind |
| 993 | IMAP over TLS | Private until you set a public bind |
| 8025 (80 in the container) | Admin, account UI, webmail, webapi | Private. Never make it public |
| 8010 | Metrics | Private. Never make it public |

Published ports bind privately by default ([App ports and exposure](/app-exposure)). Set a public bind for 25, 465, 587 and 993 in the app's Network settings and open them in the host firewall. Plain IMAP on 143 is off and not published. mox's own web server and ACME ports (80 and 443) are not published either: you supply the certificate. The Autoconfig and MTA-STS listeners exist in the container so domain configs load, but port 443 is not reachable. Serve the MTA-STS policy and autoconfig hostnames through your ingress if you want them.

## Resources

The container is capped at `APP_MAIL_DEFAULT_MEMORY_MB` (default 256) MiB and `APP_MAIL_DEFAULT_CPUS` (default 1) CPUs, read by the control plane when it loads the catalog. An idle instance uses tens of MiB.

## Pick a node

The template has no node field of its own. Choose the node in the deploy dialog (or the node option of the deploy command) like for any app. Pick a node whose provider allows outbound port 25 and lets you set reverse DNS. See [Multi-node](/multi-node) for placement.

## TLS and restarts

Put the certificate chain and key in the config volume (for example `certs/cur.pem` and `certs/cur.key`) and set the two inputs. mox has no reload: when the certificate renews, restart the container. Without a certificate the template falls back to mox's built-in ACME client, which needs ports 80 and 443 that this template does not publish, so treat a certificate as required for real use.

## Relay mode

Set `MAIL_SMARTHOST_HOST` to send through a provider. The template writes a `relay1` transport (`Submissions` for port 465, `Submission` otherwise, with `Auth` when a user is set) and a default route to it in `domains.conf` if no route exists yet. Adding or changing a transport needs a container restart.

## Backup

Back up both volumes together. For a consistent copy run `mox backup <dir>` inside the container and copy the result; the per-volume pre-backup hook is a policy set per app with `levelrail-cli backups volumes policy set <app> <volume> --pre-hook CMD`, not part of a template, so set it yourself. A raw volume copy of a busy mailbox store can be inconsistent. Restoring across a mox upgrade is one way: mox has no downgrade path.

## Deliverability reality

Running your own mail server is mostly DNS and reputation work. You need a static IP with a clean history, reverse DNS that matches `MAIL_HOSTNAME`, outbound port 25 open, and the MX, SPF, DKIM, DMARC, MTA-STS and TLS-RPT records for each domain (see [Mail DNS](/mail-dns)). Large providers may still junk or delay mail from a new IP. Relay mode is the safer start for outbound mail.
