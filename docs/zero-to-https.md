---
title: "Zero to HTTPS"
sidebarTitle: "Zero to HTTPS"
description: "From the install command to a trusted HTTPS address, first on a test address and then on your own domain, with what each failure means."
"og:url": "https://docs.levelrail.com/zero-to-https"
---

This page is the short path from a fresh Linux server to a working HTTPS address. Do the steps in order. Each one links to the full page for the details.

## 1. Install

On a fresh server, as a user that can use `sudo`:

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

The installer prints the dashboard link and a one-time setup token. Requirements and options are in [Installing Levelrail](/installing).

## 2. Open the ports

| Port | Open to | Why |
| --- | --- | --- |
| 80/tcp | The internet | Let's Encrypt HTTP-01 challenge and the redirect to HTTPS |
| 443/tcp | The internet | HTTPS |
| 22/tcp | Your address | SSH, to read the setup token |
| 8080/tcp | Your address only, or nobody | Dashboard over plain HTTP until HTTPS is on |

The installer does not open 8080. If you skip that rule, reach the dashboard through an SSH tunnel. The reasons and the tunnel command are in [Dashboard port 8080](/installing#dashboard-port-8080). Per-provider steps for the cloud firewall are in [Install on a cloud VM](/install-on-cloud#firewall-ports) and [Install on Oracle Cloud](/install-on-oracle-cloud). The host firewall is covered in [Host firewall](/host-firewall).

Check that the server is reachable from outside before you go on:

```bash
levelrail-cli doctor
```

An `external_reachability_80` warning is a hint, not proof. Some networks cannot reach their own public address, so test from another machine.

## 3. Sign in and enable HTTPS on a test address

Open the setup link from the installer output, create the admin account, then press **Enable HTTPS** on the first-run screen or the Domains page. The contact email is prefilled.

The same from a terminal:

```bash
levelrail-cli settings ingress https enable --wait 2m
levelrail-cli settings ingress https status
```

This sets the dashboard address to `<dashed-ip>.sslip.io`, for example `203-0-113-10.sslip.io`, and asks Let's Encrypt for a certificate. No DNS record is needed.

Things to know:

- The address works only if your resolver answers for sslip.io. Some routers and DNS filters block it. Run `dig <name>` before you assume the server is down.
- Enabling is capped at 5 attempts per hour. Add `--staging` to rehearse against the test CA, which has no meaningful limits. Staging certificates are not trusted by browsers.
- When the dashboard is on `https://`, sign-in over plain HTTP is refused. Use the https address and close 8080.

Details and the hint table: [Zero DNS setup](/domains-and-ingress#zero-dns-setup-sslipio-hostnames-and-one-click-https).

## 4. Use your own domain

The sslip.io address is for testing. For a real site, add a domain.

1. Create an `A` record for the name pointing at the server's public IP. Use `AAAA` for an IPv6-only server. A subdomain may use a `CNAME` to the server's public hostname instead. Do not put a `CNAME` at the zone apex.
2. Check it from outside your network:

```bash
dig +short shop.example.com
levelrail-cli domains check my-app shop.example.com
```

3. Add the domain to the app and wait for the certificate:

```bash
levelrail-cli apps domains add my-app shop.example.com --wait
```

DNS changes can take minutes to hours, depending on the record's TTL. If you connected a DNS provider, `--dns preview` shows the record without changing anything. See [Domains and ingress](/domains-and-ingress#walkthrough-your-first-domain).

## 5. When it does not work

Run the diagnosis. It checks DNS, the CAA record, ports 80 and 443 and the certificate, and gives one fix per problem:

```bash
levelrail-cli domains diagnose shop.example.com
```

| Symptom | What it means | Fix |
| --- | --- | --- |
| `dns.resolves` shows different answers from different resolvers | DNS is still propagating | Wait for the longest TTL, then run it again |
| `dns.points_here` fails | The name does not resolve to this server | Fix the `A` or `AAAA` record |
| `dns.proxied` | The domain is behind a CDN proxy, so visitors and the CA reach the proxy, not this server | See [Cloudflare in front](/cloudflare-in-front) |
| `dns.aaaa_mismatch` | An `AAAA` record sends IPv6 visitors elsewhere | Remove it or point it here |
| `caa.blocks_issuer` | A CAA record does not allow Let's Encrypt | Add a CAA record for `letsencrypt.org` or remove the block |
| `net.port80` fails or the hint is `unreachable` | Let's Encrypt cannot reach port 80 | Open 80 and 443 in the host firewall and the cloud firewall |
| `Invalid response from http://` | Something else answers on port 80 | Stop it, or forward it to Levelrail |
| Hint `rate_limited` | Let's Encrypt is throttling the name | Wait. The platform retries. Use staging while you debug |

After a fix, run the diagnosis again. It never assumes a fix worked. Full reference: [Diagnose a domain](/domain-diagnosis).

## 6. Watch the certificate budget

Let's Encrypt limits how many certificates a server can get. Every automatic app address (`<app>.<dashed-ip>.sslip.io`) gets its own certificate, so many apps add up. Check the count for the last 7 days:

```bash
levelrail-cli certs budget
```

The command only reports. It exits 0 either way and nothing is blocked. The warning level is the runtime setting `certificates.weekly_issue_warn` (default 40); see [Runtime settings](/runtime-settings).

If you are over the level, stop the automatic address for apps that already have a real domain or do not need one:

```bash
levelrail-cli apps auto-host my-app            # show the state
levelrail-cli apps auto-host my-app --off      # no automatic hostname, no certificate for it
levelrail-cli apps auto-host my-app --on       # back to the default
```

With it off, the app is reachable only on domains you add.

## Related

- [Installing Levelrail](/installing)
- [Install on a cloud VM](/install-on-cloud)
- [Domains and ingress](/domains-and-ingress)
- [Diagnose a domain](/domain-diagnosis)
- [Go live checklist](/go-live)
