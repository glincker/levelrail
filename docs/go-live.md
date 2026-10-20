---
title: "Go live checklist"
icon: "/assets/icons/globe-hemisphere-west.svg"
sidebarTitle: "Go live on your domain"
description: One guided checklist from a running app to https on your own domain, with live status, one action per step, and a timeline of what happened.
"og:url": "https://docs.levelrail.com/go-live"
---

The **Go live** page of an app lists everything between "the app is running" and "`https://your-domain` serves it", in order. Each step shows a status, what it found in plain words, the evidence, and one button: a fix the control plane applies itself, a "Check again", or a link to the one thing you must do elsewhere. It refreshes on its own while you work through it and stops when the domain is live.

Open it from the success banner after a deploy, from the **Go live** tab of an app, from a domain's row on the Domains page, or from the doctor link in a domain diagnosis.

## The steps

| # | Step | Ok when | The button |
| --- | --- | --- | --- |
| 1 | App is running and healthy | The app is running and passes its readiness check | Link to the failing deploy's diagnosis |
| 2 | Reachable on its port | The app answers on its published port (a request from this server) | Open the automatic address, or choose a test port |
| 3 | Server firewall allows 80 and 443 | The host firewall allows both, is off, or does not exist | **Open ports 80 and 443** (auto fix) or the provider's steps |
| 4 | DNS points the domain here | The domain resolves to this server | **Create the record** when a DNS provider is connected, else **Copy the record** |
| 5 | Certificate is issued | A trusted certificate is in place | **Get a real certificate** (or **Try again**) |
| 6 | `https://domain` serves this app | A valid certificate and a response that matches the app | **Check again** |

A step that waits on something outside your control (DNS spreading, a certificate request) shows when it will look again. The summary badge on lists has five states: Not live, Waiting for DNS, Waiting for certificate, Live, Needs attention.

### 2. Reachable on its port

Every app has the automatic address (an `sslip.io` hostname served by the ingress). When the app exposure feature is allowed for the app, the step also shows the direct `host:port` URL with a copy button. The direct port is plain HTTP and bypasses the domain's TLS, basic auth and sign-in gates: use it to check that the app runs, never as its public address. See [App ports and exposure](/app-exposure).

### 3. Server firewall

The checklist reads the host firewall (ufw, firewalld, nftables, iptables, in that order) and reports whether ports 80 and 443 accept traffic from anywhere. The fix opens exactly those two TCP ports and nothing else, with these rules:

- It shows the exact commands and the undo commands before it runs, and the result lists what was added. The fix needs the root ability, is recorded in the audit log (`host_firewall.web_ports_opened`) and in the fix attempt history, and is rate limited like other fixes.
- It only runs when the control plane runs as root directly on the host. Inside a container, or without root, the step becomes guided: you get the commands to run yourself.
- It never enables a disabled firewall. A firewall that is off or allows everything is reported as not blocking and left alone.
- It never edits or deletes an existing rule. An explicit deny rule for 80 or 443 is reported and left for you to remove.
- It fails closed: if the firewall cannot be read, is ambiguous (several nftables input chains that drop), or any command fails, nothing stays half applied. Commands already run are undone.
- iptables and nftables rules apply immediately but are lost on reboot unless you save your ruleset. The fix says so.

A firewall at your **cloud or VPS provider** (security groups, cloud firewalls) cannot be changed from here. The checklist detects the provider from the machine's DMI strings, or from the provider's link-local metadata service with a one second timeout, and shows the exact steps and ports for DigitalOcean, AWS, Google Cloud, Hetzner, Azure, Oracle Cloud, Vultr and Linode, with a link to the provider's documentation. The ports to open are TCP 80 and TCP 443 from anywhere (and `::/0` if the domain has an AAAA record). It is guidance only: nothing in the product changes a provider firewall today. If the host firewall is fine but port 80 does not answer on this server's address, the step turns red and puts the provider steps first.

### 4. DNS

The step lists the record to create, how far it has propagated, and when it will look again. A proxied Cloudflare record is flagged, because a proxy in front changes who answers the certificate challenge. See [DNS zones and records](/dns).

### 5. Certificate

This is the same flow as [Get a real certificate](/domains-and-ingress#tls-whats-actually-shipped-today): a preflight (DNS, port 80, CAA, contact email, rate limits), then the request. The checklist only ever asks for a real certificate. The staging authority is for rehearsal and is refused while a working certificate exists, so it is not offered here. While waiting, the plain-language reason for a CA failure is shown, and when Let's Encrypt asks you to wait, the time you can try again.

### 6. Live

An end-to-end check: this server connects to the domain's address, verifies the certificate chain for the domain against the system roots, requests the app's readiness path, and compares the answer with what the app returns directly (the status, then the body, then the content type and server header). A mismatch means another server or a default page is answering. When the app is on another node, or its port cannot be read, the check can only say that a valid certificate and a response exist, and says so.

The check runs **from the control plane host**. It cannot prove the wider internet can reach your server, and no third-party probing service is used. The proof of outside reachability is the certificate itself: Let's Encrypt only issues one after it reached the domain over port 80 from the internet.

## Timeline

Below the steps is a timeline for the domain: DNS detected, ports open, certificate requested, certificate issued, renewal scheduled, and every failure with its reason. Issue, request and failure times come from the certificate store and the CA's recorded errors. "DNS detected", "ports open" and "live" are the first time this process observed them, so they reset when the control plane restarts.

## From the terminal

```bash
levelrail-cli go-live my-app                     # domain defaults to the app's only domain
levelrail-cli go-live my-app app.example.com --wait --timeout 10m
levelrail-cli go-live my-app app.example.com --json
```

It prints every step with its status, why, the next action, and the fix commands with their undo. With `--wait` it polls and exits 7 until the domain is live or `--timeout` passes (default 5m); without it the exit code is 0 whatever the state. Apply the firewall fix with `levelrail-cli fixes apply host.firewall.open_web_ports`.

## API

| Route | Ability | What it does |
| --- | --- | --- |
| `GET /api/v1/apps/{name}/go-live/checklist?domain=` | read | The checklist, with live probes |
| `GET /api/v1/apps/{name}/go-live/stream?domain=` | read | Server-sent events: a `checklist` snapshot every few seconds, then `done` |
| `GET /api/v1/go-live/status` | read | One badge per domain from stored state only, cheap enough for lists |

Fixes use the existing routes: `POST /api/v1/fixes/{id}/apply` (root), `POST /api/v1/domains/{domain}/cert` (root) and the DNS record routes. The stream refreshes every `APP_GOLIVE_POLL_INTERVAL` (default 10s) for at most `APP_GOLIVE_STREAM_ROUNDS` rounds (default 60), and ends after two live snapshots in a row. `APP_GOLIVE_FIREWALL_TIMEOUT` (default 5s) bounds the firewall read.

## What has not been verified

The firewall reader and the fixes are tested against recorded tool output and a fake command runner, not against real ufw, firewalld, nftables or iptables hosts. The provider detection and the provider steps and documentation links are tested against fake DMI and metadata responders, not against real cloud accounts. The certificate step uses the same issuance code as [Get a real certificate](/domains-and-ingress); a live Let's Encrypt run of the whole checklist has not been recorded.
